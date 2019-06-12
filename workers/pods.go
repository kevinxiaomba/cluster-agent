package workers

import (
	//	"bufio"
	"bytes"
	"encoding/json"
	"fmt"

	"strconv"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"

	"time"

	"github.com/appdynamics/cluster-agent/config"
	"github.com/appdynamics/cluster-agent/utils"
	w "github.com/appdynamics/cluster-agent/watchers"

	m "github.com/appdynamics/cluster-agent/models"
	"github.com/fatih/structs"
	"k8s.io/client-go/rest"

	instr "github.com/appdynamics/cluster-agent/instrumentation"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	app "github.com/appdynamics/cluster-agent/appd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	//	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"
)

type PodWorker struct {
	informer                cache.SharedIndexInformer
	Client                  *kubernetes.Clientset
	ConfManager             *config.MutexConfigManager
	Logger                  *log.Logger
	SummaryMap              map[string]m.ClusterPodMetrics
	AppSummaryMap           map[string]m.ClusterAppMetrics
	ContainerSummaryMap     map[string]m.ClusterContainerMetrics
	InstanceSummaryMap      map[string]m.ClusterInstanceMetrics
	WQ                      workqueue.RateLimitingInterface
	AppdController          *app.ControllerClient
	K8sConfig               *rest.Config
	PendingCache            []string
	FailedCache             map[string]m.AttachStatus
	ServiceCache            map[string]m.ServiceSchema
	EndpointCache           map[string]v1.Endpoints
	RQCache                 map[string]v1.ResourceQuota
	PVCCache                map[string]v1.PersistentVolumeClaim
	CMCache                 map[string]v1.ConfigMap
	SecretCache             map[string]v1.Secret
	NSCache                 map[string]m.NsSchema
	OwnerMap                map[string]string
	NamespaceMap            map[string]string
	ServiceWatcher          *w.ServiceWatcher
	EndpointWatcher         *w.EndpointWatcher
	PVCWatcher              *w.PVCWatcher
	RQWatcher               *w.RQWatcher
	CMWatcher               *w.ConfigWatcher
	NSWatcher               *w.NSWatcher
	SecretWatcher           *w.SecretWathcer
	DashboardCache          map[string]m.PodSchema
	DelayDashboard          bool
	PendingAssociationQueue map[string]m.AgentRetryRequest
	EventMap                map[string][]m.EventSchema
	NodesMonitor            *NodesWorker
	ContainerCache          map[string]m.ContainerSchema
}

var lockOwnerMap = sync.RWMutex{}
var lockDashboards = sync.RWMutex{}
var lockNS = sync.RWMutex{}
var lockAssociationQueue = sync.RWMutex{}

var lockEventLock = sync.RWMutex{}

var lockServices = sync.RWMutex{}
var lockEPs = sync.RWMutex{}
var lockNSMap = sync.RWMutex{}
var lockContainerCache = sync.RWMutex{}

func NewPodWorker(client *kubernetes.Clientset, cm *config.MutexConfigManager, controller *app.ControllerClient, config *rest.Config, l *log.Logger, nw *NodesWorker) PodWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	pw := PodWorker{Client: client, ConfManager: cm, Logger: l, SummaryMap: make(map[string]m.ClusterPodMetrics), AppSummaryMap: make(map[string]m.ClusterAppMetrics),
		ContainerSummaryMap: make(map[string]m.ClusterContainerMetrics), InstanceSummaryMap: make(map[string]m.ClusterInstanceMetrics),
		WQ: queue, AppdController: controller, K8sConfig: config, PendingCache: []string{}, FailedCache: make(map[string]m.AttachStatus),
		ServiceCache: make(map[string]m.ServiceSchema), EndpointCache: make(map[string]v1.Endpoints),
		OwnerMap: make(map[string]string), NamespaceMap: make(map[string]string), EventMap: make(map[string][]m.EventSchema),
		RQCache: make(map[string]v1.ResourceQuota), PVCCache: make(map[string]v1.PersistentVolumeClaim), PendingAssociationQueue: make(map[string]m.AgentRetryRequest),
		CMCache: make(map[string]v1.ConfigMap), SecretCache: make(map[string]v1.Secret), NSCache: make(map[string]m.NsSchema), DashboardCache: make(map[string]m.PodSchema),
		ContainerCache: make(map[string]m.ContainerSchema)}
	pw.initPodInformer(client)
	pw.ServiceWatcher = w.NewServiceWatcher(client, cm, &pw.ServiceCache, pw, l)
	pw.EndpointWatcher = w.NewEndpointWatcher(client, cm, &pw.EndpointCache, l)
	pw.PVCWatcher = w.NewPVCWatcher(client, cm, &pw.PVCCache, l)
	pw.RQWatcher = w.NewRQWatcher(client, cm, &pw.RQCache, pw.Logger)
	pw.CMWatcher = w.NewConfigWatcher(client, cm, &pw.CMCache, pw, l)
	pw.SecretWatcher = w.NewSecretWathcer(client, cm, &pw.SecretCache, pw, l)
	pw.NSWatcher = w.NewNSWatcher(client, cm, &pw.NSCache, l)
	pw.DelayDashboard = true
	pw.NodesMonitor = nw

	return pw
}

func (pw *PodWorker) initPodInformer(client *kubernetes.Clientset) cache.SharedIndexInformer {
	i := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.CoreV1().Pods(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.CoreV1().Pods(metav1.NamespaceAll).Watch(options)
			},
		},
		&v1.Pod{},
		0,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	i.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    pw.onNewPod,
		DeleteFunc: pw.onDeletePod,
		UpdateFunc: pw.onUpdatePod,
	})
	pw.informer = i

	return i
}

func (pw *PodWorker) qualifies(p *v1.Pod) bool {
	bag := (*pw.ConfManager).Get()
	return utils.NSQualifiesForMonitoring(p.Namespace, bag)
}

func (pw *PodWorker) onNewPod(obj interface{}) {
	podObj, ok := obj.(*v1.Pod)
	if !ok {
		return
	}
	if !pw.qualifies(podObj) {
		return
	}
	pw.Logger.Infof("Added Pod: %s %s\n", podObj.Namespace, podObj.Name)
	podRecord, _ := pw.processObject(podObj, nil)
	pw.tryDashboardCache(&podRecord)
	pw.WQ.Add(&podRecord)
	pw.checkForInstrumentation(podObj, &podRecord)
}

func (pw *PodWorker) instrument(statusChannel chan m.AttachStatus, podObj *v1.Pod, podSchema *m.PodSchema) {
	bag := (*pw.ConfManager).Get()
	pw.Logger.Infof("Attempting instrumentation %s...\n", podObj.Name)
	injector := instr.NewAgentInjector(pw.Client, pw.K8sConfig, bag, pw.AppdController, pw.Logger)
	injector.EnsureInstrumentation(statusChannel, podObj, podSchema)
}

func (pw *PodWorker) GetCachedPod(namespace string, podName string) (*v1.Pod, string, error) {
	key := utils.GetKey(namespace, podName)
	owner := ""
	if namespace == "" {
		key = podName
	}
	var podObj *v1.Pod = nil
	interf, ok, err := pw.informer.GetStore().GetByKey(key)
	if ok {
		podObj = interf.(*v1.Pod)
		owner = pw.getPodOwner(podObj)
	} else {
		//pod must be gone, get from cache
		lockOwnerMap.RLock()
		defer lockOwnerMap.RUnlock()
		owner, _ = pw.OwnerMap[key]
		return nil, owner, fmt.Errorf("Unable to find podschema for key %s", key)
	}

	return podObj, owner, err
}

func (pw *PodWorker) tryDashboardCache(podRecord *m.PodSchema) {
	bag := (*pw.ConfManager).Get()
	if utils.StringInSlice(podRecord.Owner, bag.DeploysToDashboard) {
		lockDashboards.Lock()
		defer lockDashboards.Unlock()
		pw.DashboardCache[utils.GetKey(podRecord.Namespace, podRecord.Owner)] = *podRecord
	}
}

func (pw *PodWorker) shouldUpdateDashboard(podRecord *m.PodSchema) bool {
	lockDashboards.RLock()
	defer lockDashboards.RUnlock()
	_, ok := pw.DashboardCache[utils.GetKey(podRecord.Namespace, podRecord.Owner)]
	return ok
}

func (pw *PodWorker) GetKnownNamespaces() map[string]string {
	lockNSMap.RLock()
	defer lockNSMap.RUnlock()
	m := make(map[string]string)
	for key, val := range pw.NamespaceMap {
		m[key] = val
	}
	return m
}

func (pw *PodWorker) GetKnownDeployments() map[string]string {
	lockOwnerMap.RLock()
	defer lockOwnerMap.RUnlock()

	m := make(map[string]string)
	for key, val := range pw.OwnerMap {
		m[key] = val
	}

	return m
}

func (pw *PodWorker) startEventQueueWorker(stopCh <-chan struct{}) {
	bag := (*pw.ConfManager).Get()
	pw.eventQueueTicker(stopCh, time.NewTicker(time.Duration(bag.SnapshotSyncInterval)*time.Second))
}

func (pw *PodWorker) flushQueue() {
	pw.updateServiceCache()
	bag := (*pw.ConfManager).Get()
	bth := pw.AppdController.StartBT("FlushPodDataQueue")
	count := pw.WQ.Len()
	if count > 0 {
		pw.Logger.Infof("Flushing the queue of %d Pod records\n", count)
	}
	if count == 0 {
		pw.AppdController.StopBT(bth)
		return
	}

	var objList []m.PodSchema
	var containerList []m.ContainerSchema
	var podRecord *m.PodSchema
	var ok bool = true

	for count >= 0 {

		podRecord, ok = pw.getNextQueueItem()
		count = count - 1
		if ok {
			objList = append(objList, *podRecord)
			for _, c := range podRecord.Containers {
				containerList = append(containerList, c)
			}
		} else {
			pw.Logger.Info("Queue shut down")
		}
		if count == 0 || len(objList) >= bag.EventAPILimit {
			pw.Logger.Debugf("Sending %d pod records to AppD events API\n", len(objList))
			pw.postPodRecords(&objList)
			pw.postContainerRecords(containerList)
			pw.AppdController.StopBT(bth)
			return
		}
	}
	pw.AppdController.StopBT(bth)
}

