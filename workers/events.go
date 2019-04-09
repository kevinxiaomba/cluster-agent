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
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	m "github.com/sjeltuhin/clusterAgent/models"

	"github.com/sjeltuhin/clusterAgent/config"
	"github.com/sjeltuhin/clusterAgent/utils"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

var lockNSCache = sync.RWMutex{}
var lockTierCache = sync.RWMutex{}

type EventWorker struct {
	informer       cache.SharedIndexInformer
	Client         *kubernetes.Clientset
	ConfigManager  *config.MutexConfigManager
	SummaryMap     map[string]m.ClusterEventMetrics
	WQ             workqueue.RateLimitingInterface
	AppdController *app.ControllerClient
	PodsWorker     *PodWorker
}

func NewEventWorker(client *kubernetes.Clientset, cm *config.MutexConfigManager, appdController *app.ControllerClient, podsWorker *PodWorker) EventWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	ew := EventWorker{Client: client, ConfigManager: cm,
		AppdController: appdController, SummaryMap: make(map[string]m.ClusterEventMetrics), WQ: queue, PodsWorker: podsWorker}
	ew.informer = ew.initInformer(client)
	return ew
}

func (ew *EventWorker) initInformer(client *kubernetes.Clientset) cache.SharedIndexInformer {
	i := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.Core().Events(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.Core().Events(metav1.NamespaceAll).Watch(options)
			},
		},
		&v1.Event{},
		0,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	i.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ew.onNewEvent,
	})
	return i
}

func (pw *EventWorker) qualifies(p *v1.Event) bool {
	bag := (*pw.ConfigManager).Get()
	return (len(bag.NsToMonitor) == 0 ||
		utils.StringInSlice(p.Namespace, bag.NsToMonitor)) &&
		!utils.StringInSlice(p.Namespace, bag.NsToMonitorExclude)
}

func (ew *EventWorker) onNewEvent(obj interface{}) {
	eventObj := obj.(*v1.Event)
	if !ew.qualifies(eventObj) {
		return
	}
	//	fmt.Printf("Received event: %s %s %s\n", eventObj.Namespace, eventObj.Message, eventObj.Reason)
	eventRecord := ew.processObject(eventObj)
	ew.WQ.Add(&eventRecord)
}

func (ew *EventWorker) Observe(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	wg.Add(1)
	go ew.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, ew.HasSynced) {
		fmt.Errorf("Timed out waiting for events caches to sync")
	}
	fmt.Println("Cache syncronized. Starting events processing...")

	wg.Add(1)
	go ew.startMetricsWorker(stopCh)

	wg.Add(1)
	go ew.startEventQueueWorker(stopCh)

	<-stopCh
}

func (ew *EventWorker) HasSynced() bool {
	return ew.informer.HasSynced()
}

func (ew *EventWorker) startMetricsWorker(stopCh <-chan struct{}) {
	bag := (*ew.ConfigManager).Get()
	ew.appMetricTicker(stopCh, time.NewTicker(time.Duration(bag.MetricsSyncInterval)*time.Second))

}

func (ew *EventWorker) appMetricTicker(stop <-chan struct{}, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			ew.buildAppDMetrics()
		case <-stop:
			ticker.Stop()
			return
		}
	}
}

func (ew *EventWorker) eventQueueTicker(stop <-chan struct{}, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			ew.flushQueue()
		case <-stop:
			ticker.Stop()
			return
		}
	}
}

func (ew *EventWorker) startEventQueueWorker(stopCh <-chan struct{}) {
	bag := (*ew.ConfigManager).Get()
	ew.eventQueueTicker(stopCh, time.NewTicker(time.Duration(bag.SnapshotSyncInterval)*time.Second))
}

