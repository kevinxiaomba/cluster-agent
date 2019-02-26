package workers

import (
	//	"encoding/json"
	"fmt"
	"sync"
	"time"

	app "github.com/sjeltuhin/clusterAgent/appd"
	"github.com/sjeltuhin/clusterAgent/config"
	instr "github.com/sjeltuhin/clusterAgent/instrumentation"
	m "github.com/sjeltuhin/clusterAgent/models"
	"github.com/sjeltuhin/clusterAgent/utils"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
)

type DeployWorker struct {
	informer       cache.SharedIndexInformer
	Client         *kubernetes.Clientset
	ConfManager    *config.MutexConfigManager
	SummaryMap     map[string]m.ClusterPodMetrics
	WQ             workqueue.RateLimitingInterface
	AppdController *app.ControllerClient
	PendingCache   []string
	FailedCache    map[string]m.AttachStatus
}

func NewDeployWorker(client *kubernetes.Clientset, cm *config.MutexConfigManager, controller *app.ControllerClient) DeployWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	dw := DeployWorker{Client: client, ConfManager: cm, SummaryMap: make(map[string]m.ClusterPodMetrics), WQ: queue,
		AppdController: controller, PendingCache: []string{}, FailedCache: make(map[string]m.AttachStatus)}
	dw.initDeployInformer(client)
	return dw
}

func (nw *DeployWorker) initDeployInformer(client *kubernetes.Clientset) cache.SharedIndexInformer {
	i := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.AppsV1().Deployments(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.AppsV1().Deployments(metav1.NamespaceAll).Watch(options)
			},
		},
		&appsv1.Deployment{},
		0,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	i.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    nw.onNewDeployment,
		DeleteFunc: nw.onDeleteDeployment,
		UpdateFunc: nw.onUpdateDeployment,
	})
	nw.informer = i

	return i
}

func (dw *DeployWorker) Observe(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	defer dw.WQ.ShutDown()
	wg.Add(1)
	go dw.informer.Run(stopCh)

	<-stopCh
}

func (pw *DeployWorker) qualifies(p *appsv1.Deployment) bool {
	return (len((*pw.ConfManager).Get().IncludeNsToInstrument) == 0 ||
		utils.StringInSlice(p.Namespace, (*pw.ConfManager).Get().IncludeNsToInstrument)) &&
		!utils.StringInSlice(p.Namespace, (*pw.ConfManager).Get().ExcludeNsToInstrument)
}

func (dw *DeployWorker) onNewDeployment(obj interface{}) {
	deployObj := obj.(*appsv1.Deployment)
	if !dw.qualifies(deployObj) {
		return
	}
	fmt.Printf("Added Deployment: %s\n", deployObj.Name)

	init, biq, agentRequests := dw.shouldUpdate(deployObj)
	if init || biq {
		dw.updateDeployment(deployObj, init, biq, agentRequests)
	}
}

func (dw *DeployWorker) onDeleteDeployment(obj interface{}) {
	deployObj := obj.(*appsv1.Deployment)
	if !dw.qualifies(deployObj) {
		return
	}
	fmt.Printf("Deleted Deployment: %s\n", deployObj.Name)
}

func (dw *DeployWorker) onUpdateDeployment(objOld interface{}, objNew interface{}) {
	deployObj := objNew.(*appsv1.Deployment)
	if !dw.qualifies(deployObj) {
		return
	}
	fmt.Printf("Deployment %s changed\n", deployObj.Name)
	init, biq, agentRequests := dw.shouldUpdate(deployObj)
	if init || biq {
		fmt.Printf("Update is required. Init: %t. BiQ: %t\n", init, biq)
		dw.updateDeployment(deployObj, init, biq, agentRequests)
	}
}