func (pw *PodWorker) postContainerRecords(objList []m.ContainerSchema) {
	bag := (*pw.ConfManager).Get()
	count := 0
	var containerList []m.ContainerSchema
	for _, c := range objList {
		containerList = append(containerList, c)
		if count == bag.EventAPILimit {
			pw.postContainerBatchRecords(&containerList)
			count = 0
			containerList = containerList[:0]
		}
		count++
	}
	if count > 0 {
		pw.postContainerBatchRecords(&containerList)
	}
}

func (pw *PodWorker) postContainerBatchRecords(objList *[]m.ContainerSchema) {
	bag := (*pw.ConfManager).Get()
	rc := app.NewRestClient(bag, pw.Logger)

	schemaDefObj := m.NewContainerSchemaDefWrapper()
	err := rc.EnsureSchema(bag.ContainerSchemaName, &schemaDefObj)
	if err != nil {
		pw.Logger.Errorf("Issues when ensuring %s schema. %v\n", bag.ContainerSchemaName, err)
	} else {
		data, err := json.Marshal(objList)
		if err != nil {
			pw.Logger.Errorf("Problems when serializing array of container schemas. %v", err)
		}
		rc.PostAppDEvents(bag.ContainerSchemaName, data)
	}

}

func (pw *PodWorker) postPodRecords(objList *[]m.PodSchema) {
	bag := (*pw.ConfManager).Get()
	rc := app.NewRestClient(bag, pw.Logger)
	schemaDefObj := m.NewPodSchemaDefWrapper()

	err := rc.EnsureSchema(bag.PodSchemaName, &schemaDefObj)
	if err != nil {
		pw.Logger.Errorf("Issues when ensuring %s schema. %v\n", bag.PodSchemaName, err)
	} else {
		data, err := json.Marshal(objList)
		if err != nil {
			pw.Logger.Errorf("Problems when serializing array of pod schemas. %v", err)
		}
		rc.PostAppDEvents(bag.PodSchemaName, data)
	}
}

func (pw *PodWorker) postEPBatchRecords(objList *[]m.EpSchema) {
	bag := (*pw.ConfManager).Get()

	rc := app.NewRestClient(bag, pw.Logger)

	schemaDefObj := m.NewEpSchemaDefWrapper()

	err := rc.EnsureSchema(bag.EpSchemaName, &schemaDefObj)
	if err != nil {
		pw.Logger.Errorf("Issues when ensuring %s schema. %v\n", bag.EpSchemaName, err)
	} else {
		data, err := json.Marshal(objList)
		if err != nil {
			pw.Logger.Errorf("Problems when serializing array of endpoint schemas. %v", err)
		}
		rc.PostAppDEvents(bag.EpSchemaName, data)
	}

}

func (pw *PodWorker) postNSBatchRecords(objList *[]m.NsSchema) {
	bag := (*pw.ConfManager).Get()

	rc := app.NewRestClient(bag, pw.Logger)

	schemaDefObj := m.NewNsSchemaDefWrapper()

	err := rc.EnsureSchema(bag.NsSchemaName, &schemaDefObj)
	if err != nil {
		pw.Logger.Errorf("Issues when ensuring %s schema. %v\n", bag.NsSchemaName, err)
	} else {
		data, err := json.Marshal(objList)
		if err != nil {
			pw.Logger.Errorf("Problems when serializing array of namespace schemas. %v", err)
		}
		rc.PostAppDEvents(bag.NsSchemaName, data)
	}

}

func (pw *PodWorker) getNextQueueItem() (*m.PodSchema, bool) {
	podRecord, quit := pw.WQ.Get()

	if quit {
		return podRecord.(*m.PodSchema), false
	}
	defer pw.WQ.Done(podRecord)
	pw.WQ.Forget(podRecord)

	return podRecord.(*m.PodSchema), true
}

func (pw *PodWorker) onDeletePod(obj interface{}) {
	podObj, ok := obj.(*v1.Pod)
	if !ok {
		return
	}
	if !pw.qualifies(podObj) {
		return
	}
	pw.Logger.Debugf("Deleted Pod: %s %s\n", podObj.Namespace, podObj.Name)
	podRecord, _ := pw.processObject(podObj, nil)
	pw.WQ.Add(&podRecord)
	pw.clearContainerCache(&podRecord)
	if podRecord.NodeID > 0 {
		//mark node as historial
		pw.AppdController.MarkNodeHistorical(podRecord.NodeID)
	}
}

func (pw *PodWorker) clearContainerCache(podObject *m.PodSchema) {
	lockContainerCache.Lock()
	defer lockContainerCache.Unlock()
	for _, c := range podObject.Containers {
		key := utils.GetContainerKey(podObject, &c)
		if _, exists := pw.ContainerCache[key]; exists {
			delete(pw.ContainerCache, key)
		}
	}
}

func (pw *PodWorker) onUpdatePod(objOld interface{}, objNew interface{}) {
	podObj, ok := objNew.(*v1.Pod)
	if !ok {
		return
	}
	if !pw.qualifies(podObj) {
		return
	}
	podOldObj := objOld.(*v1.Pod)

	podRecord, _ := pw.processObject(podObj, podOldObj)
	pw.tryDashboardCache(&podRecord)

	pw.Logger.Debugf("Pod changed: %s %s\n", podObj.Namespace, podObj.Name)
	pw.WQ.Add(&podRecord)

	pw.checkForInstrumentation(podObj, &podRecord)
}

func (pw *PodWorker) checkForInstrumentation(podObj *v1.Pod, podSchema *m.PodSchema) {
	//check if already instrumented
	pw.Logger.Infof("Checking updated pods %s for instrumentation...\n", podObj.Name)
	if instr.IsPodInstrumented(podObj, pw.Logger) {
		pw.PendingCache = utils.RemoveFromSlice(utils.GetPodKey(podObj), pw.PendingCache)
		pw.Logger.Infof("Pod %s already instrumented. Skipping...\n", podObj.Name)
		return
	}

	//check if exceeded the number of failures
	status, ok := pw.FailedCache[utils.GetPodKey(podObj)]
	if ok && status.Count >= instr.MAX_INSTRUMENTATION_ATTEMPTS {
		pw.PendingCache = utils.RemoveFromSlice(utils.GetPodKey(podObj), pw.PendingCache)
		pw.Logger.Infof("Pod %s exceeded the number of instrumentaition attempts. Skipping...\n", podObj.Name)
		return
	}

	if utils.IsPodRunnnig(podObj) {
		if utils.StringInSlice(utils.GetPodKey(podObj), pw.PendingCache) {
			pw.Logger.Infof("Pod %s is in already process of instrumentation. Skipping...\n", podObj.Name)
			return
		}

		pw.PendingCache = append(pw.PendingCache, utils.GetPodKey(podObj))
		statusChannel := make(chan m.AttachStatus)
		go pw.instrument(statusChannel, podObj, podSchema)
		st := <-statusChannel
		//		fmt.Printf("Received instrumentation status for pod %s with message %s\n", st.Key, st.LastMessage)
		if st.Success {
			if st.LastMessage != "" {
				EmitInstrumentationEvent(podObj, pw.Client, "AppDInstrumentation", st.LastMessage, v1.EventTypeWarning)
				if st.RetryAssociation {
					pw.Logger.Infof("Adding pod %s to Pending Association Queue", podObj.Name)
					retryObj := m.AgentRetryRequest{Pod: podObj, Request: st.Request}
					pw.addToAssociationQueue(&retryObj)
				}
			} else {
				//now that the pod is instrumented successfully, add it to the dashboard queue
				pw.tryDashboardCache(podSchema)
				EmitInstrumentationEvent(podObj, pw.Client, "AppDInstrumentation", "Successfully instrumented", v1.EventTypeNormal)
			}
			pw.PendingCache = utils.RemoveFromSlice(st.Key, pw.PendingCache)
		} else {
			existing, present := pw.FailedCache[st.Key]
			if present {
				st.Count = existing.Count + 1
			}
			pw.FailedCache[st.Key] = st
			if st.LastMessage != "" {
				EmitInstrumentationEvent(podObj, pw.Client, "AppDInstrumentation", st.LastMessage, v1.EventTypeWarning)
			}
		}
	}
}

func (pw PodWorker) Observe(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	defer pw.WQ.ShutDown()
	wg.Add(1)
	go pw.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, pw.HasSynced) {
		pw.Logger.Errorf("Timed out waiting for caches to sync")
	}
	pw.Logger.Infof("Pod Cache syncronized. Starting the processing...")

	wg.Add(1)
	go pw.startMetricsWorker(stopCh)

	wg.Add(1)
	go pw.startEventQueueWorker(stopCh)

	go pw.ServiceWatcher.WatchServices()
	go pw.EndpointWatcher.WatchEndpoints()
	go pw.RQWatcher.WatchResourceQuotas()

	go pw.PVCWatcher.WatchPVC()

	go pw.CMWatcher.WatchConfigs()

	go pw.SecretWatcher.WatchSecrets()

	go pw.NSWatcher.WatchNamespaces()

	go pw.startRetryQueueWorker(stopCh)

	//dashbard timer
	bag := (*pw.ConfManager).Get()
	dashTimer := time.NewTimer(time.Minute * time.Duration(bag.DashboardDelayMin))
	go func() {
		<-dashTimer.C
		pw.DelayDashboard = false
		pw.Logger.Debug("Dashboard generation delay lifted")
	}()

	<-stopCh
}

func (pw *PodWorker) HasSynced() bool {
	return pw.informer.HasSynced()
}

func (pw *PodWorker) startMetricsWorker(stopCh <-chan struct{}) {
	bag := (*pw.ConfManager).Get()
	pw.appMetricTicker(stopCh, time.NewTicker(time.Duration(bag.MetricsSyncInterval)*time.Second))
}

