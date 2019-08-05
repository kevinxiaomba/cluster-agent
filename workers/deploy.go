package workers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/intstr"

	log "github.com/sirupsen/logrus"

	app "github.com/appdynamics/cluster-agent/appd"
	"github.com/appdynamics/cluster-agent/config"
	instr "github.com/appdynamics/cluster-agent/instrumentation"
	m "github.com/appdynamics/cluster-agent/models"
	"github.com/appdynamics/cluster-agent/utils"
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
	Logger         *log.Logger
}

func NewDeployWorker(client *kubernetes.Clientset, cm *config.MutexConfigManager, controller *app.ControllerClient, l *log.Logger) DeployWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	dw := DeployWorker{Client: client, ConfigManager: cm, SummaryMap: make(map[string]m.ClusterDeployMetrics), WQ: queue,
		AppdController: controller, PendingCache: []string{}, FailedCache: make(map[string]m.AttachStatus), Logger: l}
	cm.SubscribeToInstrumentationUpdates(dw.uninstrument)
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

	if !cache.WaitForCacheSync(stopCh, dw.HasSynced) {
		dw.Logger.Errorf("Timed out waiting for deployment caches to sync")
	}
	dw.Logger.Infof("Deployment Cache synchronized. Starting the processing...")

	wg.Add(1)
	go dw.startMetricsWorker(stopCh)

	wg.Add(1)
	go dw.startEventQueueWorker(stopCh)

	//instrumentation check timer
	bag := (*dw.ConfigManager).Get()
	uninstrumentTimer := time.NewTimer(time.Second * time.Duration(bag.SnapshotSyncInterval))
	go func() {
		<-uninstrumentTimer.C
		dw.Logger.Info("Running initial check to validate instrumentation")
		dw.uninstrument()
	}()

	<-stopCh
}

func (dw *DeployWorker) HasSynced() bool {
	return dw.informer.HasSynced()
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
	dw.Logger.Debugf("Added Deployment: %s\n", deployObj.Name)

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
	dw.Logger.Debugf("Deleted Deployment: %s\n", deployObj.Name)
	//clean caches
	utils.RemoveFromSlice(utils.GetDeployKey(deployObj), dw.PendingCache)
	delete(dw.FailedCache, utils.GetDeployKey(deployObj))
}

func (dw *DeployWorker) onUpdateDeployment(objOld interface{}, objNew interface{}) {
	deployObj := objNew.(*appsv1.Deployment)
	if !dw.qualifies(deployObj) {
		return
	}
	dw.Logger.Infof("Deployment %s changed\n", deployObj.Name)

	deployRecord, _ := dw.processObject(deployObj, nil)
	dw.WQ.Add(&deployRecord)

	init, biq, agentRequests := dw.shouldUpdate(deployObj)
	if init || biq {
		dw.Logger.Debugf("Deployment update is required. Init: %t. BiQ: %t\n", init, biq)
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
		pw.Logger.Infof("Flushing the queue of %d deployment records\n", count)
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
			pw.Logger.Info("Deployment Queue shut down")
		}
		if count == 0 || len(objList) >= bag.EventAPILimit {
			pw.Logger.Debugf("Sending %d deployment records to AppD events API\n", len(objList))
			pw.postDeployRecords(&objList)
			pw.AppdController.StopBT(bth)
			return
		}
	}
	pw.AppdController.StopBT(bth)
}

