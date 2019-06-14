package workers

import (
	"encoding/json"

	log "github.com/sirupsen/logrus"

	"sync"
	"time"

	app "github.com/appdynamics/cluster-agent/appd"
	"github.com/appdynamics/cluster-agent/config"

	m "github.com/appdynamics/cluster-agent/models"
	"github.com/appdynamics/cluster-agent/utils"
	appsv1 "k8s.io/api/apps/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"k8s.io/client-go/util/workqueue"
)

type RsWorker struct {
	informer       cache.SharedIndexInformer
	Client         *kubernetes.Clientset
	ConfigManager  *config.MutexConfigManager
	SummaryMap     map[string]m.ClusterRsMetrics
	WQ             workqueue.RateLimitingInterface
	AppdController *app.ControllerClient
	PendingCache   []string
	FailedCache    map[string]m.AttachStatus
	Logger         *log.Logger
}

func NewRsWorker(client *kubernetes.Clientset, cm *config.MutexConfigManager, controller *app.ControllerClient, l *log.Logger) RsWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	dw := RsWorker{Client: client, ConfigManager: cm, SummaryMap: make(map[string]m.ClusterRsMetrics), WQ: queue,
		AppdController: controller, PendingCache: []string{}, FailedCache: make(map[string]m.AttachStatus), Logger: l}
	dw.initRsInformer(client)
	return dw
}

func (nw *RsWorker) initRsInformer(client *kubernetes.Clientset) cache.SharedIndexInformer {
	i := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.AppsV1().ReplicaSets(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.AppsV1().ReplicaSets(metav1.NamespaceAll).Watch(options)
			},
		},
		&appsv1.ReplicaSet{},
		0,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	i.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    nw.onNewReplicaSet,
		DeleteFunc: nw.onDeleteReplicaSet,
		UpdateFunc: nw.onUpdateReplicaSet,
	})
	nw.informer = i

	return i
}