func (pw *PodWorker) appMetricTicker(stop <-chan struct{}, ticker *time.Ticker) {
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

func (pw *PodWorker) eventQueueTicker(stop <-chan struct{}, ticker *time.Ticker) {
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
func (pw PodWorker) CacheUpdated(namespace string) {
	pw.Logger.Debugf("CacheUpdated called for namespace %s\n", namespace)
	//queue up pod records of the namespace to update
	for _, obj := range pw.informer.GetStore().List() {
		podObject := obj.(*v1.Pod)
		if podObject.Namespace == namespace {
			podSchema, _ := pw.processObject(podObject, nil)
			pw.WQ.Add(&podSchema)
		}
	}
}

func (pw *PodWorker) buildAppDMetrics() {
	bag := (*pw.ConfManager).Get()
	bth := pw.AppdController.StartBT("PostPodMetrics")
	pw.SummaryMap = make(map[string]m.ClusterPodMetrics)
	pw.AppSummaryMap = make(map[string]m.ClusterAppMetrics)
	pw.ContainerSummaryMap = make(map[string]m.ClusterContainerMetrics)
	pw.InstanceSummaryMap = make(map[string]m.ClusterInstanceMetrics)
	pw.updateServiceCache()

	//get updated EP cache
	updatedEPs := pw.EndpointWatcher.GetUpdated()
	epList := []m.EpSchema{}
	for _, ep := range updatedEPs {
		epSchema := m.NewEpSchema(&ep)
		epSchema.ClusterName = bag.AppName
		epList = append(epList, epSchema)
	}

	dash := make(map[string]m.DashboardBag)
	pw.Logger.Debugf("Deployments configured for dashboarding: %s \n", bag.DeploysToDashboard)

	var clusterBag *m.DashboardBag = nil
	if !pw.DelayDashboard {
		//create cluster dashboard bag
		clusterBag = m.NewDashboardBagCluster()
	}
	var count int = 0
	for _, obj := range pw.informer.GetStore().List() {
		podObject := obj.(*v1.Pod)
		podSchema, changed := pw.processObject(podObject, nil)
		if changed {
			pw.WQ.Add(&podSchema)
		}
		pw.summarize(&podSchema)
		//endpoints
		for _, ep := range epList {
			ep.MatchPod(&podSchema)
		}
		//heat map
		heatNode := m.NewHeatNode(podSchema)
		if clusterBag != nil {
			heatNode.Events = pw.GetPodEvents(&podSchema)
			heatNode.Containers = pw.GetPodUtilization(&podSchema)
			clusterBag.AddNode(&heatNode)
		}
		count++
		//should build the dashboard
		if pw.shouldUpdateDashboard(&podSchema) {
			key := utils.GetKey(podSchema.Namespace, podSchema.Owner)
			dashBag, ok := dash[key]
			if !ok {
				dashBag = m.NewDashboardBagTier(podSchema.Namespace, podSchema.Owner, []m.PodSchema{})
				dashBag.AppName = podSchema.AppName
				dashBag.ClusterName = podSchema.ClusterName
				dashBag.ClusterAppID = bag.AppID
				dashBag.ClusterTierID = bag.TierID
				dashBag.ClusterNodeID = bag.NodeID
				dashBag.AppID = podSchema.AppID
				dashBag.TierID = podSchema.TierID
				dashBag.NodeID = podSchema.NodeID
			}
			dashBag.Pods = append(dashBag.Pods, podSchema)
			dash[key] = dashBag
		}
	}

	pw.processNamespaces()

	ml := pw.builAppDMetricsList()

	pw.postEPBatchRecords(&epList)

	pw.Logger.Infof("Ready to push %d Pod metrics\n", len(ml.Items))
	pw.AppdController.PostMetrics(ml)
	pw.AppdController.StopBT(bth)

	//delay dashboard generation
	if !pw.DelayDashboard && clusterBag != nil {
		clusterBag.ClusterName = bag.AppName
		clusterBag.ClusterAppID = bag.AppID
		clusterBag.ClusterTierID = bag.TierID
		clusterBag.ClusterTierID = bag.TierID
		clusterBag.ClusterNodeID = bag.NodeID
		dash[bag.AppName] = *clusterBag
		pw.Logger.Debugf("Number of dashboards to be updated %d\n", len(dash))
		go pw.buildDashboards(dash)
	}
}

func (pw *PodWorker) processNamespaces() {
	summary, okSum := pw.SummaryMap[m.ALL]
	if okSum {
		lockNS.Lock()
		defer lockNS.Unlock()
		updated := []m.NsSchema{}
		summary.NamespaceCount = int64(len(pw.NSCache))
		for k, ns := range pw.NSCache {
			count := 0
			for _, rq := range pw.RQCache {
				if rq.Namespace == ns.Name {
					count++
				}
			}
			if count == 0 {
				summary.NamespaceNoQuotas++
			}
			if ns.Quotas != count {
				ns.Quotas = count
				updated = append(updated, ns)
				pw.NSCache[k] = ns
			}
		}
		if len(updated) > 0 {
			go pw.postNSBatchRecords(&updated)
		}
		pw.SummaryMap[m.ALL] = summary
	}
}

func (pw *PodWorker) updateServiceCache() {
	pw.ServiceWatcher.UpdateServiceCache()
}

func (pw *PodWorker) summarize(podObject *m.PodSchema) {
	bag := (*pw.ConfManager).Get()
	//global metrics
	summary, okSum := pw.SummaryMap[m.ALL]
	if !okSum {
		summary = m.NewClusterPodMetrics(bag, m.ALL, m.ALL)
		summary.ServiceCount = int64(len(pw.ServiceCache))

		lockServices.RLock()
		for _, svc := range pw.ServiceCache {
			if svc.HasExternalService {
				summary.ExtServiceCount++
			}
		}
		lockServices.RUnlock()

		lockEPs.RLock()
		for _, ep := range pw.EndpointCache {
			summary.EndpointCount++
			ready, notready, orphan := m.GetEPStats(&ep)
			summary.EPReadyCount += int64(ready)
			summary.EPNotReadyCount += int64(notready)
			if orphan {
				summary.OrphanEndpoint++
			}
		}
		lockEPs.RUnlock()

		pw.SummaryMap[m.ALL] = summary
	}

	//node metrics
	summaryNode, okNode := pw.SummaryMap[podObject.NodeName]
	if !okNode {
		summaryNode = m.NewClusterPodMetrics(bag, m.ALL, podObject.NodeName)
		pw.SummaryMap[podObject.NodeName] = summaryNode
	}

	// namespace metrics
	summaryNS, okNS := pw.SummaryMap[podObject.Namespace]
	if !okNS {
		summaryNS = m.NewClusterPodMetrics(bag, podObject.Namespace, m.ALL)

		lockServices.RLock()
		for _, svc := range pw.ServiceCache {
			if svc.Namespace == podObject.Namespace {
				summaryNS.ServiceCount++
				if svc.HasExternalService {
					summaryNS.ExtServiceCount++
				}
			}
		}
		lockServices.RUnlock()

		for _, ep := range pw.EndpointCache {
			if ep.Namespace == podObject.Namespace {
				summaryNS.EndpointCount++
				ready, notready, orphan := m.GetEPStats(&ep)
				summaryNS.EPReadyCount += int64(ready)
				summaryNS.EPNotReadyCount += int64(notready)
				if orphan {
					summaryNS.OrphanEndpoint++
				}
			}
		}
		pw.SummaryMap[podObject.Namespace] = summaryNS
	}

	//app/tier metrics
	summaryApp, okApp := pw.AppSummaryMap[podObject.Owner]
	if !okApp {
		summaryApp = m.NewClusterAppMetrics(bag, podObject)
		pw.AppSummaryMap[podObject.Owner] = summaryApp
		summaryApp.ContainerCount = int64(podObject.ContainerCount)
		summaryApp.InitContainerCount = int64(podObject.InitContainerCount)

		//get quotas that apply to the Tier/Deployment
		var theQuota *m.RQSchemaObj = nil
		for _, rq := range pw.RQCache {
			rqSchema := m.NewRQ(&rq)
			if rqSchema.AppliesToPod(podObject) {
				theQuota = &rqSchema
			}
		}
		if theQuota != nil {
			theQuota.AddQuotaStatsToAppMetrics(&summaryApp)
			theQuota.AddQuotaStatsToNamespaceMetrics(&summaryNS)
			theQuota.IncrementQuotaStatsClusterMetrics(&summary)
		}
	}

	summary.PodCount++
	summaryNS.PodCount++
	summaryNode.PodCount++
	summaryApp.PodCount++

	summary.ContainerCount += int64(podObject.ContainerCount)
	summaryNS.ContainerCount += int64(podObject.ContainerCount)
	summaryNode.ContainerCount += int64(podObject.ContainerCount)

	if !podObject.LimitsDefined {
		summary.NoLimits++
		summaryNS.NoLimits++
		summaryNode.NoLimits++
		summaryApp.NoLimits++
	}

	summary.Privileged += int64(podObject.NumPrivileged)
	summaryNS.Privileged += int64(podObject.NumPrivileged)
	summaryNode.Privileged += int64(podObject.NumPrivileged)
	summaryApp.Privileged += int64(podObject.NumPrivileged)

	summary.NoLivenessProbe += int64(podObject.LiveProbes)
	summaryNS.NoLivenessProbe += int64(podObject.LiveProbes)
	summaryNode.NoLivenessProbe += int64(podObject.LiveProbes)
	summaryApp.NoLivenessProbe += int64(podObject.LiveProbes)

	summary.NoReadinessProbe += int64(podObject.ReadyProbes)
	summaryNS.NoReadinessProbe += int64(podObject.ReadyProbes)
	summaryNode.NoReadinessProbe += int64(podObject.ReadyProbes)
	summaryApp.NoReadinessProbe += int64(podObject.ReadyProbes)

	if podObject.MissingDependencies {
		summary.MissingDependencies++
		summaryNS.MissingDependencies++
		summaryApp.MissingDependencies++
	}

	if podObject.NoConnectivity {
		summary.NoConnectivity++
		summaryNS.NoConnectivity++
		summaryApp.NoConnectivity++
	}

	summary.LimitCpu += int64(podObject.CpuLimit)
	summaryNS.LimitCpu += int64(podObject.CpuLimit)
	summaryNode.LimitCpu += int64(podObject.CpuLimit)
	summaryApp.LimitCpu += int64(podObject.CpuLimit)

	summary.LimitMemory += int64(podObject.MemLimit)
	summaryNS.LimitMemory += int64(podObject.MemLimit)
	summaryNode.LimitMemory += int64(podObject.MemLimit)
	summaryApp.LimitMemory += int64(podObject.MemLimit)

	summary.RequestCpu += int64(podObject.CpuRequest)
	summaryNS.RequestCpu += int64(podObject.CpuRequest)
	summaryNode.RequestCpu += int64(podObject.CpuRequest)
	summaryApp.RequestCpu += int64(podObject.CpuRequest)

	summary.RequestMemory += int64(podObject.MemRequest)
	summaryNS.RequestMemory += int64(podObject.MemRequest)
	summaryNode.RequestMemory += int64(podObject.MemRequest)
	summaryApp.RequestMemory += int64(podObject.MemRequest)

	summary.InitContainerCount += int64(podObject.InitContainerCount)
	summaryNS.InitContainerCount += int64(podObject.InitContainerCount)
	summaryNode.InitContainerCount += int64(podObject.InitContainerCount)

	switch podObject.Phase {
	case "Pending":
		summary.PodPending++
		summaryNS.PodPending++
		summaryNode.PodPending++
		summaryApp.PodPending++
		break
	case "Failed":
		summary.PodFailed++
		summaryNS.PodFailed++
		summaryNode.PodFailed++
		summaryApp.PodFailed++
		break
	case "Running":
		summary.PodRunning++
		summaryNS.PodRunning++
		summaryNode.PodRunning++
		summaryApp.PodRunning++
		break
	}

	if podObject.Reason != "" && podObject.Reason == "Evicted" {
		summary.Evictions++
		summaryNS.Evictions++
		summaryNode.Evictions++
		summaryApp.Evictions++
	}

	//Pending phase duration
	if summary.PodCount > 0 {
		summary.PendingTime = (summary.PendingTime + podObject.PendingTime) / summary.PodCount
		summary.UpTime = (summary.UpTime + podObject.UpTimeMillis) / summary.PodCount
	}

	if summaryNS.PodCount > 0 {
		summaryNS.PendingTime = (summaryNS.PendingTime + podObject.PendingTime) / summaryNS.PodCount
		summaryNS.UpTime = (summaryNS.UpTime + podObject.UpTimeMillis) / summaryNS.PodCount
	}

	if summaryNode.PodCount > 0 {
		summaryNode.PendingTime = (summaryNode.PendingTime + podObject.PendingTime) / summaryNode.PodCount
		summaryNode.UpTime = (summaryNode.UpTime + podObject.UpTimeMillis) / summaryNode.PodCount
	}

	if summaryApp.PodCount > 0 {
		summaryApp.PendingTime = (summaryApp.PendingTime + podObject.PendingTime) / summaryApp.PodCount
		summaryApp.UpTime = (summaryApp.UpTime + podObject.UpTimeMillis) / summaryApp.PodCount
	}

	summary.PodRestarts += int64(podObject.PodRestarts)
	summaryNS.PodRestarts += int64(podObject.PodRestarts)
	summaryNode.PodRestarts += int64(podObject.PodRestarts)
	summaryApp.PodRestarts += int64(podObject.PodRestarts)

	summary.UseCpu += podObject.CpuUse
	summary.UseMemory += podObject.MemUse

	summaryNS.UseCpu += podObject.CpuUse
	summaryNS.UseMemory += podObject.MemUse

	summaryApp.UseCpu += podObject.CpuUse
	summaryApp.UseMemory += podObject.MemUse

	//consumption
	if summary.PodCount > 0 {
		summary.ConsumptionCpu = (summary.ConsumptionCpu + int64(podObject.ConsumptionCpu)) / summary.PodCount
		summary.ConsumptionMem = (summary.ConsumptionMem + int64(podObject.ConsumptionMem)) / summary.PodCount
	}
	if summaryNS.PodCount > 0 {
		summaryNS.ConsumptionCpu = (summaryNS.ConsumptionCpu + int64(podObject.ConsumptionCpu)) / summaryNS.PodCount
		summaryNS.ConsumptionMem = (summaryNS.ConsumptionMem + int64(podObject.ConsumptionMem)) / summaryNS.PodCount
	}

	if summaryNode.PodCount > 0 {
		summaryNode.ConsumptionCpu = (summaryNode.ConsumptionCpu + int64(podObject.ConsumptionCpu)) / summaryNode.PodCount
		summaryNode.ConsumptionMem = (summaryNode.ConsumptionMem + int64(podObject.ConsumptionMem)) / summaryNode.PodCount
	}

	if summaryApp.PodCount > 0 {
		summaryApp.ConsumptionCpu = (summaryApp.ConsumptionCpu + int64(podObject.ConsumptionCpu)) / summaryApp.PodCount
		summaryApp.ConsumptionMem = (summaryApp.ConsumptionMem + int64(podObject.ConsumptionMem)) / summaryApp.PodCount
	}

	//app summary for containers
	if !podObject.IsEvicted {
		for _, c := range podObject.Containers {
			key := fmt.Sprintf("%s_%s_%s", podObject.Namespace, podObject.Owner, c.Name)
			summaryContainer, okCont := pw.ContainerSummaryMap[key]
			if !okCont {
				summaryContainer = m.NewClusterContainerMetrics(bag, podObject.Namespace, podObject.Owner, c.Name)
				summaryContainer.LimitCpu = c.CpuLimit
				summaryContainer.LimitMemory = c.MemLimit
				summaryContainer.RequestCpu = c.CpuRequest
				summaryContainer.RequestMemory = c.MemRequest

				summaryContainer.NoLivenessProbe = int64(c.LiveProbes)
				summaryContainer.NoReadinessProbe = int64(c.ReadyProbes)

				summaryContainer.PodStorageRequest = c.PodStorageRequest
				summaryContainer.PodStorageLimit = c.PodStorageLimit
				summaryContainer.StorageRequest = c.StorageRequest
				summaryContainer.StorageCapacity = c.StorageCapacity

				summary.PodStorageLimit += c.PodStorageLimit
				summary.PodStorageRequest += c.PodStorageRequest
				summary.StorageCapacity += c.StorageCapacity
				summary.StorageRequest += c.StorageRequest

				summaryNS.PodStorageLimit += c.PodStorageLimit
				summaryNS.PodStorageRequest += c.PodStorageRequest
				summaryNS.StorageCapacity += c.StorageCapacity
				summaryNS.StorageRequest += c.StorageRequest

				if !c.LimitsDefined {
					summaryContainer.NoLimits = int64(1)
				}
			}
			summaryContainer.Restarts += int64(c.Restarts)
			pw.ContainerSummaryMap[key] = summaryContainer

			summaryInstance, instanceKey := pw.nextContainerInstance(podObject, c)
			if summaryInstance != nil {
				summaryInstance.UseCpu = c.CpuUse
				summaryInstance.UseMemory = c.MemUse
				summaryInstance.Restarts = int64(c.Restarts)
				pw.InstanceSummaryMap[instanceKey] = *summaryInstance
			}
		}
	}

	pw.AppSummaryMap[podObject.Owner] = summaryApp
	pw.SummaryMap[m.ALL] = summary
	pw.SummaryMap[podObject.Namespace] = summaryNS
	pw.SummaryMap[podObject.NodeName] = summaryNode
}

func (pw *PodWorker) nextContainerInstance(podObject *m.PodSchema, c m.ContainerSchema) (*m.ClusterInstanceMetrics, string) {
	bag := (*pw.ConfManager).Get()
	var summaryInstance m.ClusterInstanceMetrics
	var instanceKey string

	instanceKey = fmt.Sprintf("%s_%s_%s_%s", podObject.Namespace, podObject.Owner, c.Name, podObject.Name)
	_, okInstance := pw.InstanceSummaryMap[instanceKey]
	if !okInstance {
		summaryInstance = m.NewClusterInstanceMetrics(bag, podObject, c.Name)
		pw.InstanceSummaryMap[instanceKey] = summaryInstance
	}

	return &summaryInstance, instanceKey
}

func (pw *PodWorker) getPodOwner(p *v1.Pod) string {
	owner := ""
	for k, v := range p.Labels {
		if strings.ToLower(k) == "name" {
			owner = v
		}
	}

	if owner == "" && len(p.OwnerReferences) > 0 {
		owner = p.OwnerReferences[0].Name
	}

	if owner == "" {
		owner = p.Name
	}
	return owner
}

func (pw *PodWorker) processObject(p *v1.Pod, old *v1.Pod) (m.PodSchema, bool) {
	bag := (*pw.ConfManager).Get()
	changed := false

	if p != nil && old != nil {
		if p.Status.Reason != old.Status.Reason ||
			p.DeletionTimestamp == nil && old.DeletionTimestamp != nil ||
			len(p.Labels) != len(old.Labels) ||
			len(p.GetAnnotations()) != len(old.GetAnnotations()) {
			changed = true
		}
	}

	podObject := m.NewPodObj()
	var sb strings.Builder
	for k, v := range p.Labels {
		fmt.Fprintf(&sb, "%s:%s;", k, v)
		if k == "name" {
			podObject.Owner = v
		}
		if k == bag.AppDAppLabel {
			podObject.AppName = v
		}

		if k == bag.AppDTierLabel {
			podObject.TierName = v
		}
		if old != nil {
			if _, exists := old.Labels[k]; !exists {
				changed = false
			}
		}
	}

	if podObject.Owner == "" && len(p.OwnerReferences) > 0 {
		podObject.Owner = p.OwnerReferences[0].Name
	}

	lockOwnerMap.Lock()
	defer lockOwnerMap.Unlock()
	pw.OwnerMap[utils.GetPodKey(p)] = podObject.Owner

	podObject.Reason = p.Status.Reason
	if podObject.Reason != "" && podObject.Reason == "Evicted" {
		podObject.IsEvicted = true
	}

	podObject.Labels = sb.String()
	sb.Reset()

	podObject.Name = p.Name
	podObject.Namespace = p.Namespace
	lockNSMap.Lock()
	pw.NamespaceMap[p.Namespace] = p.Namespace
	lockNSMap.Unlock()
	podObject.NodeName = p.Spec.NodeName

	if p.ClusterName != "" {
		podObject.ClusterName = p.ClusterName
	} else {
		podObject.ClusterName = bag.AppName
	}

	if p.Status.StartTime != nil {
		podObject.StartTime = p.Status.StartTime.Time
		podObject.StartTimeMillis = podObject.StartTime.UnixNano() / 1000000
	}

	if p.DeletionTimestamp != nil {
		podObject.TerminationTime = &p.DeletionTimestamp.Time
		podObject.TerminationTimeMillis = podObject.TerminationTime.UnixNano() / 1000000
	}

	for k, v := range p.GetAnnotations() {
		fmt.Fprintf(&sb, "%s:%s;", k, v)
		if k == instr.APPD_APPID {
			appID, err := strconv.Atoi(v)
			if err == nil {
				podObject.AppID = appID
			} else {
				pw.Logger.Errorf("Unable to parse App ID value %s from the annotations", v)
			}
		}

		if k == instr.APPD_TIERID {
			tierID, err := strconv.Atoi(v)
			if err == nil {
				podObject.TierID = tierID
			} else {
				pw.Logger.Errorf("Unable to parse Tier ID value %s from the annotations", v)
			}
		}

		if k == instr.APPD_NODEID {
			nodeID, err := strconv.Atoi(v)
			if err == nil {
				podObject.NodeID = nodeID
			} else {
				pw.Logger.Errorf("Unable to parse Node ID value %s from the annotations", v)
			}
		}

		if k == instr.APPD_NODENAME {
			podObject.APMNodeName = v
		}

		if k == instr.APPD_AGENT_ID {
			podObject.AppID = bag.AppID
			podObject.TierID = bag.TierID
			podObject.NodeID = bag.NodeID
		}

		if old != nil {
			if _, exists := old.GetAnnotations()[k]; !exists {
				changed = false
			}
		}
	}

	ps := utils.TruncateString(sb.String(), app.MAX_FIELD_LENGTH)

	podObject.Annotations = ps

	podObject.HostIP = p.Status.HostIP
	podObject.PodIP = p.Status.PodIP

	if !podObject.IsEvicted {
		//find service
		podLabels := labels.Set(p.Labels)
		//check if service exists and routes to the ports
		lockServices.RLock()
		for _, svcSchema := range pw.ServiceCache {
			if svcSchema.MatchesPod(p) {
				podObject.Services = append(podObject.Services, svcSchema)
			}
		}
		lockServices.RUnlock()

		for _, ep := range pw.EndpointCache {
			epSelector := labels.SelectorFromSet(ep.Labels)
			if ep.Namespace == podObject.Namespace && epSelector.Matches(podLabels) {
				podObject.Endpoints = append(podObject.Endpoints, ep)
			}
		}

		podObject.InitContainerCount = len(p.Spec.InitContainers)
		podObject.ContainerCount = len(p.Spec.Containers)

		if podObject.ContainerCount > 0 {
			podObject.Containers = make(map[string]m.ContainerSchema, podObject.ContainerCount)
		}

		if podObject.InitContainerCount > 0 {
			podObject.InitContainers = make(map[string]m.ContainerSchema, podObject.InitContainerCount)
		}

		for _, c := range p.Spec.Containers {
			containerObj, limitsDefined := pw.processContainer(p, &podObject, c, false)

			podObject.Containers[c.Name] = containerObj
			podObject.LimitsDefined = limitsDefined
			podObject.MissingDependencies = containerObj.HasMissingDependencies()
			podObject.NoConnectivity = containerObj.NoConnectivity()
			if podObject.MissingDependencies || podObject.NoConnectivity {
				changed = true
			}
		}

		for _, c := range p.Spec.InitContainers {
			containerObj, _ := pw.processContainer(p, &podObject, c, true)
			podObject.InitContainers[c.Name] = containerObj
		}
	}

	if p.Spec.Priority != nil {
		podObject.Priority = *p.Spec.Priority
	}
	podObject.ServiceAccountName = p.Spec.ServiceAccountName
	if p.Spec.TerminationGracePeriodSeconds != nil {
		podObject.TerminationGracePeriodSeconds = *p.Spec.TerminationGracePeriodSeconds
	}

	podObject.RestartPolicy = string(p.Spec.RestartPolicy)

	if p.Spec.Tolerations != nil {
		for i, t := range p.Spec.Tolerations {
			if i == 0 {
				podObject.Tolerations = fmt.Sprintf("%s;", t.String())
			} else {
				podObject.Tolerations = fmt.Sprintf("%s;%s", podObject.Tolerations, t.String())
			}
		}
	}

	if p.Spec.Affinity != nil {
		if p.Spec.Affinity.NodeAffinity != nil {
			if p.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
				var sb strings.Builder
				for _, pna := range p.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
					sb.WriteString(fmt.Sprintf("%s;", pna.String()))
				}
				podObject.NodeAffinityPreferred = sb.String()
			}

			if p.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil && p.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms != nil {
				var sb strings.Builder
				for _, term := range p.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
					if term.MatchExpressions != nil {
						for _, mex := range term.MatchExpressions {
							sb.WriteString(fmt.Sprintf("%s;", mex.String()))
						}
					}
					if term.MatchFields != nil {
						for _, mfld := range term.MatchExpressions {
							sb.WriteString(fmt.Sprintf("%s;", mfld.String()))
						}
					}
				}
				podObject.NodeAffinityRequired = sb.String()
			}

		}

		if p.Spec.Affinity.PodAffinity != nil {
			if p.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
				var sb strings.Builder
				for _, term := range p.Spec.Affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
					sb.WriteString(fmt.Sprintf("%d %s %s %s;", term.Weight, term.PodAffinityTerm.TopologyKey, term.PodAffinityTerm.LabelSelector, term.PodAffinityTerm.Namespaces))
				}
				podObject.PodAffinityPreferred = sb.String()
			}

			if p.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
				var sb strings.Builder
				for _, term := range p.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
					sb.WriteString(fmt.Sprintf("%s %s %s;", term.TopologyKey, term.LabelSelector, term.Namespaces))
				}
				podObject.PodAffinityRequired = sb.String()
			}
		}

		if p.Spec.Affinity.PodAntiAffinity != nil {
			if p.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
				var sb strings.Builder
				for _, term := range p.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
					sb.WriteString(fmt.Sprintf("%d %s %s %s;", term.Weight, term.PodAffinityTerm.TopologyKey, term.PodAffinityTerm.LabelSelector, term.PodAffinityTerm.Namespaces))
				}
				podObject.PodAntiAffinityPreferred = sb.String()
			}
			if p.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
				var sb strings.Builder
				for _, term := range p.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
					sb.WriteString(fmt.Sprintf("%s %s %s;", term.TopologyKey, term.LabelSelector, term.Namespaces))
				}
				podObject.PodAntiAffinityRequired = sb.String()
			}
		}
	}

	if p.DeletionTimestamp != nil {
		podObject.TerminationTime = &p.DeletionTimestamp.Time
	}

	podObject.Phase = string(p.Status.Phase)

	if !podObject.IsEvicted {
		var lastCondition *v1.PodCondition = nil
		maxTimeMillis := int64(0)
		if p.Status.Conditions != nil && len(p.Status.Conditions) > 0 {
			for _, cn := range p.Status.Conditions {
				last := cn.LastTransitionTime.UnixNano() / 1000000
				if last > maxTimeMillis {
					maxTimeMillis = last
					lastCondition = &cn
				}
				switch cn.Type {
				case v1.PodScheduled:
					//					fmt.Printf("Pod %s Scheduled %s. Time: %s Probe: %s\n", podObject.Name, string(cn.Status), cn.LastTransitionTime.Time, cn.LastProbeTime.Time)
					break

				case v1.PodReady:
					//					fmt.Printf("Pod %s Ready %s. Time: %s Probe: %s\n", podObject.Name, string(cn.Status), cn.LastTransitionTime.Time, cn.LastProbeTime.Time)

					break

				case v1.PodReasonUnschedulable:
					//					fmt.Printf("Pod %s Unscheduable %s. Time: %s Probe: %s\n", podObject.Name, string(cn.Status), cn.LastTransitionTime.Time, cn.LastProbeTime.Time)
					break

				case v1.PodInitialized:
					//					fmt.Printf("Pod %s Initialized %s. Time: %s Probe: %s\n", podObject.Name, string(cn.Status), cn.LastTransitionTime.Time, cn.LastProbeTime.Time)
					break

				case v1.ContainersReady:
					//					fmt.Printf("Pod %s Initialized %s. Time: %s Probe: %s\n", podObject.Name, string(cn.Status), cn.LastTransitionTime.Time, cn.LastProbeTime.Time)
					break
				}

			}

			if lastCondition != nil {
				//				fmt.Printf("Last condition: Type=%s, Status=%s, Time:%s, Probe:%s\n", string(lastCondition.Type), string(lastCondition.Status), lastCondition.LastTransitionTime.Time, lastCondition.LastProbeTime.Time)

				podObject.ReasonCondition = fmt.Sprintf("%s. %s", lastCondition.Reason, lastCondition.Message)
				podObject.StatusCondition = string(lastCondition.Status)
				podObject.TypeCondition = string(lastCondition.Type)
				podObject.LastTransitionTimeCondition = &lastCondition.LastTransitionTime.Time
			}
		}

		if p.Status.ContainerStatuses != nil {

			for _, st := range p.Status.ContainerStatuses {
				containerObj := findContainer(&podObject, st.Name)
				mod := pw.updateConatainerStatus(&podObject, containerObj, st)
				if mod {
					changed = true
				}
			}
		}

		if p.Status.InitContainerStatuses != nil {

			for _, st := range p.Status.InitContainerStatuses {
				containerObj := findInitContainer(&podObject, st.Name)
				pw.updateConatainerStatus(&podObject, containerObj, st)
			}
		}

		//determine start time
		var firstContainer *m.ContainerSchema = nil
		i := 0
		for _, c := range podObject.Containers {
			if i == 0 {
				firstContainer = &c
			}
			if c.StartTime != nil {
				podObject.RunningStartTime = c.StartTime
				podObject.RunningStartTimeMillis = podObject.RunningStartTime.UnixNano() / 1000000

				if c.LastTerminationTime != nil {
					podObject.BreakPointMillis = c.LastTerminationTime.UnixNano() / 1000000
				} else {
					podObject.BreakPointMillis = podObject.StartTimeMillis
				}
				break
			}
			i++
		}

		//check PendingTime
		if podObject.Phase != "Failed" && podObject.StartTimeMillis > 0 {
			if podObject.RunningStartTimeMillis > 0 {
				podObject.PendingTime = podObject.RunningStartTimeMillis - podObject.BreakPointMillis
				if podObject.TerminationTimeMillis > 0 {
					podObject.UpTimeMillis = podObject.TerminationTimeMillis - podObject.BreakPointMillis
				} else {
					now := time.Now().UnixNano() / 1000000
					podObject.UpTimeMillis = now - podObject.BreakPointMillis
				}
			} else {
				now := time.Now().UnixNano() / 1000000
				podObject.PendingTime = now - podObject.StartTimeMillis
			}
			pw.Logger.Debugf("Pending time: %s/%s %d. StartTimeMillis: %d, RunningStartTimeMillis: %d, TerminationTimeMillis: %d BreakPointMillis: %d\n", podObject.Namespace, podObject.Name, podObject.PendingTime, podObject.StartTimeMillis, podObject.RunningStartTimeMillis, podObject.TerminationTimeMillis, podObject.BreakPointMillis)
			if firstContainer != nil {
				firstContKey := utils.GetContainerKey(&podObject, firstContainer)
				oldSnapshot, exists := pw.ContainerCache[firstContKey]
				if exists {
					if oldSnapshot.PendingTime != podObject.PendingTime {
						changed = true
					}
				} else {
					changed = true
				}
				lockContainerCache.Lock()
				pw.ContainerCache[firstContKey] = *firstContainer
				lockContainerCache.Unlock()
			}
		}

		//metrics
		if utils.IsPodRunnnig(p) {
			podMetricsObj := pw.GetPodMetricsSingle(p, p.Namespace, p.Name)
			if podMetricsObj != nil {
				usageObj := podMetricsObj.GetPodUsage()

				podObject.CpuUse = usageObj.CPU
				podObject.MemUse = usageObj.Memory

				for cname, c := range podObject.Containers {
					pw.Logger.WithField("Name", cname).Debug("Processing container metrics:")
					contUsageObj := podMetricsObj.GetContainerUsage(cname)
					c.CpuUse = contUsageObj.CPU
					c.MemUse = contUsageObj.Memory
					pw.Logger.WithField("CPU", c.CpuUse).Debug("CPU usage")
					pw.Logger.WithField("Mem", c.MemUse).Debug("Mem usage")
					c.ConsumptionCpu, c.ConsumptionCpuString = pw.GetCpuConsumption(&podObject, &c)
					c.ConsumptionMem, c.ConsumptionMemString = pw.GetMemConsumption(&podObject, &c)
					pw.Logger.WithField("Percent", c.ConsumptionCpu).Debug("Cpu consumption")
					pw.Logger.WithField("Percent", c.ConsumptionMem).Debug("Memory consumption")
					if podObject.ConsumptionCpu < c.ConsumptionCpu {
						podObject.ConsumptionCpu = c.ConsumptionCpu
					}

					if podObject.ConsumptionMem < c.ConsumptionMem {
						podObject.ConsumptionMem = c.ConsumptionMem
					}

					podObject.Containers[cname] = c
					key := utils.GetContainerKey(&podObject, &c)
					lastSnapshot, ok := pw.ContainerCache[key]
					if ok {
						if lastSnapshot.CpuUse != c.CpuUse ||
							lastSnapshot.MemUse != c.MemUse ||
							lastSnapshot.ConsumptionCpu != c.ConsumptionCpu ||
							lastSnapshot.ConsumptionMem != c.ConsumptionMem {
							changed = true
						}
					} else {
						changed = true
					}
					lockContainerCache.Lock()
					pw.ContainerCache[key] = c
					lockContainerCache.Unlock()
				}

				for cname, c := range podObject.InitContainers {
					contUsageObj := podMetricsObj.GetContainerUsage(cname)
					c.CpuUse = contUsageObj.CPU
					c.MemUse = contUsageObj.Memory
					podObject.InitContainers[cname] = c
				}
			}
		}
	}

	return podObject, changed
}