func (pw *DeployWorker) postDeployRecords(objList *[]m.DeploySchema) {
	bag := (*pw.ConfigManager).Get()
	rc := app.NewRestClient(bag, pw.Logger)

	schemaDefObj := m.NewDeploySchemaDefWrapper()

	err := rc.EnsureSchema(bag.DeploySchemaName, &schemaDefObj)
	if err != nil {
		pw.Logger.Errorf("Issues when ensuring %s schema. %v\n", bag.DeploySchemaName, err)
	} else {
		data, err := json.Marshal(objList)
		if err != nil {
			pw.Logger.Errorf("Problems when serializing array of deployment schemas. %v", err)
		}
		rc.PostAppDEvents(bag.DeploySchemaName, data)
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

	count := 0
	for _, obj := range pw.informer.GetStore().List() {
		deployObject := obj.(*appsv1.Deployment)
		if !pw.qualifies(deployObject) {
			continue
		}
		deploySchema, _ := pw.processObject(deployObject, nil)
		pw.summarize(&deploySchema)
		count++
	}

	if count == 0 {
		bag := (*pw.ConfigManager).Get()
		pw.SummaryMap[m.ALL] = m.NewClusterDeployMetrics(bag, m.ALL)
	}

	ml := pw.builAppDMetricsList()

	pw.Logger.Infof("Ready to push %d Deployment metrics\n", len(ml.Items))

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
	deployObject.DeploymentType = m.DEPLOYMENT_TYPE_DEPLOYMENT

	if d.ClusterName != "" {
		deployObject.ClusterName = d.ClusterName
	} else {
		deployObject.ClusterName = bag.AppName
	}

	var sb strings.Builder

	for k, l := range d.Labels {
		fmt.Fprintf(&sb, "%s:%s;", k, l)
	}

	ls := utils.TruncateString(sb.String(), app.MAX_FIELD_LENGTH)

	deployObject.Labels = ls
	sb.Reset()

	for k, l := range d.Annotations {
		fmt.Fprintf(&sb, "%s:%s;", k, l)
	}

	as := utils.TruncateString(sb.String(), app.MAX_FIELD_LENGTH)

	deployObject.Annotations = as
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

	if d.Spec.Strategy.RollingUpdate != nil && d.Spec.Strategy.RollingUpdate.MaxSurge != nil {
		deployObject.MaxSurge = d.Spec.Strategy.RollingUpdate.MaxSurge.StrVal
	}
	if d.Spec.Strategy.RollingUpdate != nil && d.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
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

	return instr.ShouldInstrumentDeployment(deployObj, bag, &dw.PendingCache, &dw.FailedCache, dw.Logger)
}

func (dw *DeployWorker) updateDeployment(deployObj *appsv1.Deployment, init bool, biq bool, agentRequests *m.AgentRequestList) {
	bag := (*dw.ConfigManager).Get()
	if (!init && !biq) || agentRequests == nil {
		return
	}

	dw.PendingCache = append(dw.PendingCache, utils.GetDeployKey(deployObj))

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		bth := dw.AppdController.StartBT("DeploymentUpdate")
		dw.Logger.WithField("Name", deployObj.Name).Info("Started deployment update for instrumentation")
		deploymentsClient := dw.Client.AppsV1().Deployments(deployObj.Namespace)
		result, getErr := deploymentsClient.Get(deployObj.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("Failed to get latest version of Deployment: %v", getErr)
		}

		//		if agentRequests.EnvRequired() {
		dw.Logger.Debug("Ensuring secret...")
		errSecret := dw.ensureSecret(deployObj.Namespace)
		if errSecret != nil {
			dw.Logger.Debugf("Failed to ensure secret in namespace %s: %v\n", deployObj.Namespace, errSecret)
			return fmt.Errorf("Failed to ensure secret in namespace %s: %v\n", deployObj.Namespace, errSecret)
		}
		//		}
		var biqContainerIndex int = -1
		initMap := []string{}
		for _, r := range agentRequests.Items {
			if r.InitContainerRequired() && !utils.StringInSlice(string(r.Tech), initMap) {
				initMap = append(initMap, string(r.Tech))
				dw.Logger.Debugf("Adding init container for %s agent...\n", r.Tech)
				agentAttachContainer := dw.buildInitContainer(&r)
				result.Spec.Template.Spec.InitContainers = append(result.Spec.Template.Spec.InitContainers, agentAttachContainer)
			}

			index, c := dw.findContainer(&r, deployObj)
			if c != nil {
				r.ContainerName = c.Name
				volName := fmt.Sprintf("%s-%s", bag.AgentMountName, string(r.Tech))
				volPath := instr.GetVolumePath(bag, &r)
				dw.updateSpec(index, result, volName, volPath, &r, r.EnvRequired())
				if r.BiQ == string(m.Sidecar) {
					biqContainerIndex = index
				}
			} else {
				return fmt.Errorf("Agent request refers to a non-existent container %s\n", r.ContainerName)
			}
		}

		if init {
			//annotate pod
			if result.Spec.Template.Annotations == nil {
				result.Spec.Template.Annotations = make(map[string]string)
			}
			result.Spec.Template.Annotations[instr.APPD_ATTACH_PENDING] = agentRequests.ToAnnotation()
			result.Spec.Template.Annotations[instr.APPD_ATTACH_DEPLOYMENT] = result.Name
			dw.Logger.Debugf("Pending annotation added: %s\n", result.Spec.Template.Annotations[instr.APPD_ATTACH_PENDING])

			//annotate deployment
			if result.Annotations == nil {
				result.Annotations = make(map[string]string)
			}
			result.Annotations[instr.DEPLOY_ANNOTATION] = time.Now().String()
		}

		if biq {

			dw.Logger.Debugf("Adding analytics agent container")
			//add analytics agent container
			analyticsContainer := dw.buildBiqSideCar(agentRequests.GetFirstRequest())
			//add volume and mounts for logging
			dw.updateSpec(biqContainerIndex, result, bag.AppLogMountName, bag.AppLogMountPath, agentRequests.GetFirstRequest(), false)
			result.Spec.Template.Spec.Containers = append(result.Spec.Template.Spec.Containers, analyticsContainer)

			//annotate that biq is instrumented
			if result.Annotations == nil {
				result.Annotations = make(map[string]string)
			}
			result.Annotations[instr.DEPLOY_BIQ_ANNOTATION] = time.Now().String()
		} else { //remote Biq
			if agentRequests.BiQRequested() {
				//ensure external name service in the namespace
				svcClient := dw.Client.CoreV1().Services(deployObj.Namespace)
				proxySvc, svcErr := svcClient.Get("analytics-proxy", metav1.GetOptions{})
				if svcErr != nil {
					if errors.IsNotFound(svcErr) {
						//create
						proxySvc.Name = "analytics-proxy"
						proxySvc.Namespace = deployObj.Namespace
						proxySvc.Spec.Type = "ExternalName"
						extName := agentRequests.GetFirstRequest().BiQ
						if !strings.Contains(extName, "svc.cluster.local") {
							extName = fmt.Sprintf("appd-infraviz.%s.svc.cluster.local", bag.AgentNamespace)
						}
						proxySvc.Spec.ExternalName = extName
						proxySvc.Spec.Ports = []v1.ServicePort{{Port: 9090, TargetPort: intstr.FromInt(9090)}}
						_, createErr := svcClient.Create(proxySvc)
						if createErr != nil {
							dw.Logger.Warn("Unable to create analytics-proxy service. The analytics transaction collection will not be possible")
						}
					} else {
						dw.Logger.Warn("Could not ensure that analytics-proxy service exists. The analytics transaction collection may not be possible")
					}
				}
			}
		}

		_, err := deploymentsClient.Update(result)
		dw.AppdController.StopBT(bth)
		return err
	})

	if retryErr != nil {
		dw.Logger.Errorf("Deployment update failed: %v\n", retryErr)
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
	} else {
		dw.Logger.WithField("Name", deployObj.Name).Info("Deployment update for instrumentation is complete")
	}

}