func (ew *EventWorker) flushQueue() {
	bag := (*ew.ConfigManager).Get()
	bth := ew.AppdController.StartBT("FlushEventDataQueue")
	count := ew.WQ.Len()
	if count > 0 {
		fmt.Printf("Flushing the queue of %d event records\n", count)
	} else {
		fmt.Println("Event queue empty")
	}
	if count == 0 {
		ew.AppdController.StopBT(bth)
		return
	}

	var objList []m.EventSchema

	var eventRecord *m.EventSchema
	var ok bool = true

	for count >= 0 {
		eventRecord, ok = ew.getNextQueueItem()
		count = count - 1
		if ok {
			objList = append(objList, *eventRecord)
		} else {
			fmt.Println("Queue shut down")
		}
		if count == 0 || len(objList) >= bag.EventAPILimit {
			fmt.Printf("Sending %d event records to AppD events API\n", len(objList))
			ew.postEventRecords(&objList)
			ew.AppdController.StopBT(bth)
			return
		}
	}
	ew.AppdController.StopBT(bth)
}

func (ew *EventWorker) postEventRecords(objList *[]m.EventSchema) {
	bag := (*ew.ConfigManager).Get()
	logger := log.New(os.Stdout, "[APPD_CLUSTER_MONITOR]", log.Lshortfile)
	rc := app.NewRestClient(bag, logger)

	schemaDefObj := m.NewEventSchemaDefWrapper()

	err := rc.EnsureSchema(bag.EventSchemaName, &schemaDefObj)
	if err != nil {
		fmt.Printf("Issues when ensuring %s schema. %v\n", bag.EventSchemaName, err)
	} else {
		data, err := json.Marshal(objList)
		if err != nil {
			fmt.Printf("Problems when serializing array of event schemas. %v", err)
		}
		rc.PostAppDEvents(bag.EventSchemaName, data)
	}
}

func (ew *EventWorker) getNextQueueItem() (*m.EventSchema, bool) {
	eventRecord, quit := ew.WQ.Get()

	if quit {
		return eventRecord.(*m.EventSchema), false
	}
	defer ew.WQ.Done(eventRecord)
	ew.WQ.Forget(eventRecord)

	return eventRecord.(*m.EventSchema), true
}

func (ew *EventWorker) buildAppDMetrics() {
	bag := (*ew.ConfigManager).Get()
	bth := ew.AppdController.StartBT("PostEventMetrics")
	ew.SummaryMap = make(map[string]m.ClusterEventMetrics)

	var count int = 0
	toDelete := []interface{}{}
	for _, obj := range ew.informer.GetStore().List() {
		eventObject := obj.(*v1.Event)
		eventSchema := ew.processObject(eventObject)
		ew.summarize(&eventSchema)
		count++
		toDelete = append(toDelete, obj)
	}

	fmt.Printf("Unique event metrics: %d\n", count)

	//add 0 values for missing entities
	if len(ew.SummaryMap) == 0 {
		//cluster
		summary := m.NewClusterEventMetrics(bag, m.ALL, m.ALL)
		ew.SummaryMap[m.ALL] = summary
	}
	//check for missing namespaces
	nsMap := ew.PodsWorker.GetKnownNamespaces()
	lockNSCache.RLock()
	for ns, _ := range nsMap {
		if _, ok := ew.SummaryMap[ns]; !ok {
			summaryNS := m.NewClusterEventMetrics(bag, ns, m.ALL)
			ew.SummaryMap[ns] = summaryNS
		}
	}
	lockNSCache.RUnlock()

	//check for missing deployments
	tierMap := ew.PodsWorker.GetKnownDeployments()
	lockTierCache.RLock()
	for key, tierName := range tierMap {
		if _, ok := ew.SummaryMap[key]; !ok {
			ns, _ := utils.SplitPodKey(key)
			emptyMetrics := m.NewClusterEventMetrics(bag, ns, tierName)
			ew.SummaryMap[tierName] = emptyMetrics
		}
	}
	lockTierCache.RUnlock()

	ml := ew.builAppDMetricsList()

	//clear cache
	for _, old := range toDelete {
		ew.informer.GetStore().Delete(old)
	}

	fmt.Printf("Ready to push %d node metrics\n", len(ml.Items))

	ew.AppdController.PostMetrics(ml)
	ew.AppdController.StopBT(bth)
}