func (dw *RsWorker) Observe(stopCh <-chan struct{}, wg *sync.WaitGroup) {
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

func (pw *RsWorker) qualifies(p *appsv1.ReplicaSet) bool {
	return (len((*pw.ConfigManager).Get().NsToMonitor) == 0 ||
		utils.StringInSlice(p.Namespace, (*pw.ConfigManager).Get().NsToMonitor)) &&
		!utils.StringInSlice(p.Namespace, (*pw.ConfigManager).Get().NsToMonitorExclude)
}

func (dw *RsWorker) onNewReplicaSet(obj interface{}) {
	RsObj := obj.(*appsv1.ReplicaSet)
	if !dw.qualifies(RsObj) {
		return
	}
	dw.Logger.Debugf("Added ReplicaSet: %s\n", RsObj.Name)

	RsRecord, _ := dw.processObject(RsObj, nil)
	dw.WQ.Add(&RsRecord)

}

func (dw *RsWorker) onDeleteReplicaSet(obj interface{}) {
	RsObj := obj.(*appsv1.ReplicaSet)
	if !dw.qualifies(RsObj) {
		return
	}
	dw.Logger.Debugf("Deleted ReplicaSet: %s\n", RsObj.Name)
}

func (dw *RsWorker) onUpdateReplicaSet(objOld interface{}, objNew interface{}) {
	RsObj := objNew.(*appsv1.ReplicaSet)
	if !dw.qualifies(RsObj) {
		return
	}
	dw.Logger.Debugf("ReplicaSet %s changed\n", RsObj.Name)

	RsRecord, _ := dw.processObject(RsObj, nil)
	dw.WQ.Add(&RsRecord)
}

func (pw *RsWorker) startMetricsWorker(stopCh <-chan struct{}) {
	bag := (*pw.ConfigManager).Get()
	pw.appMetricTicker(stopCh, time.NewTicker(time.Duration(bag.MetricsSyncInterval)*time.Second))

}

func (pw *RsWorker) appMetricTicker(stop <-chan struct{}, ticker *time.Ticker) {
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

func (pw *RsWorker) eventQueueTicker(stop <-chan struct{}, ticker *time.Ticker) {
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

func (pw *RsWorker) startEventQueueWorker(stopCh <-chan struct{}) {
	bag := (*pw.ConfigManager).Get()
	pw.eventQueueTicker(stopCh, time.NewTicker(time.Duration(bag.SnapshotSyncInterval)*time.Second))
}

func (pw *RsWorker) flushQueue() {
	bag := (*pw.ConfigManager).Get()
	bth := pw.AppdController.StartBT("FlushReplicaSetDataQueue")
	count := pw.WQ.Len()
	if count > 0 {
		pw.Logger.Infof("Flushing the queue of %d ReplicaSet records\n", count)
	}
	if count == 0 {
		pw.AppdController.StopBT(bth)
		return
	}

	var objList []m.DeploySchema

	var RsRecord *m.DeploySchema
	var ok bool = true

	for count >= 0 {
		RsRecord, ok = pw.getNextQueueItem()
		count = count - 1
		if ok {
			objList = append(objList, *RsRecord)
		} else {
			pw.Logger.Info("RS Queue shut down")
		}
		if count == 0 || len(objList) >= bag.EventAPILimit {
			pw.Logger.Debugf("Sending %d ReplicaSet records to AppD events API\n", len(objList))
			pw.postRsRecords(&objList)
			pw.AppdController.StopBT(bth)
			return
		}
	}
	pw.AppdController.StopBT(bth)
}

func (pw *RsWorker) postRsRecords(objList *[]m.DeploySchema) {
	bag := (*pw.ConfigManager).Get()

	rc := app.NewRestClient(bag, pw.Logger)

	schemaDefObj := m.NewDeploySchemaDefWrapper()

	err := rc.EnsureSchema(bag.DeploySchemaName, &schemaDefObj)
	if err != nil {
		pw.Logger.Errorf("Issues when ensuring %s schema. %v\n", bag.DeploySchemaName, err)
	} else {
		data, err := json.Marshal(objList)
		if err != nil {
			pw.Logger.Errorf("Problems when serializing array of rs schemas. %v", err)
		}
		rc.PostAppDEvents(bag.DeploySchemaName, data)
	}
}

func (pw *RsWorker) getNextQueueItem() (*m.DeploySchema, bool) {
	RsRecord, quit := pw.WQ.Get()

	if quit {
		return RsRecord.(*m.DeploySchema), false
	}
	defer pw.WQ.Done(RsRecord)
	pw.WQ.Forget(RsRecord)

	return RsRecord.(*m.DeploySchema), true
}

func (pw *RsWorker) buildAppDMetrics() {
	bth := pw.AppdController.StartBT("PostReplicaSetMetrics")
	pw.SummaryMap = make(map[string]m.ClusterRsMetrics)

	var count int = 0
	for _, obj := range pw.informer.GetStore().List() {
		RsObject := obj.(*appsv1.ReplicaSet)
		if !pw.qualifies(RsObject) {
			continue
		}
		DeploySchema, _ := pw.processObject(RsObject, nil)
		pw.summarize(&DeploySchema)
		count++
	}

	if count == 0 {
		bag := (*pw.ConfigManager).Get()
		pw.SummaryMap[m.ALL] = m.NewClusterRsMetrics(bag, m.ALL)
	}

	ml := pw.builAppDMetricsList()

	pw.Logger.Infof("Ready to push %d ReplicaSet metrics\n", len(ml.Items))

	pw.AppdController.PostMetrics(ml)
	pw.AppdController.StopBT(bth)
}

func (pw *RsWorker) summarize(RsObject *m.DeploySchema) {
	bag := (*pw.ConfigManager).Get()
	//global metrics
	summary, okSum := pw.SummaryMap[m.ALL]
	if !okSum {
		summary = m.NewClusterRsMetrics(bag, m.ALL)
		pw.SummaryMap[m.ALL] = summary
	}

	//namespace metrics
	summaryNS, okNS := pw.SummaryMap[RsObject.Namespace]
	if !okNS {
		summaryNS = m.NewClusterRsMetrics(bag, RsObject.Namespace)
		pw.SummaryMap[RsObject.Namespace] = summaryNS
	}

	summary.RsCount++
	summaryNS.RsCount++

	if RsObject.Replicas == 0 {
		summary.RsStaleCount++
		summaryNS.RsStaleCount++
	}

	summary.RsReplicas = summary.RsReplicas + int64(RsObject.Replicas)
	summaryNS.RsReplicas = summaryNS.RsReplicas + int64(RsObject.Replicas)

	summary.RsReplicasUnAvailable = summary.RsReplicasUnAvailable + int64(RsObject.ReplicasUnAvailable)
	summaryNS.RsReplicasUnAvailable = summaryNS.RsReplicasUnAvailable + int64(RsObject.ReplicasUnAvailable)

	summary.RsReplicasAvailable = summary.RsReplicasAvailable + int64(RsObject.ReplicasAvailable)
	summaryNS.RsReplicasAvailable = summaryNS.RsReplicasAvailable + int64(RsObject.ReplicasAvailable)

	pw.SummaryMap[m.ALL] = summary
	pw.SummaryMap[RsObject.Namespace] = summaryNS
}

func (pw *RsWorker) processObject(d *appsv1.ReplicaSet, old *appsv1.ReplicaSet) (m.DeploySchema, bool) {
	changed := true
	bag := (*pw.ConfigManager).Get()

	RsObject := m.NewDeployObj()
	RsObject.Name = d.Name
	RsObject.Namespace = d.Namespace
	RsObject.DeploymentType = m.DEPLOYMENT_TYPE_RS

	if d.ClusterName != "" {
		RsObject.ClusterName = d.ClusterName
	} else {
		RsObject.ClusterName = bag.AppName
	}

	RsObject.ObjectUid = string(d.GetUID())
	RsObject.CreationTimestamp = d.GetCreationTimestamp().Time
	if d.GetDeletionTimestamp() != nil {
		RsObject.DeletionTimestamp = d.GetDeletionTimestamp().Time
	}
	RsObject.MinReadySecs = d.Spec.MinReadySeconds

	RsObject.Replicas = d.Status.Replicas

	RsObject.ReplicasAvailable = d.Status.AvailableReplicas
	RsObject.ReplicasUnAvailable = d.Status.Replicas - d.Status.AvailableReplicas
	RsObject.ReplicasReady = d.Status.ReadyReplicas
	RsObject.ReplicasLabeled = d.Status.FullyLabeledReplicas

	return RsObject, changed
}

func (pw RsWorker) builAppDMetricsList() m.AppDMetricList {
	ml := m.NewAppDMetricList()
	var list []m.AppDMetric
	for _, metricNode := range pw.SummaryMap {
		objMap := metricNode.Unwrap()
		pw.addMetricToList(*objMap, metricNode, &list)
	}

	ml.Items = list
	return ml
}

func (pw RsWorker) addMetricToList(objMap map[string]interface{}, metric m.AppDMetricInterface, list *[]m.AppDMetric) {

	for fieldName, fieldValue := range objMap {
		if !metric.ShouldExcludeField(fieldName) {
			appdMetric := m.NewAppDMetric(fieldName, fieldValue.(int64), metric.GetPath())
			*list = append(*list, appdMetric)
		}
	}
}