func (pw PodWorker) updateConatainerStatus(podObject *m.PodSchema, containerObj *m.ContainerSchema, st v1.ContainerStatus) bool {
	changed := false

	bag := (*pw.ConfManager).Get()
	if containerObj != nil {
		containerObj.Restarts = st.RestartCount
		containerObj.Image = st.Image
	}

	podObject.PodRestarts += st.RestartCount

	if st.State.Waiting != nil {
		if containerObj != nil {
			containerObj.WaitReason = st.State.Waiting.Reason
		}
	}

	if st.State.Terminated != nil {
		if containerObj != nil {
			containerObj.TermReason = st.State.Terminated.Reason
			containerObj.TerminationTime = st.State.Terminated.FinishedAt.Time
			podObject.TerminationTime = &containerObj.TerminationTime
		}
	}

	if st.State.Running != nil {
		if containerObj != nil {
			containerObj.StartTime = &st.State.Running.StartedAt.Time
			if st.LastTerminationState.Terminated != nil {
				containerObj.LastTerminationTime = &st.LastTerminationState.Terminated.FinishedAt.Time
				containerObj.ExitCode = st.LastTerminationState.Terminated.ExitCode
			}
		}
	}

	podObject.Containers[containerObj.Name] = *containerObj

	if bag.LogLines > 0 && (containerObj.Restarts > 2 || podObject.Phase == "Failed") {
		po := v1.PodLogOptions{}
		var since int64 = int64(bag.SnapshotSyncInterval)
		var lines int64 = int64(bag.LogLines)
		po.SinceSeconds = &since
		po.Container = containerObj.Name
		po.Follow = false
		po.Previous = false
		po.Timestamps = true
		po.TailLines = &lines
		pw.saveLogs(podObject.ClusterName, podObject.Namespace, podObject.Owner, podObject.Name, &po)
	}

	key := utils.GetContainerKey(podObject, containerObj)
	lastSnapshot, ok := pw.ContainerCache[key]
	if ok {
		if lastSnapshot.Restarts != st.RestartCount ||
			containerObj.LastTerminationTime == nil && st.LastTerminationState.Terminated != nil {
			changed = true
		}
	} else {
		changed = true
	}
	lockContainerCache.Lock()
	pw.ContainerCache[key] = *containerObj
	lockContainerCache.Unlock()

	return changed
}