func (dw *DeployWorker) uninstrument() {
	bag := (*dw.ConfigManager).Get()
	dw.Logger.Info("Starting de-instrumentation check due to changes in instrumentation config")
	//loop through cached deployments
	count := 0
	for _, obj := range dw.informer.GetStore().List() {
		deployObject := obj.(*appsv1.Deployment)
		count++
		_, biqExists := deployObject.Annotations[instr.DEPLOY_BIQ_ANNOTATION]
		_, instrExists := deployObject.Annotations[instr.DEPLOY_ANNOTATION]
		v, ok := deployObject.Spec.Template.Annotations[instr.APPD_ATTACH_PENDING]
		if biqExists || instrExists || (ok && v != instr.APPD_ATTACH_FAILED) {
			newReq := instr.GetAgentRequestsForDeployment(deployObject, bag, dw.Logger)
			//if instrumented, but does not have any matching requests based on new rules or differs from
			//the original request
			//re-create request from annotaion
			oldRequests := m.FromAnnotation(v)
			if newReq == nil || !newReq.Equals(oldRequests) {
				dw.Logger.Infof("Deployment %s does not match the instrumentation rules any longer. Removing instrumentation...", deployObject.Name)
				// call ReverseDeploymentInstrumentation
				ReverseDeploymentInstrumentation(deployObject.Name, deployObject.Namespace, oldRequests, true, bag, dw.Logger, dw.Client)
			}
		} else {
			//check if needs to be instrumented
			init, biq, agentRequests := dw.shouldUpdate(deployObject)
			if init || biq {
				dw.Logger.Debugf("Deployment update is required. Init: %t. BiQ: %t\n", init, biq)
				dw.updateDeployment(deployObject, init, biq, agentRequests)
			}
		}
	}

	dw.Logger.Infof("De-instrumentation check complete. Scanned %d deployments", count)

}