func (dw *DeployWorker) shouldUpdate(deployObj *appsv1.Deployment) (bool, bool, *m.AgentRequestList) {
	var appName, tierName, appAgent string
	var biqRequested = false
	var biQDeploymentOption m.BiQDeploymentOption
	for k, v := range deployObj.Labels {
		if k == (*dw.ConfManager).Get().AppDAppLabel {
			appName = v
		}

		if k == (*dw.ConfManager).Get().AppDTierLabel {
			tierName = v
		}

		if k == (*dw.ConfManager).Get().AgentLabel {
			appAgent = v
		}

		if k == (*dw.ConfManager).Get().AppDAnalyticsLabel {
			biqRequested = true
			biQDeploymentOption = m.BiQDeploymentOption(v)
		}
	}
	initRequested := !instr.AgentInitExists(&deployObj.Spec.Template.Spec, (*dw.ConfManager).Get()) && (appName != "" || appAgent != "") && (*dw.ConfManager).Get().InstrumentationMethod == m.Mount
	//check if already updated
	updated := false
	biqUpdated := false

	for k, v := range deployObj.Annotations {
		if k == instr.DEPLOY_ANNOTATION && v != "" {
			updated = true
		}

		if k == instr.DEPLOY_BIQ_ANNOTATION && v != "" {
			biqUpdated = true
		}
	}

	fmt.Printf("Update status: %t. BiQ updated: %t\n", updated, biqUpdated)
	if (!initRequested || updated) && (!biqRequested || biqUpdated) {
		if updated || biqUpdated {
			dw.PendingCache = utils.RemoveFromSlice(utils.GetDeployKey(deployObj), dw.PendingCache)
			fmt.Printf("Deployment %s already updated for AppD. Skipping...\n", deployObj.Name)
		} else {
			fmt.Printf("Instrumentation not requested. Skipping %s...\n", deployObj.Name)
		}
		return false, false, nil
	}

	if utils.StringInSlice(utils.GetDeployKey(deployObj), dw.PendingCache) {
		fmt.Printf("Deployment %s is in process of update. Waiting...\n", deployObj.Name)
		return false, false, nil
	}

	//check Failed cache not to exceed failuer limit
	status, ok := dw.FailedCache[utils.GetDeployKey(deployObj)]
	if ok && status.Count >= instr.MAX_INSTRUMENTATION_ATTEMPTS {
		fmt.Printf("Deployment %s exceeded the max number of failed instrumentation attempts. Skipping...\n", deployObj.Name)
		return false, false, nil
	}

	biq := biQDeploymentOption == m.Sidecar && !instr.AnalyticsAgentExists(&deployObj.Spec.Template.Spec, (*dw.ConfManager).Get())

	if tierName == "" {
		tierName = deployObj.Name
	}
	agentRequests := m.NewAgentRequestList(appAgent, appName, tierName)

	return initRequested, biq, &agentRequests
}

func (dw *DeployWorker) updateDeployment(deployObj *appsv1.Deployment, init bool, biq bool, agentRequests *m.AgentRequestList) {
	if !init && !biq {
		return
	}

	dw.PendingCache = append(dw.PendingCache, utils.GetDeployKey(deployObj))

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		bth := dw.AppdController.StartBT("DeploymentUpdate")
		deploymentsClient := dw.Client.AppsV1().Deployments(deployObj.Namespace)
		result, getErr := deploymentsClient.Get(deployObj.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("Failed to get latest version of Deployment: %v", getErr)
		}

		if init {
			fmt.Println("Adding init container...")
			//add volume and mounts for agent binaries
			dw.updateSpec(result, (*dw.ConfManager).Get().AgentMountName, (*dw.ConfManager).Get().AgentMountPath, agentRequests, true)
			//add init container for agent attach
			agentReq := agentRequests.GetFirstRequest()
			agentAttachContainer := dw.buildInitContainer(&agentReq)
			result.Spec.Template.Spec.InitContainers = append(result.Spec.Template.Spec.InitContainers, agentAttachContainer)

			//annotate
			if result.Annotations == nil {
				result.Annotations = make(map[string]string)
			}
			result.Annotations[instr.DEPLOY_ANNOTATION] = time.Now().String()
		}

		if biq {
			fmt.Println("Adding analytics container")
			//add volume and mounts for logging
			dw.updateSpec(result, (*dw.ConfManager).Get().AppLogMountName, (*dw.ConfManager).Get().AppLogMountPath, agentRequests, false)

			//add analytics agent container
			analyticsContainer := dw.buildBiqSideCar()
			result.Spec.Template.Spec.Containers = append(result.Spec.Template.Spec.Containers, analyticsContainer)

			//annotate that biq is instrumented
			if result.Annotations == nil {
				result.Annotations = make(map[string]string)
			}
			result.Annotations[instr.DEPLOY_BIQ_ANNOTATION] = time.Now().String()
		}

		_, err := deploymentsClient.Update(result)
		dw.AppdController.StopBT(bth)
		return err
	})

	if retryErr != nil {
		fmt.Printf("Deployment update failed: %v\n", retryErr)
		//add to failed cache
		status, ok := dw.FailedCache[utils.GetDeployKey(deployObj)]
		if !ok {
			status = m.AttachStatus{Key: utils.GetDeployKey(deployObj)}
		}
		status.Count++
		status.LastAttempt = time.Now()
		status.LastMessage = retryErr.Error()
		dw.FailedCache[utils.GetDeployKey(deployObj)] = status
		//clear from pending
		dw.PendingCache = utils.RemoveFromSlice(utils.GetDeployKey(deployObj), dw.PendingCache)
	}

	fmt.Println("Updated deployment...")
}