func findContainer(podObject *m.PodSchema, containerName string) *m.ContainerSchema {
	if c, ok := podObject.Containers[containerName]; ok {
		return &c
	}
	return nil
}

func findInitContainer(podObject *m.PodSchema, containerName string) *m.ContainerSchema {
	if c, ok := podObject.InitContainers[containerName]; ok {
		return &c
	}
	return nil
}

func (pw PodWorker) processContainer(podObj *v1.Pod, podSchema *m.PodSchema, c v1.Container, init bool) (m.ContainerSchema, bool) {
	var sb strings.Builder
	var limitsDefined bool = false
	containerObj := m.NewContainerObj()
	containerObj.Name = c.Name
	containerObj.ClusterName = podSchema.ClusterName
	containerObj.Namespace = podSchema.Namespace
	containerObj.NodeName = podSchema.NodeName
	containerObj.PodName = podSchema.Name
	containerObj.Init = init
	containerObj.PodInitTime = podSchema.StartTime

	if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
		podSchema.NumPrivileged++
		containerObj.Privileged = 1
	}

	if c.LivenessProbe == nil {
		podSchema.LiveProbes++
		containerObj.LiveProbes = 1
	}

	for _, p := range c.Ports {
		cp := pw.CheckPort(&p, podObj, podSchema)
		containerObj.ContainerPorts = append(containerObj.ContainerPorts, *cp)
		if !cp.Mapped {
			containerObj.MissingServices += fmt.Sprintf("%s;", strconv.Itoa(int(cp.PortNumber)))
		}
	}

	if c.ReadinessProbe == nil {
		podSchema.ReadyProbes++
		containerObj.ReadyProbes = 1
	}

	if c.Resources.Requests != nil {

		var cpuReq int64 = 0
		if c.Resources.Requests.Cpu() != nil {
			cpuReq = c.Resources.Requests.Cpu().MilliValue()

			podSchema.CpuRequest += cpuReq
			containerObj.CpuRequest = cpuReq
			containerObj.RequestCpuString = c.Resources.Requests.Cpu().String()
		}

		var memReq int64 = 0
		if c.Resources.Requests.Memory() != nil {
			memReq = c.Resources.Requests.Memory().MilliValue() / 1000

			podSchema.MemRequest += memReq
			containerObj.MemRequest = memReq

			containerObj.RequestMemString = c.Resources.Requests.Memory().String()
		}

		limitsDefined = true
	}

	if c.Resources.Limits != nil {
		var cpuLim int64 = 0

		if c.Resources.Limits.Cpu() != nil {
			cpuLim = c.Resources.Limits.Cpu().MilliValue()

			podSchema.CpuLimit += cpuLim
			containerObj.CpuLimit = cpuLim
			containerObj.LimitCpuString = c.Resources.Limits.Cpu().String()
		}

		var memLim int64 = 0
		if c.Resources.Limits.Memory() != nil {
			memLim = c.Resources.Limits.Memory().MilliValue() / 1000

			podSchema.MemLimit += memLim
			containerObj.MemLimit = memLim
			containerObj.LimitMemString = c.Resources.Limits.Memory().String()
		}

		//check storage needs
		//ephemeral
		for key, val := range c.Resources.Requests {
			if key == v1.ResourceRequestsEphemeralStorage {
				containerObj.PodStorageRequest = val.MilliValue()
				podSchema.PodStorageRequest += containerObj.PodStorageRequest
			}
		}

		for key, val := range c.Resources.Limits {
			if key == v1.ResourceLimitsEphemeralStorage {
				containerObj.PodStorageLimit = val.MilliValue()
				podSchema.PodStorageLimit += containerObj.PodStorageLimit
			}
		}

		limitsDefined = true
	}

	//check missing dependencies
	for _, ev := range c.Env {
		if ev.ValueFrom != nil && ev.ValueFrom.ConfigMapKeyRef != nil {
			cmKey := utils.GetKey(podObj.Namespace, ev.ValueFrom.ConfigMapKeyRef.Name)
			if _, okCM := pw.CMCache[cmKey]; !okCM {
				pw.Logger.Debugf("CM with key %s not found\n", cmKey)
				containerObj.MissingConfigs += fmt.Sprintf("%s;", ev.ValueFrom.ConfigMapKeyRef.Name)
			}
		}
		if ev.ValueFrom != nil && ev.ValueFrom.SecretKeyRef != nil {
			sKey := utils.GetKey(podObj.Namespace, ev.ValueFrom.SecretKeyRef.Name)
			if _, okS := pw.SecretCache[sKey]; !okS {
				containerObj.MissingSecrets += fmt.Sprintf("%s;", ev.ValueFrom.SecretKeyRef.Name)
			}
		}
	}

	for _, evf := range c.EnvFrom {
		if evf.ConfigMapRef != nil {
			cmKey := utils.GetKey(podObj.Namespace, evf.ConfigMapRef.Name)
			if _, okCM := pw.CMCache[cmKey]; !okCM {
				containerObj.MissingConfigs += fmt.Sprintf("%s;", evf.ConfigMapRef.Name)
			}
		}

		if evf.SecretRef != nil {
			sKey := utils.GetKey(podObj.Namespace, evf.SecretRef.Name)
			if _, okS := pw.SecretCache[sKey]; !okS {
				containerObj.MissingSecrets += fmt.Sprintf("%s;", evf.SecretRef.Name)
			}
		}
	}

	if c.VolumeMounts != nil {
		sb.Reset()
		for _, vol := range c.VolumeMounts {
			fmt.Fprintf(&sb, "%s;", vol.MountPath)
			//check PVC limits
			for _, vm := range podObj.Spec.Volumes {
				if vm.Name == vol.Name && vm.PersistentVolumeClaim != nil {
					claim := vm.PersistentVolumeClaim
					//lookup claim by name in the cache of known claims
					key := utils.GetKey(podSchema.Namespace, claim.ClaimName)
					pvc, ok := pw.PVCCache[key]
					if ok {
						pvcReq, exists := pvc.Spec.Resources.Requests[v1.ResourceStorage]
						if exists {
							containerObj.StorageRequest += pvcReq.MilliValue()
							podSchema.StorageRequest += pvcReq.MilliValue()
						}

						pvcCapacity, exists := pvc.Status.Capacity[v1.ResourceStorage]
						if exists {
							containerObj.StorageCapacity += pvcCapacity.MilliValue()
							podSchema.StorageCapacity += pvcCapacity.MilliValue()
						}
					}
				}
				//check missing dependencies
				if vm.ConfigMap != nil && vm.Name == vol.Name {
					cmKey := utils.GetKey(podObj.Namespace, vm.ConfigMap.Name)
					if _, okCM := pw.CMCache[cmKey]; !okCM {
						containerObj.MissingConfigs += fmt.Sprintf("%s;", vm.ConfigMap.Name)
					}
				}
				if vm.Secret != nil && vm.Name == vol.Name {
					sKey := utils.GetKey(podObj.Namespace, vm.Secret.SecretName)
					if _, okS := pw.SecretCache[sKey]; !okS {
						containerObj.MissingSecrets += fmt.Sprintf("%s;", vm.Secret.SecretName)
					}
				}
			}
		}
		containerObj.Mounts = sb.String()
	}
	containerObj.LimitsDefined = limitsDefined

	return containerObj, limitsDefined
}