func ReverseDeploymentInstrumentation(deployName string, namespace string, agentRequests *m.AgentRequestList, removeAnnotations bool, bag *m.AppDBag, l *log.Logger, client *kubernetes.Clientset) {
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deploymentsClient := client.AppsV1().Deployments(namespace)
		d, getErr := deploymentsClient.Get(deployName, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("Failed to get deployment object %s. Cannot reverse instrumentation: %v", deployName, getErr)
		}

		//strip init container with the agent
		index := -1
		for i, c := range d.Spec.Template.Spec.InitContainers {
			if c.Name == bag.AppDInitContainerName {
				index = i
				break
			}
		}
		if index >= 0 {
			d.Spec.Template.Spec.InitContainers[index] = d.Spec.Template.Spec.InitContainers[len(d.Spec.Template.Spec.InitContainers)-1]
			d.Spec.Template.Spec.InitContainers = d.Spec.Template.Spec.InitContainers[:len(d.Spec.Template.Spec.InitContainers)-1]
		}

		//strip analytics container
		indexA := -1
		for i, c := range d.Spec.Template.Spec.Containers {
			if c.Name == bag.AnalyticsAgentContainerName {
				indexA = i
				break
			}
		}
		if indexA >= 0 {
			d.Spec.Template.Spec.Containers[indexA] = d.Spec.Template.Spec.Containers[len(d.Spec.Template.Spec.Containers)-1]
			d.Spec.Template.Spec.Containers = d.Spec.Template.Spec.Containers[:len(d.Spec.Template.Spec.Containers)-1]
		}

		//strip env vars
		match := []string{"Dappdynamics", "javaagent"}
		stripEnvVars(d, agentRequests.GetFirstRequest().AgentEnvVar, match, l)
		stripEnvVars(d, "APPDYNAMICS_AGENT_ACCOUNT_ACCESS_KEY", []string{}, l)
		stripEnvVars(d, "APPDYNAMICS_NETVIZ_AGENT_HOST", []string{}, l)
		stripEnvVars(d, "APPDYNAMICS_NETVIZ_AGENT_PORT", []string{}, l)

		//strip volumes and volume mounts
		if d.Spec.Template.Spec.Volumes != nil {
			for _, r := range agentRequests.Items {
				volName := fmt.Sprintf("%s-%s", bag.AgentMountName, string(r.Tech))
				volIndex := -1
				for i, ev := range d.Spec.Template.Spec.Volumes {
					if ev.Name == volName {
						volIndex = i
					}
				}
				if volIndex >= 0 {
					d.Spec.Template.Spec.Volumes[volIndex] = d.Spec.Template.Spec.Volumes[len(d.Spec.Template.Spec.Volumes)-1]
					d.Spec.Template.Spec.Volumes = d.Spec.Template.Spec.Volumes[:len(d.Spec.Template.Spec.Volumes)-1]
				}

				analyticsIndex := -1
				for i, ev := range d.Spec.Template.Spec.Volumes {
					if ev.Name == bag.AppLogMountName {
						analyticsIndex = i
					}
				}

				if analyticsIndex >= 0 {
					d.Spec.Template.Spec.Volumes[analyticsIndex] = d.Spec.Template.Spec.Volumes[len(d.Spec.Template.Spec.Volumes)-1]
					d.Spec.Template.Spec.Volumes = d.Spec.Template.Spec.Volumes[:len(d.Spec.Template.Spec.Volumes)-1]
				}

				for containerIndex, c := range d.Spec.Template.Spec.Containers {
					if c.Name == r.ContainerName {
						//strip volume mounts
						if d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts != nil {
							volMountIndex := -1
							for i, vm := range d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts {
								if vm.Name == volName {
									volMountIndex = i
								}
							}
							if volMountIndex >= 0 {
								d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts[volMountIndex] = d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts[len(d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts)-1]
								d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts = d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts[:len(d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts)-1]
							}

							volMountBiq := -1
							for i, vm := range d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts {
								if vm.Name == bag.AppLogMountName {
									volMountBiq = i
								}
							}

							if volMountBiq >= 0 {
								d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts[volMountBiq] = d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts[len(d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts)-1]
								d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts = d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts[:len(d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts)-1]
							}
						}
					}
				}
			}
		}

		//validate clean removal (sanity check)
		removeVolume(bag.AppLogMountName, d, bag, l)
		javaVolume := fmt.Sprintf("%s-%s", bag.AgentMountName, m.Java)
		removeVolume(javaVolume, d, bag, l)
		dotNetVolume := fmt.Sprintf("%s-%s", bag.AgentMountName, m.DotNet)
		removeVolume(dotNetVolume, d, bag, l)

		if removeAnnotations == false {
			if d.Spec.Template.Annotations == nil {
				d.Spec.Template.Annotations = make(map[string]string)
			}
			d.Spec.Template.Annotations[instr.APPD_ATTACH_PENDING] = instr.APPD_ATTACH_FAILED
		} else {
			if d.Spec.Template.Annotations != nil {
				delete(d.Spec.Template.Annotations, instr.APPD_ATTACH_PENDING)
				delete(d.Spec.Template.Annotations, instr.APPD_ATTACH_DEPLOYMENT)
			}
			if d.Annotations != nil {
				delete(d.Annotations, instr.DEPLOY_ANNOTATION)
				delete(d.Annotations, instr.DEPLOY_BIQ_ANNOTATION)
			}
		}

		_, err := deploymentsClient.Update(d)
		if err != nil {
			l.Errorf("Unable to save deployment after reversing the instrumentation. %v", err)
		}
		return err
	})

	if retryErr != nil {
		l.Errorf("Failed to reverse instrumentation of the deployment %s: %v\n", deployName, retryErr)
	} else {
		l.Info("Successfully removed instrumentation from %s", deployName)
	}
}

