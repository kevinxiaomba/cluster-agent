package workers

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/appdynamics/cluster-agent/config"
	m "github.com/appdynamics/cluster-agent/models"
	"github.com/appdynamics/cluster-agent/utils"

	app "github.com/appdynamics/cluster-agent/appd"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type NodesWorker struct {
	informer       cache.SharedIndexInformer
	Client         *kubernetes.Clientset
	ConfigManager  *config.MutexConfigManager
	SummaryMap     map[string]m.ClusterNodeMetrics
	WQ             workqueue.RateLimitingInterface
	AppdController *app.ControllerClient
	CapacityMap    map[string]m.NodeSchema
	Logger         *log.Logger
}

var lockCapacityMap = sync.RWMutex{}

func NewNodesWorker(client *kubernetes.Clientset, cm *config.MutexConfigManager, controller *app.ControllerClient, l *log.Logger) NodesWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	pw := NodesWorker{Client: client, ConfigManager: cm, SummaryMap: make(map[string]m.ClusterNodeMetrics), WQ: queue, AppdController: controller,
		CapacityMap: make(map[string]m.NodeSchema), Logger: l}
	pw.initNodeInformer(client)
	return pw
}

func (nw *NodesWorker) initNodeInformer(client *kubernetes.Clientset) cache.SharedIndexInformer {
	i := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.CoreV1().Nodes().List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.CoreV1().Nodes().Watch(options)
			},
		},
		&v1.Node{},
		0,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	i.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    nw.onNewNode,
		DeleteFunc: nw.onDeleteNode,
		UpdateFunc: nw.onUpdateNode,
	})
	nw.informer = i

	return i
}

func (pw *NodesWorker) qualifies(p *v1.Node) bool {
	bag := (*pw.ConfigManager).Get()
	return (len(bag.NodesToMonitor) == 0 ||
		utils.StringInSlice(p.Name, bag.NodesToMonitor)) &&
		!utils.StringInSlice(p.Name, bag.NodesToMonitorExclude)
}

func (nw *NodesWorker) onNewNode(obj interface{}) {
	nodeObj := obj.(*v1.Node)
	if !nw.qualifies(nodeObj) {
		return
	}
	nw.Logger.Debugf("Added Node: %s\n", nodeObj.Name)
	nodeRecord, _ := nw.processObject(nodeObj, nil)
	nw.WQ.Add(&nodeRecord)

}

func (nw *NodesWorker) onDeleteNode(obj interface{}) {
	nodeObj := obj.(*v1.Node)
	if !nw.qualifies(nodeObj) {
		return
	}
	nw.Logger.Debugf("Deleted Node: %s\n", nodeObj.Name)
	nodeRecord, _ := nw.processObject(nodeObj, nil)
	nw.WQ.Add(&nodeRecord)
}

func (nw *NodesWorker) onUpdateNode(objOld interface{}, objNew interface{}) {
	nodeObj := objNew.(*v1.Node)
	if !nw.qualifies(nodeObj) {
		return
	}
	nodeOldObj := objOld.(*v1.Node)
	if m.CompareNodeObjects(nodeObj, nodeOldObj) == true {
		nodeRecord, changed := nw.processObject(nodeObj, nodeOldObj)

		if changed {
			nw.Logger.Debugf("Node changed: %s\n", nodeObj.Name)
			nw.WQ.Add(&nodeRecord)
		}
	}
}

func (pw NodesWorker) Observe(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	defer pw.WQ.ShutDown()
	wg.Add(1)
	go pw.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, pw.HasSynced) {
		pw.Logger.Errorf("Timed out waiting for node caches to sync")
	}
	pw.Logger.Info("Node Cache synchronized. Starting node processing...")

	wg.Add(1)
	go pw.startMetricsWorker(stopCh)

	wg.Add(1)
	go pw.startEventQueueWorker(stopCh)

	<-stopCh
}

func (pw *NodesWorker) HasSynced() bool {
	return pw.informer.HasSynced()
}