func (ew *EventWorker) processObject(e *v1.Event) m.EventSchema {
	bag := (*ew.ConfigManager).Get()
	eventObject := m.NewEventObj()

	if e.ClusterName != "" {
		eventObject.ClusterName = e.ClusterName
	} else {
		eventObject.ClusterName = bag.AppName
	}

	eventObject.Count = e.Count
	eventObject.CreationTimestamp = e.CreationTimestamp.Time
	if e.DeletionTimestamp != nil {
		eventObject.DeletionTimestamp = e.DeletionTimestamp.Time
	}

	eventObject.GenerateName = e.GenerateName
	eventObject.Generation = e.Generation

	eventObject.LastTimestamp = e.LastTimestamp.Time
	eventObject.Message = e.Message
	eventObject.Name = e.Name
	eventObject.Namespace = e.Namespace
	eventObject.ObjectKind = e.InvolvedObject.Kind
	eventObject.ObjectName = e.InvolvedObject.Name
	eventObject.ObjectNamespace = e.ObjectMeta.GetNamespace()
	eventObject.ObjectResourceVersion = e.ObjectMeta.GetResourceVersion()
	eventObject.ObjectUid = string(e.GetObjectMeta().GetUID())
	for _, r := range e.GetOwnerReferences() {
		eventObject.OwnerReferences += r.Name + ";"
	}

	eventObject.Reason = e.Reason
	eventObject.Type = e.Type
	eventObject.Count = e.Count
	eventObject.ResourceVersion = e.GetResourceVersion()
	eventObject.SelfLink = e.GetSelfLink()
	eventObject.SourceComponent = e.Source.Component
	eventObject.SourceHost = e.Source.Host

	cat, sub := ew.GetEventCategory(&eventObject)
	eventObject.Category = cat
	eventObject.SubCategory = sub

	if ew.PodsWorker != nil && eventObject.ObjectKind == "Pod" && eventObject.Category == "error" {
		elapsedTime := time.Now().Sub(eventObject.LastTimestamp)
		ew.PodsWorker.OnPodrrorEvent(eventObject.ObjectName, eventObject.Namespace, eventObject.Reason, int64(elapsedTime))
	}

	return eventObject
}

func buildTierKeyForEvent(namespace, tierName string) string {
	return fmt.Sprintf("%s/%s", namespace, tierName)
}

func (ew *EventWorker) summarize(eventObject *m.EventSchema) {
	bag := (*ew.ConfigManager).Get()
	//	var err error = nil
	var tierName string = ""
	var tierKey string = ""
	if ew.PodsWorker != nil && eventObject.ObjectKind == "Pod" {
		_, tierName, _ = ew.PodsWorker.GetCachedPod(eventObject.ObjectNamespace, eventObject.ObjectName)
		//		if err != nil {
		//			fmt.Printf("Unable to lookup Pod %s for event. %v\n", eventObject.ObjectName, err)
		//		} else {
		//			fmt.Printf("Found Pod %s for event. %s\n", eventObject.ObjectName, eventObject.Reason)
		//		}
	}
	//global metrics
	summary, okSum := ew.SummaryMap[m.ALL]
	if !okSum {
		summary = m.NewClusterEventMetrics(bag, m.ALL, m.ALL)
		ew.SummaryMap[m.ALL] = summary
	}

	//node namespace
	summaryNS, okNS := ew.SummaryMap[eventObject.Namespace]
	if !okNS {
		summaryNS = m.NewClusterEventMetrics(bag, eventObject.Namespace, m.ALL)
		ew.SummaryMap[eventObject.Namespace] = summaryNS
	}

	var summaryTier *m.ClusterEventMetrics = nil

	var okTier bool = false
	//node metrics
	if tierName != "" {
		tierKey = buildTierKeyForEvent(eventObject.ObjectNamespace, tierName)
		var tierObj m.ClusterEventMetrics
		tierObj, okTier = ew.SummaryMap[tierKey]
		if !okTier {
			tierObj = m.NewClusterEventMetrics(bag, eventObject.Namespace, tierName)
			ew.SummaryMap[tierKey] = tierObj
		}
		summaryTier = &tierObj
	}
	summary.EventCount++
	summaryNS.EventCount++

	if summaryTier != nil {
		summaryTier.EventCount++
	}

	cat := eventObject.Category
	sub := eventObject.SubCategory

	if cat == "error" {
		summary.EventError++
		summaryNS.EventError++
		if summaryTier != nil {
			summaryTier.EventError++
		}
	} else {
		summary.EventInfo++
		summaryNS.EventInfo++
		if summaryTier != nil {
			summaryTier.EventInfo++
		}
		if eventObject.Reason == "Pulling" {
			summary.ImagePulls++
			summaryNS.ImagePulls++
			if summaryTier != nil {
				summaryTier.ImagePulls++
			}
		}
	}
	switch sub {
	case "pod":
		summary.PodIssues++
		summaryNS.PodIssues++
		if summaryTier != nil {
			summaryTier.PodIssues++
		}
		break
	case "image":
		summary.ImagePullErrors++
		summaryNS.ImagePullErrors++
		if summaryTier != nil {
			summaryTier.ImagePullErrors++
		}
		break
	case "reduction":
		summary.ScaleDowns++
		summaryNS.ScaleDowns++
		if summaryTier != nil {
			summaryTier.ScaleDowns++
		}
		break
	case "storage":
		summary.StorageIssues++
		summaryNS.StorageIssues++
		if summaryTier != nil {
			summaryTier.StorageIssues++
		}
		break
	case "eviction":
		summary.EvictionThreats++
		summaryNS.EvictionThreats++
		if summaryTier != nil {
			summaryTier.EvictionThreats++
		}
		break

	}

	ew.SummaryMap[m.ALL] = summary
	ew.SummaryMap[eventObject.Namespace] = summaryNS
	if summaryTier != nil {
		ew.SummaryMap[tierKey] = *summaryTier
	}
}