func (pw PodWorker) CheckPort(port *v1.ContainerPort, podObj *v1.Pod, podSchema *m.PodSchema) *m.ContainerPort {
	cp := m.ContainerPort{}
	cp.PortNumber = port.ContainerPort
	cp.Name = port.Name

	//check if service exists and routes to the ports
	for _, svc := range podSchema.Services {
		mapped, ready := svc.IsPortAvailable(&cp, podSchema)
		if !cp.Mapped {
			cp.Mapped = mapped
		}
		if !cp.Ready {
			cp.Ready = ready
		}
	}
	return &cp
}

func compareString(val1, val2 string, differ bool) bool {
	if differ {
		return differ
	}
	return val1 != val2
}

func compareInt64(val1, val2 int64, differ bool) bool {
	if differ {
		return differ
	}
	return val1 != val2
}

func (pw PodWorker) saveLogs(clusterName string, namespace string, podOwner string, podName string, logOptions *v1.PodLogOptions) error {
	req := pw.Client.CoreV1().RESTClient().Get().
		Namespace(namespace).
		Name(podName).
		Resource("pods").
		SubResource("log").
		Param("follow", strconv.FormatBool(logOptions.Follow)).
		Param("container", logOptions.Container).
		Param("previous", strconv.FormatBool(logOptions.Previous)).
		Param("timestamps", strconv.FormatBool(logOptions.Timestamps))

	if logOptions.SinceSeconds != nil {
		req.Param("sinceSeconds", strconv.FormatInt(*logOptions.SinceSeconds, 10))
	}
	if logOptions.SinceTime != nil {
		req.Param("sinceTime", logOptions.SinceTime.Format(time.RFC3339))
	}
	if logOptions.LimitBytes != nil {
		req.Param("limitBytes", strconv.FormatInt(*logOptions.LimitBytes, 10))
	}
	if logOptions.TailLines != nil {
		req.Param("tailLines", strconv.FormatInt(*logOptions.TailLines, 10))
	}
	readCloser, err := req.Stream()
	if err != nil {
		pw.Logger.Errorf("Issues when reading logs for pod %s %s:. %v\n", namespace, podName, err)
		return err
	}

	defer readCloser.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(readCloser)
	s := buf.String()
	logs := strings.Split(s, "\n")
	batchTS := time.Now().Unix()
	objList := []m.LogSchema{}
	for _, l := range logs {
		if l != "" {
			//			fmt.Printf("Pod %s logs:\n %s.\n", podName, l)
			logSchema := m.NewLogObj()
			logSchema.ClusterName = clusterName
			logSchema.Namespace = namespace
			logSchema.PodOwner = podOwner
			logSchema.PodName = podName
			logSchema.ContainerName = logOptions.Container
			//get the timestamp
			line := strings.Split(l, " ")
			m := l
			if len(line) > 0 {
				t, errTime := time.Parse(time.RFC3339Nano, line[0])
				if errTime == nil {
					logSchema.Timestamp = &t
				}
				if len(line) > 1 {
					m = strings.TrimLeft(l, line[0]+" ")
				}
			}

			logSchema.Message = utils.TruncateString(m, app.MAX_FIELD_LENGTH)
			logSchema.BatchTimestamp = batchTS
			objList = append(objList, logSchema)
		}
	}
	if len(objList) > 0 {
		pw.postLogRecords(&objList)
	}

	return err
}