func removeVolume(volName string, d *appsv1.Deployment, bag *m.AppDBag, l *log.Logger) {
	volumeIndex := -1
	for i, ev := range d.Spec.Template.Spec.Volumes {
		if ev.Name == volName {
			volumeIndex = i
		}
	}

	if volumeIndex >= 0 {
		d.Spec.Template.Spec.Volumes[volumeIndex] = d.Spec.Template.Spec.Volumes[len(d.Spec.Template.Spec.Volumes)-1]
		d.Spec.Template.Spec.Volumes = d.Spec.Template.Spec.Volumes[:len(d.Spec.Template.Spec.Volumes)-1]
	}

	for containerIndex, _ := range d.Spec.Template.Spec.Containers {
		//strip appd-volume mounts
		if d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts != nil {
			volMountIndex := -1
			for i, vm := range d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts {
				if vm.Name == volName {
					volMountIndex = i
				}
			}

			if volMountIndex >= 0 {
				d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts[volMountIndex] = d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts[len(d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts)-1]
				d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts = d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts[:len(d.Spec.Template.Spec.Containers[containerIndex].VolumeMounts)-1]
			}
		}
	}
}

func stripEnvVars(d *appsv1.Deployment, envVarName string, match []string, l *log.Logger) {
	//strip only what was added
	envVarIndex := -1
	containerIndex := -1
	for i, _ := range d.Spec.Template.Spec.Containers {
		for ii, ev := range d.Spec.Template.Spec.Containers[i].Env {
			if ev.Name == envVarName {
				envVarIndex = ii
				containerIndex = i
				break
			}
		}
	}
	if containerIndex >= 0 && envVarIndex >= 0 {
		l.Infof("Container env var %s found", envVarName)
		if len(match) > 0 {
			l.Infof("Matching strings %v provided", match)
			envVar := d.Spec.Template.Spec.Containers[containerIndex].Env[envVarIndex]
			ar := strings.Fields(envVar.Value)
			l.Infof("Current value split: %v", ar)
			noApm := []string{}
			for _, v := range ar {
				l.Infof("Checking segment: %s ", v)
				keep := true
				for _, ms := range match {
					if strings.Contains(v, ms) {
						keep = false
						break
					}
				}
				if keep {
					l.Infof("Matching string is not present. Keeping the segment %", v)
					noApm = append(noApm, v)
				}
			}
			if len(noApm) > 0 {
				l.Infof("NoAPM values detected: %v", noApm)
				originalValue := strings.Join(noApm, " ")
				l.Infof("Restoring the original value: %s", originalValue)
				d.Spec.Template.Spec.Containers[containerIndex].Env[envVarIndex].Value = originalValue
			} else {
				l.Infof("Only APM values detected. Deleting the variable")
				d.Spec.Template.Spec.Containers[containerIndex].Env[envVarIndex] = d.Spec.Template.Spec.Containers[containerIndex].Env[len(d.Spec.Template.Spec.Containers[containerIndex].Env)-1]
				d.Spec.Template.Spec.Containers[containerIndex].Env = d.Spec.Template.Spec.Containers[containerIndex].Env[:len(d.Spec.Template.Spec.Containers[containerIndex].Env)-1]
			}

		} else {
			l.Infof("No match. Deleting the entire variable")
			d.Spec.Template.Spec.Containers[containerIndex].Env[envVarIndex] = d.Spec.Template.Spec.Containers[containerIndex].Env[len(d.Spec.Template.Spec.Containers[containerIndex].Env)-1]
			d.Spec.Template.Spec.Containers[containerIndex].Env = d.Spec.Template.Spec.Containers[containerIndex].Env[:len(d.Spec.Template.Spec.Containers[containerIndex].Env)-1]
		}
	}
}

func (dw *DeployWorker) findContainer(agentRequest *m.AgentRequest, deployObj *appsv1.Deployment) (int, *v1.Container) {
	for index, c := range deployObj.Spec.Template.Spec.Containers {
		if c.Name == agentRequest.ContainerName {
			return index, &c
		}
	}
	fmt.Printf("Agent request refers to a non-existent container %s\n", agentRequest.ContainerName)
	return -1, nil
}