func (ew *EventWorker) builAppDMetricsList() m.AppDMetricList {
	ml := m.NewAppDMetricList()
	var list []m.AppDMetric
	for _, metricEvent := range ew.SummaryMap {
		objMap := metricEvent.Unwrap()
		ew.addMetricToList(*objMap, metricEvent, &list)
	}

	ml.Items = list
	return ml
}

func (ew *EventWorker) addMetricToList(objMap map[string]interface{}, metric m.AppDMetricInterface, list *[]m.AppDMetric) {

	for fieldName, fieldValue := range objMap {
		if !metric.ShouldExcludeField(fieldName) {
			appdMetric := m.NewAppDMetric(fieldName, fieldValue.(int64), metric.GetPath())
			*list = append(*list, appdMetric)
		}
	}
}

//custom events
func EmitInstrumentationEvent(pod *v1.Pod, client *kubernetes.Clientset, reason, message, eventType string) error {
	fmt.Printf("About to emit event: %s %s %s for pod %s-%s\n", reason, message, eventType, pod.Namespace, pod.Name)
	event := eventFromPod(pod, reason, message, eventType)
	_, err := client.Core().Events(pod.Namespace).Create(event)
	if err != nil {
		fmt.Printf("Issues when emitting instrumentation event %v\n", err)
	}
	return err
}

func eventFromPod(podObj *v1.Pod, reason string, message string, eventType string) *v1.Event {
	or := v1.ObjectReference{Kind: "Pods", Namespace: podObj.Namespace, Name: podObj.Name}
	source := v1.EventSource{Component: "AppDClusterAgent", Host: podObj.Spec.NodeName}
	t := metav1.Time{Time: time.Now()}
	namespace := podObj.Namespace
	if namespace == "" {
		namespace = metav1.NamespaceDefault
	}
	return &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("%v.%x", podObj.Name, t.UnixNano()),
			Namespace:   namespace,
			Annotations: podObj.Annotations,
		},
		InvolvedObject: or,
		Reason:         reason,
		Message:        message,
		FirstTimestamp: t,
		LastTimestamp:  t,
		Count:          1,
		Type:           eventType,
		Source:         source,
	}

}