func (pw *NodesWorker) startMetricsWorker(stopCh <-chan struct{}) {
	bag := (*pw.ConfigManager).Get()
	pw.appMetricTicker(stopCh, time.NewTicker(time.Duration(bag.MetricsSyncInterval)*time.Second))

}

func (pw *NodesWorker) appMetricTicker(stop <-chan struct{}, ticker *time.Ticker) {
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

func (pw *NodesWorker) eventQueueTicker(stop <-chan struct{}, ticker *time.Ticker) {
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

func (pw *NodesWorker) startEventQueueWorker(stopCh <-chan struct{}) {
	//wait.Until(pw.flushQueue, 30*time.Second, stopCh)
	pw.eventQueueTicker(stopCh, time.NewTicker(15*time.Second))
}

func (pw *NodesWorker) flushQueue() {
	bag := (*pw.ConfigManager).Get()
	bth := pw.AppdController.StartBT("FlushNodeDataQueue")
	count := pw.WQ.Len()
	if count > 0 {
		pw.Logger.Infof("Flushing the queue of %d node records\n", count)
	}
	if count == 0 {
		pw.AppdController.StopBT(bth)
		return
	}

	var objList []m.NodeSchema

	var nodeRecord *m.NodeSchema
	var ok bool = true

	for count >= 0 {
		nodeRecord, ok = pw.getNextQueueItem()
		count = count - 1
		if ok {
			objList = append(objList, *nodeRecord)
		} else {
			pw.Logger.Infof("Queue shut down")
		}
		if count == 0 || len(objList) >= bag.EventAPILimit {
			pw.Logger.Debugf("Sending %d node records to AppD events API\n", len(objList))
			pw.postNodeRecords(&objList)
			pw.AppdController.StopBT(bth)
			return
		}
	}
	pw.AppdController.StopBT(bth)
}

func (pw *NodesWorker) postNodeRecords(objList *[]m.NodeSchema) {
	bag := (*pw.ConfigManager).Get()

	rc := app.NewRestClient(bag, pw.Logger)

	schemaDefObj := m.NewNodeSchemaDefWrapper()
	err := rc.EnsureSchema(bag.NodeSchemaName, &schemaDefObj)
	if err != nil {
		pw.Logger.Errorf("Issues when ensuring %s schema. %v\n", bag.NodeSchemaName, err)
	} else {
		data, err := json.Marshal(objList)
		if err != nil {
			pw.Logger.Errorf("Problems when serializing array of node schemas. %v", err)
		}
		rc.PostAppDEvents(bag.NodeSchemaName, data)
	}
}

func (pw *NodesWorker) getNextQueueItem() (*m.NodeSchema, bool) {
	nodeRecord, quit := pw.WQ.Get()

	if quit {
		return nodeRecord.(*m.NodeSchema), false
	}
	defer pw.WQ.Done(nodeRecord)
	pw.WQ.Forget(nodeRecord)

	return nodeRecord.(*m.NodeSchema), true
}

func (pw *NodesWorker) buildAppDMetrics() {
	bth := pw.AppdController.StartBT("PostNodeMetrics")
	pw.SummaryMap = make(map[string]m.ClusterNodeMetrics)

	count := 0
	for _, obj := range pw.informer.GetStore().List() {
		nodeObject := obj.(*v1.Node)
		if !pw.qualifies(nodeObject) {
			continue
		}
		nodeSchema, _ := pw.processObject(nodeObject, nil)
		pw.summarize(&nodeSchema)
		count++
	}

	if len(pw.SummaryMap) == 0 {
		bag := (*pw.ConfigManager).Get()
		pw.SummaryMap[m.ALL] = m.NewClusterNodeMetrics(bag, m.ALL)
	}

	ml := pw.builAppDMetricsList()

	pw.Logger.Infof("Ready to push %d Node metrics\n", len(ml.Items))

	pw.AppdController.PostMetrics(ml)
	pw.AppdController.StopBT(bth)
}

func (pw *NodesWorker) summarize(nodeObject *m.NodeSchema) {
	bag := (*pw.ConfigManager).Get()
	//global metrics
	summary, okSum := pw.SummaryMap[m.ALL]
	if !okSum {
		summary = m.NewClusterNodeMetrics(bag, m.ALL)
		pw.SummaryMap[m.ALL] = summary
	}

	//node metrics
	summaryNode, okNode := pw.SummaryMap[nodeObject.NodeName]
	if !okNode {
		summaryNode = m.NewClusterNodeMetrics(bag, nodeObject.NodeName)
		pw.SummaryMap[nodeObject.NodeName] = summaryNode
	}

	if nodeObject.DiskPressure == "true" {
		summary.DiskPressureNodes++
	}
	if nodeObject.MemoryPressure == "true" {
		summary.MemoryPressureNodes++
	}
	if nodeObject.OutOfDisk == "true" {
		summary.OutOfDiskNodes++
	}
	if nodeObject.Ready == "true" {
		summary.ReadyNodes++
	}
	if nodeObject.Role == "master" {
		summary.Masters++
	} else {
		summary.Workers++
	}

	summary.TaintsTotal += int64(nodeObject.TaintsNumber)

	summary.CapacityCpu += nodeObject.CpuCapacity
	summary.CapacityMemory += nodeObject.MemCapacity
	summary.CapacityPods += nodeObject.PodCapacity
	summary.AllocationsCpu += nodeObject.CpuAllocations
	summary.AllocationsMemory += nodeObject.MemAllocations

	summary.UseCpu += nodeObject.CpuUse
	summary.UseMemory += nodeObject.MemUse

	summaryNode.CapacityCpu = nodeObject.CpuCapacity
	summaryNode.CapacityMemory = nodeObject.MemCapacity
	summaryNode.CapacityPods = nodeObject.PodCapacity
	summaryNode.AllocationsCpu = nodeObject.CpuAllocations
	summaryNode.AllocationsMemory = nodeObject.MemAllocations

	summaryNode.UseCpu = nodeObject.CpuUse
	summaryNode.UseMemory = nodeObject.MemUse

	pw.SummaryMap[m.ALL] = summary
	pw.SummaryMap[nodeObject.NodeName] = summaryNode
	pw.updateCapcityMap(*nodeObject)
}

func (pw *NodesWorker) updateCapcityMap(node m.NodeSchema) {
	lockCapacityMap.Lock()
	defer lockCapacityMap.Unlock()
	pw.CapacityMap[node.NodeName] = node
}

func (pw *NodesWorker) GetNodeData(nodeName string) *m.NodeSchema {
	lockCapacityMap.RLock()
	defer lockCapacityMap.RUnlock()
	node := pw.CapacityMap[nodeName]
	return &node
}

func (pw *NodesWorker) processObject(n *v1.Node, old *v1.Node) (m.NodeSchema, bool) {
	changed := true

	nodeObject := m.NewNodeObj()
	nodeObject.NodeName = n.Name
	nodeObject.PodCIDR = n.Spec.PodCIDR

	var sb strings.Builder
	for _, t := range n.Spec.Taints {
		fmt.Fprintf(&sb, "%s;", t.ToString())
		nodeObject.TaintsNumber++
	}
	nodeObject.Taints = sb.String()
	sb.Reset()

	nodeObject.Phase = string(n.Status.Phase)

	for _, a := range n.Status.Addresses {
		fmt.Fprintf(&sb, "%s;", a.Address)
	}
	nodeObject.Addresses = sb.String()
	sb.Reset()

	isMaster := false
	for k, l := range n.Labels {
		fmt.Fprintf(&sb, "%s:%s;", k, l)
		if !isMaster && strings.Contains(k, "node-role.kubernetes.io/master") {
			isMaster = true
		}
	}
	nodeObject.Labels = sb.String()
	sb.Reset()
	if isMaster {
		nodeObject.Role = "master"
	} else {
		nodeObject.Role = "worker"
	}

	for key, c := range n.Status.Capacity {
		if key == "memory" {
			//MB
			nodeObject.MemCapacity = c.Value()
		}
		if key == "cpu" {
			nodeObject.CpuCapacity = c.Value() * 1000 //milli
		}
		if key == "pods" {
			nodeObject.PodCapacity = c.Value()
		}
	}

	for k, a := range n.Status.Allocatable {
		if k == "memory" {
			//MB
			nodeObject.MemAllocations = a.Value()
		}
		if k == "cpu" {
			nodeObject.CpuAllocations = a.Value() * 1000 //milli
		}
		if k == "pods" {
			nodeObject.PodAllocations = a.Value()
		}
	}

	nodeObject.KubeletPort = n.Status.DaemonEndpoints.KubeletEndpoint.Port
	nodeObject.OsArch = n.Status.NodeInfo.Architecture
	nodeObject.KubeletVersion = n.Status.NodeInfo.KubeletVersion
	nodeObject.RuntimeVersion = n.Status.NodeInfo.ContainerRuntimeVersion
	nodeObject.MachineID = n.Status.NodeInfo.MachineID
	nodeObject.OsName = n.Status.NodeInfo.OperatingSystem

	for _, cond := range n.Status.Conditions {
		if cond.Type == "Ready" {
			status := strings.ToLower(string(cond.Status))
			nodeObject.Ready = status
		}
		if cond.Type == "OutOfDisk" {
			status := strings.ToLower(string(cond.Status))
			nodeObject.OutOfDisk = status
		}
		if cond.Type == "MemoryPressure" {
			status := strings.ToLower(string(cond.Status))
			nodeObject.MemoryPressure = status
		}
		if cond.Type == "DiskPressure" {
			status := strings.ToLower(string(cond.Status))
			nodeObject.DiskPressure = status
		}
	}

	for _, av := range n.Status.VolumesAttached {
		fmt.Fprintf(&sb, "%s:%s;", av.Name, av.DevicePath)
	}
	nodeObject.AttachedVolumes = sb.String()
	sb.Reset()

	for _, vu := range n.Status.VolumesInUse {
		fmt.Fprintf(&sb, "%s;", vu)
	}
	nodeObject.VolumesInUse = sb.String()
	sb.Reset()

	nodeMetricsObj := pw.GetNodeMetricsSingle(n.Name)
	if nodeMetricsObj != nil {
		usageObj := nodeMetricsObj.GetNodeUsage()

		nodeObject.CpuUse = usageObj.CPU
		nodeObject.MemUse = usageObj.Memory
	}

	return nodeObject, changed
}

func (pw NodesWorker) GetNodeMetricsSingle(podName string) *m.NodeMetricsObj {

	metricsDone := make(chan *m.NodeMetricsObj)
	go metricsWorkerSingleNode(metricsDone, pw.Client, podName)

	metricsData := <-metricsDone
	return metricsData
}

func metricsWorkerSingleNode(finished chan *m.NodeMetricsObj, client *kubernetes.Clientset, nodeName string) {
	var path string = ""
	var metricsObj m.NodeMetricsObj
	if nodeName != "" {
		path = fmt.Sprintf("apis/metrics.k8s.io/v1beta1/nodes/%s", nodeName)

		data, err := client.RESTClient().Get().AbsPath(path).DoRaw()
		if err != nil {
			fmt.Printf("Issues when requesting metrics from metrics with path %s from server %s\n", path, err.Error())
		}

		merde := json.Unmarshal(data, &metricsObj)
		if merde != nil {
			fmt.Printf("Unmarshal issues. %v\n", merde)
		}
	}
	finished <- &metricsObj
}

func (pw NodesWorker) builAppDMetricsList() m.AppDMetricList {
	ml := m.NewAppDMetricList()
	var list []m.AppDMetric
	for _, metricNode := range pw.SummaryMap {
		objMap := metricNode.Unwrap()
		pw.addMetricToList(*objMap, metricNode, &list)
	}

	ml.Items = list
	return ml
}

func (pw NodesWorker) addMetricToList(objMap map[string]interface{}, metric m.AppDMetricInterface, list *[]m.AppDMetric) {

	for fieldName, fieldValue := range objMap {
		if !metric.ShouldExcludeField(fieldName) {
			appdMetric := m.NewAppDMetric(fieldName, fieldValue.(int64), metric.GetPath())
			*list = append(*list, appdMetric)
		}
	}
}
