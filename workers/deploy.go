package workers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	app "github.com/sjeltuhin/clusterAgent/appd"
	"github.com/sjeltuhin/clusterAgent/config"
	instr "github.com/sjeltuhin/clusterAgent/instrumentation"
	m "github.com/sjeltuhin/clusterAgent/models"
	"github.com/sjeltuhin/clusterAgent/utils"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
	ConfigManager  *config.MutexConfigManager
	SummaryMap     map[string]m.ClusterDeployMetrics
	WQ             workqueue.RateLimitingInterface
	AppdController *app.ControllerClient
	PendingCache   []string
	FailedCache    map[string]m.AttachStatus
}

func NewDeployWorker(client *kubernetes.Clientset, cm *config.MutexConfigManager, controller *app.ControllerClient) DeployWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	dw := DeployWorker{Client: client, ConfigManager: cm, SummaryMap: make(map[string]m.ClusterDeployMetrics), WQ: queue,
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

	wg.Add(1)
	go dw.startMetricsWorker(stopCh)

	wg.Add(1)
	go dw.startEventQueueWorker(stopCh)

	<-stopCh
}

func (pw *DeployWorker) qualifies(p *appsv1.Deployment) bool {
	return (len((*pw.ConfigManager).Get().NsToMonitor) == 0 ||
		utils.StringInSlice(p.Namespace, (*pw.ConfigManager).Get().NsToMonitor)) &&
		!utils.StringInSlice(p.Namespace, (*pw.ConfigManager).Get().NsToMonitorExclude)
}

func (dw *DeployWorker) onNewDeployment(obj interface{}) {
	deployObj := obj.(*appsv1.Deployment)
	if !dw.qualifies(deployObj) {
		return
	}
	fmt.Printf("Added Deployment: %s\n", deployObj.Name)

	deployRecord, _ := dw.processObject(deployObj, nil)
	dw.WQ.Add(&deployRecord)

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

	deployRecord, _ := dw.processObject(deployObj, nil)
	dw.WQ.Add(&deployRecord)

	init, biq, agentRequests := dw.shouldUpdate(deployObj)
	if init || biq {
		fmt.Printf("Update is required. Init: %t. BiQ: %t\n", init, biq)
		dw.updateDeployment(deployObj, init, biq, agentRequests)
	}
}

func (pw *DeployWorker) startMetricsWorker(stopCh <-chan struct{}) {
	bag := (*pw.ConfigManager).Get()
	pw.appMetricTicker(stopCh, time.NewTicker(time.Duration(bag.MetricsSyncInterval)*time.Second))

}

func (pw *DeployWorker) appMetricTicker(stop <-chan struct{}, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			pw.buildAppDMetrics()
		case <-stop:
			ticker.Stop()
			return
		}
	}
}

func (pw *DeployWorker) eventQueueTicker(stop <-chan struct{}, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			pw.flushQueue()
		case <-stop:
			ticker.Stop()
			return
		}
	}
}

func (pw *DeployWorker) startEventQueueWorker(stopCh <-chan struct{}) {
	bag := (*pw.ConfigManager).Get()
	pw.eventQueueTicker(stopCh, time.NewTicker(time.Duration(bag.SnapshotSyncInterval)*time.Second))
}

func (pw *DeployWorker) flushQueue() {
	bag := (*pw.ConfigManager).Get()
	bth := pw.AppdController.StartBT("FlushDeploymentDataQueue")
	count := pw.WQ.Len()
	if count > 0 {
		fmt.Printf("Flushing the queue of %d deployment records\n", count)
	}
	if count == 0 {
		pw.AppdController.StopBT(bth)
		return
	}

	var objList []m.DeploySchema

	var deployRecord *m.DeploySchema
	var ok bool = true

	for count >= 0 {
		deployRecord, ok = pw.getNextQueueItem()
		count = count - 1
		if ok {
			objList = append(objList, *deployRecord)
		} else {
			fmt.Println("Queue shut down")
		}
		if count == 0 || len(objList) >= bag.EventAPILimit {
			fmt.Printf("Sending %d deployment records to AppD events API\n", len(objList))
			pw.postDeployRecords(&objList)
			pw.AppdController.StopBT(bth)
			return
		}
	}
	pw.AppdController.StopBT(bth)
}