func (dw *DeployWorker) updateContainerEnv(ar *m.AgentRequest, deployObj *appsv1.Deployment, containerIndex int) {
	bag := (*dw.ConfigManager).Get()

	tech := ar.Tech
	if tech == m.DotNet {
		dw.Logger.Debugf("Requested env var update for DotNet container %s\n", deployObj.Spec.Template.Spec.Containers[containerIndex].Name)
		dotnetInjector := instr.NewDotNetInjector(bag, dw.AppdController)
		c := &(deployObj.Spec.Template.Spec.Containers[containerIndex])
		dotnetInjector.AddEnvVars(c, ar)
	}

	//if the method is MountEnv and tech is Java, build the env var for the agent
	dw.Logger.Infof("instrument method =  %s\n", ar.Method)
	if tech == m.Java && ar.Method == m.MountEnv {
		nodePrefix := bag.NodeNamePrefix
		if nodePrefix == "" {
			nodePrefix = ar.TierName
		}
		dw.Logger.Debugf("Requested env var update for java container %s\n", deployObj.Spec.Template.Spec.Containers[containerIndex].Name)
		optsExist := false
		volPath := instr.GetVolumePath(bag, ar)
		javaOptsVal := fmt.Sprintf(` -Dappdynamics.agent.accountAccessKey=$(APPDYNAMICS_AGENT_ACCOUNT_ACCESS_KEY) -Dappdynamics.controller.hostName=%s -Dappdynamics.controller.port=%d -Dappdynamics.controller.ssl.enabled=%t -Dappdynamics.agent.accountName=%s -Dappdynamics.agent.applicationName=%s -Dappdynamics.agent.tierName=%s -Dappdynamics.agent.reuse.nodeName=true -Dappdynamics.agent.reuse.nodeName.prefix=%s -javaagent:%s/javaagent.jar `,
			bag.ControllerUrl, bag.ControllerPort, bag.SSLEnabled, bag.Account, ar.AppName, ar.TierName, nodePrefix, volPath)
		if ar.IsBiQRemote() {
			javaOptsVal = fmt.Sprintf("%s -Dappdynamics.analytics.agent.url=%s/v2/sinks/bt", javaOptsVal, bag.AnalyticsAgentUrl)
		}

		if bag.AgentLogOverride != "" {
			javaOptsVal = fmt.Sprintf("%s -Dappdynamics.agent.logs.dir=%s", javaOptsVal, bag.AgentLogOverride)
		}

		if bag.NetVizPort > 0 {
			javaOptsVal = fmt.Sprintf("%s -Dappdynamics.socket.collection.bci.enable=true", javaOptsVal)
		}

		if bag.ProxyHost != "" {
			javaOptsVal = fmt.Sprintf("%s -Dappdynamics.http.proxyHost=%s -Dappdynamics.http.proxyPort=%s", javaOptsVal, bag.ProxyHost, bag.ProxyPort)
		}

		if bag.ProxyUser != "" {
			javaOptsVal = fmt.Sprintf("%s -Dappdynamics.http.proxyUser=%s -Dappdynamics.http.proxyPasswordFile=%s", javaOptsVal, bag.ProxyUser, bag.ProxyPass)
		}

		if deployObj.Spec.Template.Spec.Containers[containerIndex].Env == nil {
			deployObj.Spec.Template.Spec.Containers[containerIndex].Env = []v1.EnvVar{}
		} else {
			for i, ev := range deployObj.Spec.Template.Spec.Containers[containerIndex].Env {
				if ev.Name == ar.AgentEnvVar {
					deployObj.Spec.Template.Spec.Containers[containerIndex].Env[i].Value += javaOptsVal
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
			envJavaOpts := v1.EnvVar{Name: ar.AgentEnvVar, Value: javaOptsVal}
			deployObj.Spec.Template.Spec.Containers[containerIndex].Env = append(deployObj.Spec.Template.Spec.Containers[containerIndex].Env, envJavaOpts)
		}
		//prepend the secret ref
		deployObj.Spec.Template.Spec.Containers[containerIndex].Env = append([]v1.EnvVar{envVarKey}, deployObj.Spec.Template.Spec.Containers[containerIndex].Env...)

		//netviz
		if bag.NetVizPort > 0 {
			fieldRef := v1.ObjectFieldSelector{FieldPath: "status.hostIP"}
			netVizHostVar := v1.EnvVar{Name: "APPDYNAMICS_NETVIZ_AGENT_HOST",
				ValueFrom: &v1.EnvVarSource{FieldRef: &fieldRef}}
			deployObj.Spec.Template.Spec.Containers[containerIndex].Env = append(deployObj.Spec.Template.Spec.Containers[containerIndex].Env, netVizHostVar)

			netVizHostPort := v1.EnvVar{Name: "APPDYNAMICS_NETVIZ_AGENT_PORT",
				Value: strconv.Itoa(bag.NetVizPort)}
			deployObj.Spec.Template.Spec.Containers[containerIndex].Env = append(deployObj.Spec.Template.Spec.Containers[containerIndex].Env, netVizHostPort)
		}

	}

}

func (dw *DeployWorker) updateSpec(containerIndex int, result *appsv1.Deployment, volName string, volumePath string, agentRequest *m.AgentRequest, envUpdate bool) {

	//add shared volume to the deployment spec
	volumeExists := false
	if result.Spec.Template.Spec.Volumes != nil {
		for _, ev := range result.Spec.Template.Spec.Volumes {
			if ev.Name == volName {
				volumeExists = true
				break
			}
		}
	}
	if !volumeExists {
		vol := v1.Volume{Name: volName, VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		}}
		if result.Spec.Template.Spec.Volumes == nil || len(result.Spec.Template.Spec.Volumes) == 0 {
			result.Spec.Template.Spec.Volumes = []v1.Volume{vol}
		} else {
			result.Spec.Template.Spec.Volumes = append(result.Spec.Template.Spec.Volumes, vol)
		}
	}

	//add volume mount to the application container
	volumeMountExists := false
	if result.Spec.Template.Spec.Containers[containerIndex].VolumeMounts != nil {
		for _, vm := range result.Spec.Template.Spec.Containers[containerIndex].VolumeMounts {
			if vm.Name == volName {
				volumeMountExists = true
				break
			}
		}
	}

	if volumeMountExists == false {
		volumeMount := v1.VolumeMount{Name: volName, MountPath: volumePath}
		if result.Spec.Template.Spec.Containers[containerIndex].VolumeMounts == nil || len(result.Spec.Template.Spec.Containers[containerIndex].VolumeMounts) == 0 {
			result.Spec.Template.Spec.Containers[containerIndex].VolumeMounts = []v1.VolumeMount{volumeMount}
		} else {
			result.Spec.Template.Spec.Containers[containerIndex].VolumeMounts = append(result.Spec.Template.Spec.Containers[containerIndex].VolumeMounts, volumeMount)
		}
	}

	if envUpdate {
		dw.updateContainerEnv(agentRequest, result, containerIndex)
	}
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
		dw.Logger.Debugf("Secret %s does not exist in namespace %s. Creating...\n", secret.Name, ns)

		secret.StringData = make(map[string]string)
		secret.StringData[instr.APPD_SECRET_KEY_NAME] = bag.AccessKey

		_, err := dw.Client.CoreV1().Secrets(ns).Create(secret)
		fmt.Printf("Secret %s. %v\n", secret.Name, err)
		if err != nil {
			dw.Logger.Errorf("Unable to create secret. %v\n", err)
		}
		return err
	}
	dw.Logger.Debugf("Secret %s exists. No action required\n", instr.APPD_SECRET_NAME)

	return nil
}