func (ew *EventWorker) GetEventCategory(eventSchema *m.EventSchema) (string, string) {
	msg := eventSchema.Message
	cat := "info"
	sub := ""
	switch eventSchema.Reason {
	case "Created":
	case "Started":
	case "Pulling":
	case "Pulled":
	case "NodeReady":
	case "NodeSchedulable":
	case "VolumeResizeSuccessful":
	case "FileSystemResizeSuccessful":
	case "AlreadyMountedVolume":
	case "SuccessfulDetachVolume":
	case "SuccessfulAttachVolume":
	case "SuccessfulMountVolume":
	case "SuccessfulUnMountVolume":
	case "Rebooted":
		break
	case "NodeAllocatableEnforced":
		cat = "error"
		break
	case "ScalingReplicaSet":
		if strings.Contains(msg, "Scaled down") {
			sub = "reduction"
			cat = "error"
		} else {
			cat = "info"
		}
		break
	case "Killing":
		sub = "reduction"
		cat = "error"
		break
	case "Failed":
		if strings.Contains(msg, "ImagePullBackOff") {
			sub = "image"
		} else {
			sub = "pod"
		}
		cat = "error"
		break
	case "Preempting":
		cat = "error"
		break
	case "BackOff":
		if strings.Contains(msg, "image") {
			sub = "image"
		}
		if strings.Contains(msg, "container") {
			sub = "pod"
		}
		cat = "error"
		break
	case "FailedCreate":
		if strings.Contains(msg, "quota") {
			sub = "quota"
		} else {
			sub = "pod"
		}
		cat = "error"
		break
	case "ExceededGracePeriod":
	case "FailedKillPod":
		sub = "pod"
		cat = "error"
		break
	case "FailedCreatePodContainer":
		sub = "pod"
		cat = "error"
		break
	case "NetworkNotReady":
	case "InspectFailed":
		cat = "error"
		break
	case "ErrImageNeverPull":
		sub = "image"
		cat = "error"
		break
	case "NodeNotReady":
	case "NodeNotSchedulable":
	case "KubeletSetupFailed":
		cat = "error"
		break
	case "FailedAttachVolume":
		sub = "storage"
		cat = "error"
		break
	case "FailedDetachVolume":
		sub = "storage"
		cat = "error"
		break
	case "FailedMount":
		sub = "storage"
		cat = "error"
		break
	case "FailedBinding":
		sub = "storage"
		cat = "error"
		break
	case "VolumeResizeFailed":
		sub = "storage"
		cat = "error"
		break
	case "FileSystemResizeFailed":
		sub = "storage"
		cat = "error"
		break
	case "FailedUnMount":
		sub = "storage"
		cat = "error"
		break
	case "FailedMapVolume":
		sub = "storage"
		cat = "error"
		break
	case "FailedUnmapDevice":
		sub = "storage"
		cat = "error"
		break
	case "HostPortConflict":
	case "NodeSelectorMismatching":
		cat = "error"
		break
	case "ContainerGCFailed":
		sub = "pod"
		cat = "error"
		break
	case "ImageGCFailed":
		sub = "image"
		cat = "error"
		break
	case "FailedNodeAllocatableEnforcement":
		cat = "error"
		break
	case "UnsupportedMountOption":
		sub = "storage"
		cat = "error"
		break
	case "SandboxChanged":
		sub = "pod"
		cat = "error"
		break
	case "FailedCreatePodSandBox":
		sub = "pod"
		cat = "error"
		break
	case "FailedPodSandBoxStatus":
		sub = "pod"
		cat = "error"
		break
	case "InvalidDiskCapacity":
		sub = "storage"
		cat = "error"
		break
	case "FreeDiskSpaceFailed":
		sub = "storage"
		cat = "error"
		break
	case "Unhealthy":
		sub = "pod"
		cat = "error"
		break
	case "FailedSync":
	case "FailedValidation":
	case "FailedPostStartHook":
	case "FailedPreStopHook":
	case "UnfinishedPreStopHook":
		cat = "error"
		break
	case "Evicted":
		sub = "eviction"
	case "InsufficientFreeCPU":
	case "InsufficientFreeMemory":
	case "FailedDaemonPod":
	case "NodeHasDiskPressure":
	case "EvictionThresholdMet":
	case "ErrorReconciliationRetryTimeout":

		cat = "error"
		break
	}
	return cat, sub
}