func (pw *DeployWorker) postDeployRecords(objList *[]m.DeploySchema) {
	bag := (*pw.ConfigManager).Get()
	logger := log.New(os.Stdout, "[APPD_CLUSTER_MONITOR]", log.Lshortfile)
	rc := app.NewRestClient(bag, logger)
	data, err := json.Marshal(objList)
	schemaDefObj := m.NewDeploySchemaDefWrapper()
	schemaDef, e := json.Marshal(schemaDefObj)
	if err == nil && e == nil {
		if rc.SchemaExists(bag.DeploySchemaName) == false {
			fmt.Printf("Creating schema. %s\n", bag.DeploySchemaName)
			schemaObj, err := rc.CreateSchema(bag.DeploySchemaName, schemaDef)
			if err != nil {
				return
			} else if schemaObj != nil {
				fmt.Printf("Schema %s created\n", bag.DeploySchemaName)
			} else {
				fmt.Printf("Schema %s exists\n", bag.DeploySchemaName)
			}
		}
		fmt.Println("About to post records")
		rc.PostAppDEvents(bag.DeploySchemaName, data)
	} else {
		fmt.Printf("Problems when serializing array of pod schemas. %v\n", err)
	}
}

func (pw *DeployWorker) getNextQueueItem() (*m.DeploySchema, bool) {
	deployRecord, quit := pw.WQ.Get()

	if quit {
		return deployRecord.(*m.DeploySchema), false
	}
	defer pw.WQ.Done(deployRecord)
	pw.WQ.Forget(deployRecord)

	return deployRecord.(*m.DeploySchema), true
}

func (pw *DeployWorker) buildAppDMetrics() {
	bth := pw.AppdController.StartBT("PostDeploymentMetrics")
	pw.SummaryMap = make(map[string]m.ClusterDeployMetrics)

	var count int = 0
	for _, obj := range pw.informer.GetStore().List() {
		deployObject := obj.(*appsv1.Deployment)
		deploySchema, _ := pw.processObject(deployObject, nil)
		pw.summarize(&deploySchema)
		count++
	}

	fmt.Printf("Unique deployment metrics: %d\n", count)

	ml := pw.builAppDMetricsList()

	fmt.Printf("Ready to push %d deployment metrics\n", len(ml.Items))

	pw.AppdController.PostMetrics(ml)
	pw.AppdController.StopBT(bth)
}

func (pw *DeployWorker) summarize(deployObject *m.DeploySchema) {
	bag := (*pw.ConfigManager).Get()
	//global metrics
	summary, okSum := pw.SummaryMap[m.ALL]
	if !okSum {
		summary = m.NewClusterDeployMetrics(bag, m.ALL)
		pw.SummaryMap[m.ALL] = summary
	}

	//namespace metrics
	summaryNS, okNS := pw.SummaryMap[deployObject.Namespace]
	if !okNS {
		summaryNS = m.NewClusterDeployMetrics(bag, deployObject.Namespace)
		pw.SummaryMap[deployObject.Namespace] = summaryNS
	}

	summary.DeployCount++
	summaryNS.DeployCount++

	summary.DeployReplicas = summary.DeployReplicas + int64(deployObject.Replicas)
	summaryNS.DeployReplicas = summaryNS.DeployReplicas + int64(deployObject.Replicas)

	summary.DeployReplicasUnAvailable = summary.DeployReplicasUnAvailable + int64(deployObject.ReplicasUnAvailable)
	summaryNS.DeployReplicasUnAvailable = summaryNS.DeployReplicasUnAvailable + int64(deployObject.ReplicasUnAvailable)

	summary.DeployCollisionCount = summary.DeployCollisionCount + int64(deployObject.CollisionCount)
	summaryNS.DeployCollisionCount = summaryNS.DeployCollisionCount + int64(deployObject.CollisionCount)

	pw.SummaryMap[m.ALL] = summary
	pw.SummaryMap[deployObject.Namespace] = summaryNS
}