func (pw *PodWorker) postLogRecords(objList *[]m.LogSchema) {
	bag := (*pw.ConfManager).Get()

	rc := app.NewRestClient(bag, pw.Logger)
	data, err := json.Marshal(objList)
	schemaDefObj := m.NewLogSchemaDefWrapper()
	schemaDef, e := json.Marshal(schemaDefObj)
	if err == nil && e == nil {
		if rc.SchemaExists(bag.LogSchemaName) == false {
			pw.Logger.Debugf("Creating Log schema. %s\n", bag.LogSchemaName)
			schemaObj, err := rc.CreateSchema(bag.LogSchemaName, schemaDef)
			if err != nil {
				return
			} else if schemaObj != nil {
				pw.Logger.Debugf("Schema %s created\n", bag.LogSchemaName)
			} else {
				pw.Logger.Debugf("Schema %s exists\n", bag.LogSchemaName)
			}
		}

		rc.PostAppDEvents(bag.LogSchemaName, data)
	} else {
		pw.Logger.Errorf("Problems when serializing array of logs . %v", err)
	}
}

func (pw PodWorker) GetPodMetrics() *map[string]m.UsageStats {
	metricsDonePods := make(chan *m.PodMetricsObjList)
	go metricsWorkerPods(metricsDonePods, pw.Client, pw.Logger)

	metricsDataPods := <-metricsDonePods
	objMap := metricsDataPods.PrintPodList()
	return &objMap
}

func (pw PodWorker) GetPodMetricsSingle(p *v1.Pod, namespace string, podName string) *m.PodMetricsObj {
	if !utils.IsPodRunnnig(p) {
		return nil
	}
	metricsDone := make(chan *m.PodMetricsObj)
	go metricsWorkerSingle(metricsDone, pw.Client, namespace, podName, pw.Logger)

	metricsData := <-metricsDone
	return metricsData
}

func metricsWorkerPods(finished chan *m.PodMetricsObjList, client *kubernetes.Clientset, l *log.Logger) {
	l.Debug("Metrics Worker Pods: Started")
	var path string = "apis/metrics.k8s.io/v1beta1/pods"

	data, err := client.RESTClient().Get().AbsPath(path).DoRaw()
	if err != nil {
		l.Errorf("Issues when requesting metrics from metrics with path %s from server %s\n", path, err.Error())
	}

	var list m.PodMetricsObjList
	merde := json.Unmarshal(data, &list)
	if merde != nil {
		l.Errorf("Unmarshal issues. %v\n", merde)
	}

	fmt.Println("Metrics Worker Pods: Finished. Metrics records: %d", len(list.Items))
	fmt.Println(&list)
	finished <- &list
}

func metricsWorkerSingle(finished chan *m.PodMetricsObj, client *kubernetes.Clientset, namespace string, podName string, l *log.Logger) {
	var path string = ""
	var metricsObj m.PodMetricsObj
	if namespace != "" && podName != "" {
		path = fmt.Sprintf("apis/metrics.k8s.io/v1beta1/namespaces/%s/pods/%s", namespace, podName)

		data, err := client.RESTClient().Get().AbsPath(path).DoRaw()
		if err != nil {
			l.Errorf("Issues when requesting metrics from metrics with path %s from server %s\n", path, err.Error())
		}

		merde := json.Unmarshal(data, &metricsObj)
		if merde != nil {
			l.Errorf("Unmarshal issues when getting pods metrics. %v\n", merde)
		}

	}

	finished <- &metricsObj
}

func (pw PodWorker) builAppDMetricsList() m.AppDMetricList {
	ml := m.NewAppDMetricList()
	var list []m.AppDMetric
	for _, metricPod := range pw.SummaryMap {
		objMap := metricPod.Unwrap()
		pw.addMetricToList(*objMap, metricPod, &list)
		//quotas
		quotaSpecs := metricPod.GetQuotaSpecMetrics()
		qsMap := quotaSpecs.Unwrap()
		pw.addMetricToList(*qsMap, quotaSpecs, &list)

		quotaUsed := metricPod.GetQuotaUsedMetrics()
		quMap := quotaUsed.Unwrap()
		pw.addMetricToList(*quMap, quotaUsed, &list)
	}
	for _, metricApp := range pw.AppSummaryMap {
		objMap := metricApp.Unwrap()
		pw.addMetricToList(*objMap, metricApp, &list)
		//quotas
		quotaSpecs := metricApp.GetQuotaSpecMetrics()
		qsMap := quotaSpecs.Unwrap()
		pw.addMetricToList(*qsMap, quotaSpecs, &list)

		quotaUsed := metricApp.GetQuotaUsedMetrics()
		quMap := quotaUsed.Unwrap()
		pw.addMetricToList(*quMap, quotaUsed, &list)

		//services
		for _, svcMetrics := range metricApp.Services {
			objSvcMap := svcMetrics.Unwrap()
			pw.addMetricToList(*objSvcMap, svcMetrics, &list)
			//endpoints
			for _, epMetrics := range svcMetrics.Endpoints {
				objEPMap := epMetrics.Unwrap()
				pw.addMetricToList(*objEPMap, epMetrics, &list)
			}
		}
	}

	for _, metricContainer := range pw.ContainerSummaryMap {
		objMap := structs.Map(metricContainer)
		pw.addMetricToList(objMap, metricContainer, &list)
	}

	for _, metricInstance := range pw.InstanceSummaryMap {
		objMap := structs.Map(metricInstance)
		pw.addMetricToList(objMap, metricInstance, &list)
		for _, portMetric := range metricInstance.PortMetrics {
			portObjMap := structs.Map(portMetric)
			pw.addMetricToList(portObjMap, portMetric, &list)
		}
	}

	ml.Items = list
	return ml
}

