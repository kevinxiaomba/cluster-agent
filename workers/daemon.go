package workers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	app "github.com/sjeltuhin/clusterAgent/appd"
	"github.com/sjeltuhin/clusterAgent/config"

	m "github.com/sjeltuhin/clusterAgent/models"
	"github.com/sjeltuhin/clusterAgent/utils"
	appsv1 "k8s.io/api/apps/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"k8s.io/client-go/util/workqueue"
)

type DaemonWorker struct {
	informer       cache.SharedIndexInformer
	Client         *kubernetes.Clientset
	ConfigManager  *config.MutexConfigManager
	SummaryMap     map[string]m.ClusterDaemonMetrics
	WQ             workqueue.RateLimitingInterface
	AppdController *app.ControllerClient
	PendingCache   []string
	FailedCache    map[string]m.AttachStatus
}

func NewDaemonWorker(client *kubernetes.Clientset, cm *config.MutexConfigManager, controller *app.ControllerClient) DaemonWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	dw := DaemonWorker{Client: client, ConfigManager: cm, SummaryMap: make(map[string]m.ClusterDaemonMetrics), WQ: queue,
		AppdController: controller, PendingCache: []string{}, FailedCache: make(map[string]m.AttachStatus)}
	dw.initDaemonInformer(client)
	return dw
}

func (nw *DaemonWorker) initDaemonInformer(client *kubernetes.Clientset) cache.SharedIndexInformer {
	i := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.AppsV1().DaemonSets(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.AppsV1().DaemonSets(metav1.NamespaceAll).Watch(options)
			},
		},
		&appsv1.DaemonSet{},
		0,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	i.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    nw.onNewDaemonSet,
		DeleteFunc: nw.onDeleteDaemonSet,
		UpdateFunc: nw.onUpdateDaemonSet,
	})
	nw.informer = i

	return i
}