func (pw *DeployWorker) processObject(d *appsv1.Deployment, old *appsv1.Deployment) (m.DeploySchema, bool) {
	changed := true
	bag := (*pw.ConfigManager).Get()

	deployObject := m.NewDeployObj()
	deployObject.Name = d.Name
	deployObject.Namespace = d.Namespace

	if d.ClusterName != "" {
		deployObject.ClusterName = d.ClusterName
	} else {
		deployObject.ClusterName = bag.AppName
	}

	var sb strings.Builder

	for k, l := range d.Labels {
		fmt.Fprintf(&sb, "%s:%s;", k, l)
	}
	deployObject.Labels = sb.String()
	sb.Reset()

	for k, l := range d.Annotations {
		fmt.Fprintf(&sb, "%s:%s;", k, l)
	}

	deployObject.Annotations = sb.String()
	sb.Reset()

	deployObject.ObjectUid = string(d.GetUID())
	deployObject.CreationTimestamp = d.GetCreationTimestamp().Time
	if d.GetDeletionTimestamp() != nil {
		deployObject.DeletionTimestamp = d.GetDeletionTimestamp().Time
	}
	deployObject.MinReadySecs = d.Spec.MinReadySeconds
	if d.Spec.ProgressDeadlineSeconds != nil {
		deployObject.ProgressDeadlineSecs = *d.Spec.ProgressDeadlineSeconds
	}

	if d.Spec.RevisionHistoryLimit != nil {
		deployObject.RevisionHistoryLimits = *d.Spec.RevisionHistoryLimit
	}

	deployObject.Replicas = d.Status.Replicas

	deployObject.Strategy = string(d.Spec.Strategy.Type)

	if d.Spec.Strategy.RollingUpdate.MaxSurge != nil {
		deployObject.MaxSurge = d.Spec.Strategy.RollingUpdate.MaxSurge.StrVal
	}
	if d.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
		deployObject.MaxUnavailable = d.Spec.Strategy.RollingUpdate.MaxUnavailable.StrVal
	}
	deployObject.ReplicasAvailable = d.Status.AvailableReplicas
	deployObject.ReplicasUnAvailable = d.Status.UnavailableReplicas
	deployObject.ReplicasUpdated = d.Status.UpdatedReplicas
	if d.Status.CollisionCount != nil {
		deployObject.CollisionCount = *d.Status.CollisionCount
	}
	deployObject.ReplicasReady = d.Status.ReadyReplicas

	return deployObject, changed
}

func (pw DeployWorker) builAppDMetricsList() m.AppDMetricList {
	ml := m.NewAppDMetricList()
	var list []m.AppDMetric
	for _, metricNode := range pw.SummaryMap {
		objMap := metricNode.Unwrap()
		pw.addMetricToList(*objMap, metricNode, &list)
	}

	ml.Items = list
	return ml
}

func (pw DeployWorker) addMetricToList(objMap map[string]interface{}, metric m.AppDMetricInterface, list *[]m.AppDMetric) {

	for fieldName, fieldValue := range objMap {
		if !metric.ShouldExcludeField(fieldName) {
			appdMetric := m.NewAppDMetric(fieldName, fieldValue.(int64), metric.GetPath())
			*list = append(*list, appdMetric)
		}
	}
}