func (dw *DeployWorker) updateSpec(result *appsv1.Deployment, volName string, volumePath string, agentRequests *m.AgentRequestList, envUpdate bool) {
	applyTo := agentRequests.GetContainerNames()
	if applyTo == nil {
		fmt.Printf("Adding volume %s to all containers\n", volName)
	} else {
		fmt.Printf("Adding volume %s to the following containers: %s\n", volName, *applyTo)
	}
	//add shared volume to the deployment spec for logging
	vol := v1.Volume{Name: volName, VolumeSource: v1.VolumeSource{
		EmptyDir: &v1.EmptyDirVolumeSource{},
	}}
	if result.Spec.Template.Spec.Volumes == nil || len(result.Spec.Template.Spec.Volumes) == 0 {
		result.Spec.Template.Spec.Volumes = []v1.Volume{vol}
	} else {
		result.Spec.Template.Spec.Volumes = append(result.Spec.Template.Spec.Volumes, vol)
	}

	//add volume mount to the application container(s)
	clist := []v1.Container{}
	for _, c := range result.Spec.Template.Spec.Containers {
		if applyTo != nil && !utils.StringInSlice(c.Name, *applyTo) {
			clist = append(clist, c)
			fmt.Printf("Container %s is not on the list. Not adding volume mount...\n", c.Name)
			continue
		}
		volumeMount := v1.VolumeMount{Name: volName, MountPath: volumePath}
		if c.VolumeMounts == nil || len(c.VolumeMounts) == 0 {
			c.VolumeMounts = []v1.VolumeMount{volumeMount}
		} else {
			c.VolumeMounts = append(c.VolumeMounts, volumeMount)
		}
		if envUpdate {
			tech := agentRequests.GetFirstRequest().Tech
			if tech == m.DotNet {
				fmt.Printf("Requested env var update for DotNet container %s\n", c.Name)
				dotnetInjector := instr.NewDotNetInjector((*dw.ConfManager).Get(), dw.AppdController)
				dotnetInjector.AddEnvVars(&c, agentRequests.AppName, agentRequests.TierName)
			}
		}
		clist = append(clist, c)
	}
	result.Spec.Template.Spec.Containers = clist
}

func (dw *DeployWorker) buildInitContainer(agentrequest *m.AgentRequest) v1.Container {
	fmt.Printf("Building init container using agent request %s\n", agentrequest.ToString())
	//volume mount for agent files
	volumeMount := v1.VolumeMount{Name: (*dw.ConfManager).Get().AgentMountName, MountPath: (*dw.ConfManager).Get().AgentMountPath}
	mounts := []v1.VolumeMount{volumeMount}

	cmd := []string{"cp", "-ra", (*dw.ConfManager).Get().InitContainerDir, (*dw.ConfManager).Get().AgentMountPath}
	cont := v1.Container{Name: (*dw.ConfManager).Get().AppDInitContainerName, Image: agentrequest.GetAgentImageName((*dw.ConfManager).Get()), ImagePullPolicy: v1.PullIfNotPresent,
		VolumeMounts: mounts, Command: cmd}

	return cont
}

func (dw *DeployWorker) buildBiqSideCar() v1.Container {
	// configMap reference
	cmRef := v1.ConfigMapEnvSource{}
	cmRef.Name = "controller-config"
	envFromMap := v1.EnvFromSource{ConfigMapRef: &cmRef}
	envFrom := []v1.EnvFromSource{envFromMap}

	//key reference
	keyRef := v1.SecretKeySelector{Key: "appd-key", LocalObjectReference: v1.LocalObjectReference{
		Name: "appd-secret"}}
	envVar := v1.EnvVar{Name: "ACCOUNT_ACCESS_KEY", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &keyRef}}
	env := []v1.EnvVar{envVar}

	//ports
	p := v1.ContainerPort{ContainerPort: 9090}
	ports := []v1.ContainerPort{p}

	//volume mount for logs
	volumeMount := v1.VolumeMount{Name: (*dw.ConfManager).Get().AppLogMountName, MountPath: (*dw.ConfManager).Get().AppLogMountPath}
	mounts := []v1.VolumeMount{volumeMount}

	cont := v1.Container{Name: (*dw.ConfManager).Get().AnalyticsAgentContainerName, Image: (*dw.ConfManager).Get().AnalyticsAgentImage, ImagePullPolicy: v1.PullIfNotPresent,
		Ports: ports, EnvFrom: envFrom, Env: env, VolumeMounts: mounts}

	return cont
}

func (dw *DeployWorker) addVolume(deployObj *appsv1.Deployment, container *v1.Container) {
	bag := (*dw.ConfManager).Get()
	mountExists := false
	for _, vm := range container.VolumeMounts {
		if vm.Name == bag.AgentMountName {
			mountExists = true
			break
		}
	}
	if !mountExists {
		//container volume mount
		volumeMount := v1.VolumeMount{Name: bag.AgentMountName, MountPath: "/opt/appd", ReadOnly: true}
		if container.VolumeMounts == nil || len(container.VolumeMounts) == 0 {
			container.VolumeMounts = []v1.VolumeMount{volumeMount}
		} else {
			container.VolumeMounts = append(container.VolumeMounts, volumeMount)
		}
	}

	volExists := false
	for _, vol := range deployObj.Spec.Template.Spec.Volumes {
		if vol.Name == bag.AgentMountName {
			volExists = true
			break
		}
	}
	if !volExists {
		//pod volume
		pathType := v1.HostPathDirectory
		source := v1.HostPathVolumeSource{Path: "/opt/appd", Type: &pathType}
		volSource := v1.VolumeSource{HostPath: &source}
		vol := v1.Volume{Name: "appd-repo", VolumeSource: volSource}
		deployObj.Spec.Template.Spec.Volumes = []v1.Volume{vol}
	}
}