func (dw *DaemonWorker) Observe(stopCh <-chan struct{}, wg *sync.WaitGroup) {
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

func (pw *DaemonWorker) qualifies(p *appsv1.DaemonSet) bool {
	return (len((*pw.ConfigManager).Get().NsToMonitor) == 0 ||
		utils.StringInSlice(p.Namespace, (*pw.ConfigManager).Get().NsToMonitor)) &&
		!utils.StringInSlice(p.Namespace, (*pw.ConfigManager).Get().NsToMonitorExclude)
}

func (dw *DaemonWorker) onNewDaemonSet(obj interface{}) {
	DaemonObj := obj.(*appsv1.DaemonSet)
	if !dw.qualifies(DaemonObj) {
		return
	}
	fmt.Printf("Added DaemonSet: %s\n", DaemonObj.Name)

	DaemonRecord, _ := dw.processObject(DaemonObj, nil)
	dw.WQ.Add(&DaemonRecord)
}

func (dw *DaemonWorker) onDeleteDaemonSet(obj interface{}) {
	DaemonObj := obj.(*appsv1.DaemonSet)
	if !dw.qualifies(DaemonObj) {
		return
	}
	fmt.Printf("Deleted DaemonSet: %s\n", DaemonObj.Name)
}

func (dw *DaemonWorker) onUpdateDaemonSet(objOld interface{}, objNew interface{}) {
	DaemonObj := objNew.(*appsv1.DaemonSet)
	if !dw.qualifies(DaemonObj) {
		return
	}
	fmt.Printf("DaemonSet %s changed\n", DaemonObj.Name)

	DaemonRecord, _ := dw.processObject(DaemonObj, nil)
	dw.WQ.Add(&DaemonRecord)

}

func (pw *DaemonWorker) startMetricsWorker(stopCh <-chan struct{}) {
	bag := (*pw.ConfigManager).Get()
	pw.appMetricTicker(stopCh, time.NewTicker(time.Duration(bag.MetricsSyncInterval)*time.Second))

}

func (pw *DaemonWorker) appMetricTicker(stop <-chan struct{}, ticker *time.Ticker) {
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

func (pw *DaemonWorker) eventQueueTicker(stop <-chan struct{}, ticker *time.Ticker) {
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

func (pw *DaemonWorker) startEventQueueWorker(stopCh <-chan struct{}) {
	bag := (*pw.ConfigManager).Get()
	pw.eventQueueTicker(stopCh, time.NewTicker(time.Duration(bag.SnapshotSyncInterval)*time.Second))
}

func (pw *DaemonWorker) flushQueue() {
	bag := (*pw.ConfigManager).Get()
	bth := pw.AppdController.StartBT("FlushDaemonSetDataQueue")
	count := pw.WQ.Len()
	if count > 0 {
		fmt.Printf("Flushing the queue of %d DaemonSet records\n", count)
	}
	if count == 0 {
		pw.AppdController.StopBT(bth)
		return
	}

	var objList []m.DaemonSchema

	var DaemonRecord *m.DaemonSchema
	var ok bool = true

	for count >= 0 {
		DaemonRecord, ok = pw.getNextQueueItem()
		count = count - 1
		if ok {
			objList = append(objList, *DaemonRecord)
		} else {
			fmt.Println("Queue shut down")
		}
		if count == 0 || len(objList) >= bag.EventAPILimit {
			fmt.Printf("Sending %d DaemonSet records to AppD events API\n", len(objList))
			pw.postDaemonRecords(&objList)
			pw.AppdController.StopBT(bth)
			return
		}
	}
	pw.AppdController.StopBT(bth)
}

func (pw *DaemonWorker) postDaemonRecords(objList *[]m.DaemonSchema) {
	bag := (*pw.ConfigManager).Get()
	logger := log.New(os.Stdout, "[APPD_CLUSTER_MONITOR]", log.Lshortfile)
	rc := app.NewRestClient(bag, logger)
	data, err := json.Marshal(objList)
	schemaDefObj := m.NewDaemonSchemaDefWrapper()
	schemaDef, e := json.Marshal(schemaDefObj)
	if err == nil && e == nil {
		if rc.SchemaExists(bag.DaemonSchemaName) == false {
			fmt.Printf("Creating schema. %s\n", bag.DaemonSchemaName)
			schemaObj, err := rc.CreateSchema(bag.DaemonSchemaName, schemaDef)
			if err != nil {
				return
			} else if schemaObj != nil {
				fmt.Printf("Schema %s created\n", bag.DaemonSchemaName)
			} else {
				fmt.Printf("Schema %s exists\n", bag.DaemonSchemaName)
			}
		}
		fmt.Println("About to post records")
		rc.PostAppDEvents(bag.DaemonSchemaName, data)
	} else {
		fmt.Printf("Problems when serializing array of pod schemas. %v\n", err)
	}
}

func (pw *DaemonWorker) getNextQueueItem() (*m.DaemonSchema, bool) {
	DaemonRecord, quit := pw.WQ.Get()

	if quit {
		return DaemonRecord.(*m.DaemonSchema), false
	}
	defer pw.WQ.Done(DaemonRecord)
	pw.WQ.Forget(DaemonRecord)

	return DaemonRecord.(*m.DaemonSchema), true
}

func (pw *DaemonWorker) buildAppDMetrics() {
	bth := pw.AppdController.StartBT("PostDaemonSetMetrics")
	pw.SummaryMap = make(map[string]m.ClusterDaemonMetrics)

	var count int = 0
	for _, obj := range pw.informer.GetStore().List() {
		DaemonObject := obj.(*appsv1.DaemonSet)
		DaemonSchema, _ := pw.processObject(DaemonObject, nil)
		pw.summarize(&DaemonSchema)
		count++
	}

	fmt.Printf("Unique DaemonSet metrics: %d\n", count)

	ml := pw.builAppDMetricsList()

	fmt.Printf("Ready to push %d DaemonSet metrics\n", len(ml.Items))

	pw.AppdController.PostMetrics(ml)
	pw.AppdController.StopBT(bth)
}

func (pw *DaemonWorker) summarize(DaemonObject *m.DaemonSchema) {
	bag := (*pw.ConfigManager).Get()
	//global metrics
	summary, okSum := pw.SummaryMap[m.ALL]
	if !okSum {
		summary = m.NewClusterDaemonMetrics(bag, m.ALL)
		pw.SummaryMap[m.ALL] = summary
	}

	//namespace metrics
	summaryNS, okNS := pw.SummaryMap[DaemonObject.Namespace]
	if !okNS {
		summaryNS = m.NewClusterDaemonMetrics(bag, DaemonObject.Namespace)
		pw.SummaryMap[DaemonObject.Namespace] = summaryNS
	}

	summary.DaemonCount++
	summaryNS.DaemonCount++

	summary.DaemonReplicasAvailable = summary.DaemonReplicasAvailable + int64(DaemonObject.ReplicasAvailable)
	summaryNS.DaemonReplicasAvailable = summaryNS.DaemonReplicasAvailable + int64(DaemonObject.ReplicasAvailable)

	summary.DaemonReplicasUnAvailable = summary.DaemonReplicasUnAvailable + int64(DaemonObject.ReplicasUnAvailable)
	summaryNS.DaemonReplicasUnAvailable = summaryNS.DaemonReplicasUnAvailable + int64(DaemonObject.ReplicasUnAvailable)

	summary.DaemonCollisionCount = summary.DaemonCollisionCount + int64(DaemonObject.CollisionCount)
	summaryNS.DaemonCollisionCount = summaryNS.DaemonCollisionCount + int64(DaemonObject.CollisionCount)

	summary.DaemonMissScheduled = summary.DaemonMissScheduled + int64(DaemonObject.MissScheduled)
	summaryNS.DaemonMissScheduled = summaryNS.DaemonMissScheduled + int64(DaemonObject.MissScheduled)

	pw.SummaryMap[m.ALL] = summary
	pw.SummaryMap[DaemonObject.Namespace] = summaryNS
}

func (pw *DaemonWorker) processObject(d *appsv1.DaemonSet, old *appsv1.DaemonSet) (m.DaemonSchema, bool) {
	changed := true
	bag := (*pw.ConfigManager).Get()

	DaemonObject := m.NewDaemonObj()
	DaemonObject.Name = d.Name
	DaemonObject.Namespace = d.Namespace

	if d.ClusterName != "" {
		DaemonObject.ClusterName = d.ClusterName
	} else {
		DaemonObject.ClusterName = bag.AppName
	}

	DaemonObject.ObjectUid = string(d.GetUID())
	DaemonObject.CreationTimestamp = d.GetCreationTimestamp().Time
	if d.GetDeletionTimestamp() != nil {
		DaemonObject.DeletionTimestamp = d.GetDeletionTimestamp().Time
	}
	DaemonObject.MinReadySecs = d.Spec.MinReadySeconds

	if d.Spec.RevisionHistoryLimit != nil {
		DaemonObject.RevisionHistoryLimits = *d.Spec.RevisionHistoryLimit
	}

	DaemonObject.MissScheduled = d.Status.NumberMisscheduled
	DaemonObject.DesiredNumber = d.Status.DesiredNumberScheduled

	DaemonObject.ReplicasAvailable = d.Status.NumberAvailable
	DaemonObject.ReplicasUnAvailable = d.Status.NumberUnavailable
	DaemonObject.UpdatedNumberScheduled = d.Status.UpdatedNumberScheduled
	if d.Status.CollisionCount != nil {
		DaemonObject.CollisionCount = *d.Status.CollisionCount
	}
	DaemonObject.ReplicasReady = d.Status.NumberReady

	return DaemonObject, changed
}

func (pw DaemonWorker) builAppDMetricsList() m.AppDMetricList {
	ml := m.NewAppDMetricList()
	var list []m.AppDMetric
	for _, metricNode := range pw.SummaryMap {
		objMap := metricNode.Unwrap()
		pw.addMetricToList(*objMap, metricNode, &list)
	}

	ml.Items = list
	return ml
}

func (pw DaemonWorker) addMetricToList(objMap map[string]interface{}, metric m.AppDMetricInterface, list *[]m.AppDMetric) {

	for fieldName, fieldValue := range objMap {
		if !metric.ShouldExcludeField(fieldName) {
			appdMetric := m.NewAppDMetric(fieldName, fieldValue.(int64), metric.GetPath())
			*list = append(*list, appdMetric)
		}
	}
}