//instrumentation
func (dw *DeployWorker) shouldUpdate(deployObj *appsv1.Deployment) (bool, bool, *m.AgentRequestList) {
	bag := (*dw.ConfigManager).Get()
	var appName, tierName, appAgent string
	var biqRequested = false
	var agentRequests *m.AgentRequestList = nil
	var biQDeploymentOption m.BiQDeploymentOption
	for k, v := range deployObj.Labels {
		if k == bag.AppDAppLabel {
			appName = v
		}

		if k == bag.AppDTierLabel {
			tierName = v
		}

		if k == bag.AgentLabel {
			appAgent = v
		}

		if k == bag.AppDAnalyticsLabel {
			biqRequested = true
			biQDeploymentOption = m.BiQDeploymentOption(v)
		}
	}
	initRequested := !instr.AgentInitExists(&deployObj.Spec.Template.Spec, bag) && (appName != "" || appAgent != "") && (bag.InstrumentationMethod == m.Mount || bag.InstrumentationMethod == m.MountEnv)

	//	if !initRequested {
	//		initRequested = utils.DeployQualifiesForInstrumentation(deployObj, bag)
	//		if initRequested {
	//			agentRequests = utils.GetAgentRequestsForDeployment(deployObj, bag)
	//		}
	//	}

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

	//	check Failed cache not to exceed failuer limit
	status, ok := dw.FailedCache[utils.GetDeployKey(deployObj)]
	if ok && status.Count >= instr.MAX_INSTRUMENTATION_ATTEMPTS {
		fmt.Printf("Deployment %s exceeded the max number of failed instrumentation attempts. Skipping...\n", deployObj.Name)
		return false, false, nil
	}

	biq := biQDeploymentOption == m.Sidecar && !instr.AnalyticsAgentExists(&deployObj.Spec.Template.Spec, (*dw.ConfigManager).Get())

	if tierName == "" {
		tierName = deployObj.Name
	}

	if initRequested || biq {
		if agentRequests == nil {
			l := m.NewAgentRequestList(appAgent, appName, tierName)
			agentRequests = &l
		} else {
			//merge
			agentRequests.AppName = appName
			agentRequests.TierName = tierName
		}
		agentRequests.ApplyInstrumentationMethod(bag.InstrumentationMethod)
	}

	return initRequested, biq, agentRequests
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

			//check if appd-secret exists
			//if not copy into the namespace
			fmt.Println("Ensuring secret...")
			errSecret := dw.ensureSecret(deployObj.Namespace)
			if errSecret != nil {
				fmt.Printf("Failed to ensure secret in namespace %s: %v", deployObj.Namespace, errSecret)
				return fmt.Errorf("Failed to ensure secret in namespace %s: %v", deployObj.Namespace, errSecret)
			}
			//add volume and mounts for agent binaries
			dw.updateSpec(result, (*dw.ConfigManager).Get().AgentMountName, (*dw.ConfigManager).Get().AgentMountPath, agentRequests, true)
			//add init container for agent attach
			agentReq := agentRequests.GetFirstRequest()
			agentAttachContainer := dw.buildInitContainer(&agentReq)
			result.Spec.Template.Spec.InitContainers = append(result.Spec.Template.Spec.InitContainers, agentAttachContainer)

			//annotate pod
			if result.Spec.Template.Annotations == nil {
				result.Spec.Template.Annotations = make(map[string]string)
			}
			result.Spec.Template.Annotations["appd-instr-method"] = string(agentReq.Method)
			result.Spec.Template.Annotations["appd-tech"] = string(agentReq.Tech)
			contList := agentRequests.GetContainerNames()
			if contList != nil {
				result.Spec.Template.Annotations["appd-container-list"] = strings.Join(*contList, ",")
			}

			//annotate deployment
			if result.Annotations == nil {
				result.Annotations = make(map[string]string)
			}
			result.Annotations[instr.DEPLOY_ANNOTATION] = time.Now().String()
		}

		if biq {
			fmt.Println("Adding analytics container")
			//add volume and mounts for logging
			dw.updateSpec(result, (*dw.ConfigManager).Get().AppLogMountName, (*dw.ConfigManager).Get().AppLogMountPath, agentRequests, false)

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
	bag := (*dw.ConfigManager).Get()
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
		ar := agentRequests.GetRequest(c.Name)
		if ar.Method == m.None {
			ar.Method = bag.InstrumentationMethod
		}
		volumeMount := v1.VolumeMount{Name: volName, MountPath: volumePath}
		if c.VolumeMounts == nil || len(c.VolumeMounts) == 0 {
			c.VolumeMounts = []v1.VolumeMount{volumeMount}
		} else {
			c.VolumeMounts = append(c.VolumeMounts, volumeMount)
		}
		if envUpdate {
			tech := ar.Tech
			if tech == m.DotNet {
				fmt.Printf("Requested env var update for DotNet container %s\n", c.Name)
				dotnetInjector := instr.NewDotNetInjector(bag, dw.AppdController)
				dotnetInjector.AddEnvVars(&c, agentRequests.AppName, agentRequests.TierName)
			}

			//if the method is MountEnv and tech is Java, build the env var for the agent
			fmt.Printf("instrument method =  %s\n", m.MountEnv)
			if tech == m.Java && ar.Method == m.MountEnv {
				fmt.Printf("Requested env var update for java container %s\n", c.Name)
				optsExist := false
				javaOptsVal := fmt.Sprintf(` -Dappdynamics.agent.accountAccessKey=$(APPDYNAMICS_AGENT_ACCOUNT_ACCESS_KEY) -Dappdynamics.controller.hostName=%s -Dappdynamics.controller.port=%d -Dappdynamics.controller.ssl.enabled=%t -Dappdynamics.agent.accountName=%s -Dappdynamics.agent.applicationName=%s -Dappdynamics.agent.tierName=%s -Dappdynamics.agent.reuse.nodeName=true -Dappdynamics.agent.reuse.nodeName.prefix=%s -javaagent:%s/javaagent.jar `,
					bag.ControllerUrl, bag.ControllerPort, bag.SSLEnabled, bag.Account, agentRequests.AppName, agentRequests.TierName, agentRequests.TierName, bag.AgentMountPath)
				if c.Env == nil {
					c.Env = []v1.EnvVar{}
				} else {
					for _, ev := range c.Env {
						if ev.Name == bag.AgentEnvVar {
							ev.Value += javaOptsVal
							optsExist = true
							break
						}
					}
				}
				//key reference
				keyRef := v1.SecretKeySelector{Key: instr.APPD_SECRET_KEY_NAME, LocalObjectReference: v1.LocalObjectReference{
					Name: instr.APPD_SECRET_NAME}}
				envVarKey := v1.EnvVar{Name: "APPDYNAMICS_AGENT_ACCOUNT_ACCESS_KEY", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &keyRef}}
				if !optsExist {
					envJavaOpts := v1.EnvVar{Name: bag.AgentEnvVar, Value: javaOptsVal}
					c.Env = append(c.Env, envJavaOpts)
				}
				//prepend the secret ref
				c.Env = append([]v1.EnvVar{envVarKey}, c.Env...)
			}

		}
		clist = append(clist, c)
		if applyTo == nil {
			//apply only first container
			break
		}
	}
	result.Spec.Template.Spec.Containers = clist
}