func (dw *DeployWorker) buildInitContainer(agentrequest *m.AgentRequest) v1.Container {
	bag := (*dw.ConfigManager).Get()
	//volume mount for agent files
	volName := fmt.Sprintf("%s-%s", bag.AgentMountName, string(agentrequest.Tech))
	volumeMount := v1.VolumeMount{Name: volName, MountPath: bag.InitContainerDir}
	mounts := []v1.VolumeMount{volumeMount}

	cmd := []string{"cp", "-ra", fmt.Sprintf("%s/.", bag.AgentMountPath), bag.InitContainerDir}

	if bag.AgentUserOverride != "" {
		cmd = []string{"/bin/sh", "-c", fmt.Sprintf("cp -ra %s/. %s && chown -R %s %s ", bag.AgentMountPath, bag.InitContainerDir, bag.AgentUserOverride, bag.InitContainerDir)}
	}

	reqCPU, reqMem, limitCpu, limitMem := dw.getResourceLimits("init")

	resRequest := v1.ResourceList{}
	resRequest[v1.ResourceCPU] = resource.MustParse(reqCPU)
	resRequest[v1.ResourceMemory] = resource.MustParse(reqMem)

	resLimit := v1.ResourceList{}
	resLimit[v1.ResourceCPU] = resource.MustParse(limitCpu)
	resLimit[v1.ResourceMemory] = resource.MustParse(limitMem)
	reqs := v1.ResourceRequirements{Requests: resRequest, Limits: resLimit}

	cont := v1.Container{Name: (*dw.ConfigManager).Get().AppDInitContainerName, Image: agentrequest.GetAgentImageName((*dw.ConfigManager).Get()), ImagePullPolicy: v1.PullIfNotPresent,
		VolumeMounts: mounts, Command: cmd, Resources: reqs}

	return cont
}