func (pw PodWorker) addMetricToList(objMap map[string]interface{}, metric m.AppDMetricInterface, list *[]m.AppDMetric) {

	for fieldName, fieldValue := range objMap {
		if !metric.ShouldExcludeField(fieldName) {
			appdMetric := m.NewAppDMetric(fieldName, fieldValue.(int64), metric.GetPath())
			*list = append(*list, appdMetric)
		}
	}
}

//dashboards
func (pw *PodWorker) buildDashboards(dashData map[string]m.DashboardBag) {
	bag := (*pw.ConfManager).Get()
	bth := pw.AppdController.StartBT("BuildDashboard")
	//clear cache
	pw.DashboardCache = make(map[string]m.PodSchema)
	dw := NewDashboardWorker(bag, pw.Logger, pw.AppdController)
	for _, bag := range dashData {
		if bag.Type == "tier" && len(bag.Pods) > 0 {
			dw.updateTierDashboard(&bag)
		}
		if bag.Type == "cluster" {
			err := dw.updateClusterOverview(&bag)
			if err != nil {
				pw.Logger.WithField("error", err).Error("Unable to build cluster overview dashboard")
			}
		}
	}
	pw.AppdController.StopBT(bth)
}

func (pw *PodWorker) addToAssociationQueue(retryObj *m.AgentRetryRequest) {
	lockAssociationQueue.Lock()
	defer lockAssociationQueue.Unlock()
	pw.PendingAssociationQueue[utils.GetPodKey(retryObj.Pod)] = *retryObj
}

func (pw *PodWorker) startRetryQueueWorker(stopCh <-chan struct{}) {
	bag := (*pw.ConfManager).Get()
	pw.retryQueueTicker(stopCh, time.NewTicker(time.Duration(bag.MetricsSyncInterval)*time.Second))
}

func (pw *PodWorker) retryQueueTicker(stop <-chan struct{}, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			pw.flushAssociationQueue()
		case <-stop:
			ticker.Stop()
			return
		}
	}
}

func (pw *PodWorker) flushAssociationQueue() {
	bag := (*pw.ConfManager).Get()
	purgeList := []m.AgentRetryRequest{}
	lockAssociationQueue.RLock()
	injector := instr.NewAgentInjector(pw.Client, pw.K8sConfig, bag, pw.AppdController, pw.Logger)
	for _, p := range pw.PendingAssociationQueue {
		//update pod from cache
		podKey := utils.GetPodKey(p.Pod)
		updated, ok, err := pw.informer.GetStore().GetByKey(podKey)
		if err == nil && ok {
			podObj := updated.(*v1.Pod)
			if utils.IsPodRunnnig(podObj) {
				pw.Logger.Debugf("Updated version of pod %s found. Retrying association...\n", podKey)
				p.Pod = podObj
				err := injector.RetryAssociate(p.Pod, p.Request)
				if err == nil {
					purgeList = append(purgeList, p)
				}
			}
		} else {
			pw.Logger.Infof("Pod %s no longer exits. Removing from the association queue...\n", podKey)
			purgeList = append(purgeList, p)
		}
	}
	lockAssociationQueue.RUnlock()
	if len(purgeList) > 0 {
		pw.purgeAssociationQueue(purgeList)
	}
}

func (pw *PodWorker) purgeAssociationQueue(retryObjects []m.AgentRetryRequest) {
	lockAssociationQueue.Lock()
	defer lockAssociationQueue.Unlock()
	for _, p := range retryObjects {
		key := utils.GetPodKey(p.Pod)
		if _, ok := pw.PendingAssociationQueue[key]; ok {
			delete(pw.PendingAssociationQueue, key)
		}
	}
}

func (pw *PodWorker) OnPodErrorEvent(podName string, eventSchema m.EventSchema) {
	bag := (*pw.ConfManager).Get()
	lockEventLock.Lock()
	defer lockEventLock.Unlock()
	key := utils.GetKey(eventSchema.Namespace, podName)
	list, ok := pw.EventMap[key]
	if !ok {
		list = []m.EventSchema{}
	}
	limit := len(list)
	for limit >= bag.PodEventNumber {
		list = append(list[:0], list[1:]...)
		limit = len(list)
	}
	pw.Logger.Debugf("Registering pod event %s %s", eventSchema.Reason, eventSchema.Message)
	list = append(list, eventSchema)

	pw.EventMap[key] = list

	//check if pending pod is failing due to inability to load image
	issuePod, ok, err := pw.informer.GetStore().GetByKey(key)
	pendingAttachPod := false
	deployName := ""
	var podObj *v1.Pod = nil
	if err == nil && ok {
		p := issuePod.(*v1.Pod)
		for k, v := range p.Annotations {
			if k == instr.APPD_ATTACH_PENDING {
				pendingAttachPod = true
			}
			if k == instr.APPD_ATTACH_DEPLOYMENT {
				deployName = v
			}
		}
		if pendingAttachPod && deployName != "" {
			podObj = p
		}
	}

	if podObj != nil && eventSchema.Reason == "Failed" && (strings.Contains(eventSchema.Message, "ErrImagePull") || strings.Contains(eventSchema.Message, "Failed to pull image")) {
		msg := fmt.Sprintf("AppDynamics instrumentation cannot be complete as one of the agent images is not accessible. The instrumentation is being canceled. Make sure that AppDynamics images are available in namespace %s", eventSchema.Namespace)
		EmitInstrumentationEvent(podObj, pw.Client, "AppDInstrumentation", msg, v1.EventTypeWarning)
		ReverseDeploymentInstrumentation(deployName, eventSchema.Namespace, bag, pw.Logger, pw.Client)
	}
}

func (pw *PodWorker) GetPodEvents(podSchema *m.PodSchema) []string {
	lockEventLock.RLock()
	defer lockEventLock.RUnlock()
	key := utils.GetKey(podSchema.Namespace, podSchema.Name)
	if list, ok := pw.EventMap[key]; ok {
		messages := []string{}
		for _, eventSchema := range list {
			elapsedTime := time.Now().Sub(eventSchema.LastTimestamp)
			eventMessage := fmt.Sprintf("%s. %s", utils.FormatDuration(int64(elapsedTime)), eventSchema.Reason)
			pw.Logger.Debugf("Adding pod event message %s", eventMessage)
			messages = append(messages, eventMessage)
		}
		return messages
	}

	return []string{}
}

func (pw *PodWorker) GetCpuConsumption(podSchema *m.PodSchema, containerSchema *m.ContainerSchema) (float64, string) {
	if containerSchema.CpuUse > 0 {
		if containerSchema.CpuRequest > 0 {
			pw.Logger.WithFields(log.Fields{"Pod": podSchema.Name, "MemUsed": containerSchema.CpuUse, "MemRequest": containerSchema.CpuRequest}).Debug("CPU Consumption check")
			return float64(containerSchema.CpuUse) / float64(containerSchema.CpuRequest) * float64(100), containerSchema.RequestCpuString
		} else if containerSchema.CpuLimit > 0 {
			return float64(containerSchema.CpuUse) / float64(containerSchema.CpuLimit) * float64(100), containerSchema.LimitCpuString
		} else if pw.NodesMonitor != nil {
			nodeInfo := pw.NodesMonitor.GetNodeData(podSchema.NodeName)
			if nodeInfo != nil {
				if nodeInfo.CpuCapacity > 0 {
					return float64(containerSchema.CpuUse) / float64(nodeInfo.CpuCapacity) * float64(100), ""
				}
			}
		}
	}
	return -1, ""
}

func (pw *PodWorker) GetMemConsumption(podSchema *m.PodSchema, containerSchema *m.ContainerSchema) (float64, string) {
	if containerSchema.MemUse > 0 {
		if containerSchema.MemRequest > 0 {
			pw.Logger.WithFields(log.Fields{"Pod": podSchema.Name, "MemUsed": containerSchema.MemUse, "MemRequest": containerSchema.MemRequest}).Debug("Mem Consumption check")
			return float64(containerSchema.MemUse) / float64(containerSchema.MemRequest) * float64(100), containerSchema.RequestMemString
		} else if containerSchema.MemLimit > 0 {
			return float64(containerSchema.MemUse) / float64(containerSchema.MemLimit) * float64(100), containerSchema.LimitMemString
		} else if pw.NodesMonitor != nil {
			nodeInfo := pw.NodesMonitor.GetNodeData(podSchema.NodeName)
			if nodeInfo != nil {
				if nodeInfo.MemCapacity > 0 {
					return float64(containerSchema.MemUse) / float64(nodeInfo.MemCapacity) * float64(100), ""
				}
			}
		}
	}
	return -1, ""
}

func (pw *PodWorker) GetPodUtilization(podSchema *m.PodSchema) map[string]m.Utilization {
	bag := (*pw.ConfManager).Get()
	mu := make(map[string]m.Utilization)
	for _, c := range podSchema.Containers {
		if !c.Init {
			pw.Logger.WithFields(log.Fields{"Container": c.Name, "Cpu": c.ConsumptionCpu, "Mem": c.ConsumptionMem}).Debug("GetPodUtilization")
			cu := m.Utilization{CpuUse: c.ConsumptionCpu, CpuUseString: c.ConsumptionCpuString, MemUse: c.ConsumptionMem, MemUseString: c.ConsumptionMemString, Restarts: c.Restarts}
			cpuCeiling := c.CpuRequest
			if cpuCeiling == 0 {
				cpuCeiling = c.CpuLimit
			}
			memCeiling := c.MemRequest
			if memCeiling == 0 {
				memCeiling = c.MemLimit
			}
			cu.CheckStatus(float64(bag.OverconsumptionThreshold), float64(cpuCeiling), float64(memCeiling))
			mu[c.Name] = cu
		}
	}

	return mu
}