func (dw *DeployWorker) ensureSecret(ns string) error {
	bag := (*dw.ConfigManager).Get()
	var secret *v1.Secret

	_, errGet := dw.Client.CoreV1().Secrets(ns).Get(instr.APPD_SECRET_NAME, metav1.GetOptions{})
	if errGet != nil && !errors.IsNotFound(errGet) {
		return errGet
	}

	if errors.IsNotFound(errGet) {
		secret = &v1.Secret{
			Type: v1.SecretTypeOpaque,
			ObjectMeta: metav1.ObjectMeta{
				Name:      instr.APPD_SECRET_NAME,
				Namespace: ns,
			},
		}
		fmt.Printf("Secret %s does not exist in namespace %s. Creating...\n", secret.Name, ns)

		secret.StringData = make(map[string]string)
		secret.StringData[instr.APPD_SECRET_KEY_NAME] = bag.AccessKey

		_, err := dw.Client.CoreV1().Secrets(ns).Create(secret)
		fmt.Printf("Secret %s. %v\n", secret.Name, err)
		if err != nil {
			fmt.Printf("Unable to create secret. %v\n", err)
		}
		return err
	}
	fmt.Printf("Secret %s exists. No action required\n", instr.APPD_SECRET_NAME)

	return nil
}

func (dw *DeployWorker) buildInitContainer(agentrequest *m.AgentRequest) v1.Container {
	fmt.Printf("Building init container using agent request %s\n", agentrequest.ToString())
	//volume mount for agent files
	volumeMount := v1.VolumeMount{Name: (*dw.ConfigManager).Get().AgentMountName, MountPath: (*dw.ConfigManager).Get().AgentMountPath}
	mounts := []v1.VolumeMount{volumeMount}

	cmd := []string{"cp", "-ra", (*dw.ConfigManager).Get().InitContainerDir, (*dw.ConfigManager).Get().AgentMountPath}

	resRequest := v1.ResourceList{}
	resRequest[v1.ResourceCPU] = resource.MustParse("0.1")
	resRequest[v1.ResourceMemory] = resource.MustParse("50M")

	resLimit := v1.ResourceList{}
	resLimit[v1.ResourceCPU] = resource.MustParse("0.15")
	resLimit[v1.ResourceMemory] = resource.MustParse("75M")
	reqs := v1.ResourceRequirements{Requests: resRequest, Limits: resLimit}

	cont := v1.Container{Name: (*dw.ConfigManager).Get().AppDInitContainerName, Image: agentrequest.GetAgentImageName((*dw.ConfigManager).Get()), ImagePullPolicy: v1.PullIfNotPresent,
		VolumeMounts: mounts, Command: cmd, Resources: reqs}

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
	volumeMount := v1.VolumeMount{Name: (*dw.ConfigManager).Get().AppLogMountName, MountPath: (*dw.ConfigManager).Get().AppLogMountPath}
	mounts := []v1.VolumeMount{volumeMount}

	resRequest := v1.ResourceList{}
	resRequest[v1.ResourceCPU] = resource.MustParse("0.1")
	resRequest[v1.ResourceMemory] = resource.MustParse("50M")

	resLimit := v1.ResourceList{}
	resLimit[v1.ResourceCPU] = resource.MustParse("0.2")
	resLimit[v1.ResourceMemory] = resource.MustParse("100M")
	reqs := v1.ResourceRequirements{Requests: resRequest, Limits: resLimit}

	cont := v1.Container{Name: (*dw.ConfigManager).Get().AnalyticsAgentContainerName, Image: (*dw.ConfigManager).Get().AnalyticsAgentImage, ImagePullPolicy: v1.PullIfNotPresent,
		Ports: ports, EnvFrom: envFrom, Env: env, VolumeMounts: mounts, Resources: reqs}

	return cont
}

func (dw *DeployWorker) addVolume(deployObj *appsv1.Deployment, container *v1.Container) {
	bag := (*dw.ConfigManager).Get()
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