func (dw *DeployWorker) buildBiqSideCar(agentrequest *m.AgentRequest) v1.Container {
	bag := (*dw.ConfigManager).Get()
	//key reference
	keyRef := v1.SecretKeySelector{Key: instr.APPD_SECRET_KEY_NAME, LocalObjectReference: v1.LocalObjectReference{
		Name: instr.APPD_SECRET_NAME}}
	envVar := v1.EnvVar{Name: "APPDYNAMICS_AGENT_ACCOUNT_ACCESS_KEY", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &keyRef}}
	env := []v1.EnvVar{envVar}
	env = append(env, v1.EnvVar{Name: "APPDYNAMICS_AGENT_APPLICATION_NAME", Value: fmt.Sprintf("%s", agentrequest.AppName)})
	env = append(env, v1.EnvVar{Name: "APPDYNAMICS_CONTROLLER_HOST_NAME", Value: fmt.Sprintf("%s", bag.ControllerUrl)})
	env = append(env, v1.EnvVar{Name: "APPDYNAMICS_CONTROLLER_PORT", Value: fmt.Sprintf("%d", bag.ControllerPort)})
	env = append(env, v1.EnvVar{Name: "APPDYNAMICS_CONTROLLER_SSL_ENABLED", Value: fmt.Sprintf("%t", bag.SSLEnabled)})
	env = append(env, v1.EnvVar{Name: "APPDYNAMICS_EVENTS_API_URL", Value: fmt.Sprintf("%s", bag.EventServiceUrl)})
	env = append(env, v1.EnvVar{Name: "APPDYNAMICS_AGENT_ACCOUNT_NAME", Value: fmt.Sprintf("%s", bag.Account)})
	env = append(env, v1.EnvVar{Name: "APPDYNAMICS_GLOBAL_ACCOUNT_NAME", Value: fmt.Sprintf("%s", bag.GlobalAccount)})
	if bag.ProxyHost != "" {
		env = append(env, v1.EnvVar{Name: "APPDYNAMICS_CONTROLLER_PROXY_HOST", Value: fmt.Sprintf("%s", bag.ProxyHost)})
	}

	if bag.ProxyPass != "" {
		env = append(env, v1.EnvVar{Name: "APPDYNAMICS_CONTROLLER_PROXY_PORT", Value: fmt.Sprintf("%s", bag.ProxyPort)})
	}

	//ports
	p := v1.ContainerPort{ContainerPort: 9090}
	ports := []v1.ContainerPort{p}

	//volume mount for logs
	volumeMount := v1.VolumeMount{Name: bag.AppLogMountName, MountPath: bag.AppLogMountPath}
	mounts := []v1.VolumeMount{volumeMount}

	reqCPU, reqMem, limitCpu, limitMem := dw.getResourceLimits("biq")

	resRequest := v1.ResourceList{}
	resRequest[v1.ResourceCPU] = resource.MustParse(reqCPU)
	resRequest[v1.ResourceMemory] = resource.MustParse(reqMem)

	resLimit := v1.ResourceList{}
	resLimit[v1.ResourceCPU] = resource.MustParse(limitCpu)
	resLimit[v1.ResourceMemory] = resource.MustParse(limitMem)
	reqs := v1.ResourceRequirements{Requests: resRequest, Limits: resLimit}

	cont := v1.Container{Name: (*dw.ConfigManager).Get().AnalyticsAgentContainerName, Image: (*dw.ConfigManager).Get().AnalyticsAgentImage, ImagePullPolicy: v1.PullIfNotPresent,
		Ports: ports, Env: env, VolumeMounts: mounts, Resources: reqs}

	return cont
}

func (dw *DeployWorker) getResourceLimits(containerType string) (string, string, string, string) {
	bag := (*dw.ConfigManager).Get()
	reqCPU := ""
	reqMem := ""

	if containerType == "biq" {
		reqCPU = bag.BiqRequestCpu
		reqMem = bag.BiqRequestMem
	}

	if containerType == "init" {
		reqCPU = bag.InitRequestCpu
		reqMem = bag.InitRequestMem
	}

	if reqCPU == "" {
		reqCPU = "0.1"
	}

	if reqMem == "" {
		reqMem = "600"
	}

	limitCpu := "0.2"
	limitCpuVal, e := strconv.ParseFloat(reqCPU, 32)
	if e == nil {
		limitCpu = fmt.Sprintf("%.1f", limitCpuVal*2)
	}

	limitMem := "800M"
	limitMemVal, eMem := strconv.ParseInt(reqMem, 10, 0)
	if eMem == nil {
		limitMem = fmt.Sprintf("%dM", int(limitMemVal*3/2))
	}

	reqMem = reqMem + "M"

	return reqCPU, reqMem, limitCpu, limitMem
}
