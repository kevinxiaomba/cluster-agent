package workers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"time"

	"github.com/fatih/structs"
	m "github.com/sjeltuhin/clusterAgent/models"

	//	"github.com/sjeltuhin/clusterAgent/watchers"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	app "github.com/sjeltuhin/clusterAgent/appd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"
)

type PodWorker struct {
	informer   cache.SharedIndexInformer
	Client     *kubernetes.Clientset
	Bag        *m.AppDBag
	SummaryMap map[string]m.ClusterPodMetrics
	WQ         workqueue.RateLimitingInterface
}

func NewPodWorker(client *kubernetes.Clientset, bag *m.AppDBag) PodWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	pw := PodWorker{Client: client, Bag: bag, SummaryMap: make(map[string]m.ClusterPodMetrics), WQ: queue}
	pw.initPodInformer(client)
	return pw
}

func (pw *PodWorker) initPodInformer(client *kubernetes.Clientset) cache.SharedIndexInformer {
	i := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.Core().Pods(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.Core().Pods(metav1.NamespaceAll).Watch(options)
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

func (pw *PodWorker) onNewPod(obj interface{}) {
	podObj := obj.(*v1.Pod)
	fmt.Printf("Added Pod: %s %s\n", podObj.Namespace, podObj.Name)
	podRecord := pw.processObject(podObj)
	pw.WQ.Add(&podRecord)
}

func (pw *PodWorker) flushQueue() {
	fmt.Println("Time to flush the queue")
	var objList []m.PodSchema

	var podRecord *m.PodSchema
	var ok bool = true
	count := 0
	for ok == true {
		podRecord, ok = pw.getNextQueueItem()
		objList = append(objList, *podRecord)
		count++
		if count >= pw.Bag.EventAPILimit {
			ok = false
		}
	}
	fmt.Printf("Flushing queue with %d records", count)
	logger := log.New(os.Stdout, "[APPD_CLUSTER_MONITOR]", log.Lshortfile)
	rc := app.NewRestClient(pw.Bag, logger)
	data, err := json.Marshal(objList)
	schemaDef, e := json.Marshal(m.PodSchemaDefWrapper{})
	if err == nil && e == nil {
		if rc.SchemaExists(pw.Bag.PodSchemaName) == false {
			fmt.Printf("Creating schema. %s", pw.Bag.PodSchemaName)
			rc.CreateSchema(pw.Bag.PodSchemaName, schemaDef)
			fmt.Printf("Schema %s created", pw.Bag.PodSchemaName)
		} else {
			fmt.Printf("Schema %s exists", pw.Bag.PodSchemaName)
		}
		rc.PostAppDEvents(pw.Bag.PodSchemaName, data)
	} else {
		fmt.Printf("Problems when serializing array of pod schemas. %v", err)
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
	podObj := obj.(*v1.Pod)
	fmt.Printf("Deleted Pod: %s %s\n", podObj.Namespace, podObj.Name)
}

func (pw *PodWorker) onUpdatePod(objOld interface{}, objNew interface{}) {
	podObj := objNew.(*v1.Pod)
	fmt.Printf("Pod changed: %s %s\n", podObj.Namespace, podObj.Name)
}

func (pw PodWorker) Observe(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	defer pw.WQ.ShutDown()
	wg.Add(1)

	go pw.informer.Run(stopCh)

	wg.Add(1)

	go pw.appMetricTicker(stopCh, time.NewTicker(15*time.Second))

	//	wg.Add(1)

	//	go pw.eventQueueTicker(stopCh, time.NewTicker(30*time.Second))

	<-stopCh
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

func (pw *PodWorker) buildAppDMetrics() {
	pw.SummaryMap = make(map[string]m.ClusterPodMetrics)
	fmt.Println("Time to send metrics. Current cache:")
	var count int = 0
	for _, obj := range pw.informer.GetStore().List() {
		podObject := obj.(*v1.Pod)
		pw.processObject(podObject)
		fmt.Printf("Pod %s\n", podObject.Name)
		count++
	}
	fmt.Printf("Total: %d\n", count)

	ml := pw.builAppDMetricsList()

	fmt.Printf("Ready to push %d metrics\n", len(ml.Items))
	logger := log.New(os.Stdout, "[APPD_CLUSTER_MONITOR]", log.Lshortfile)
	appdController := app.NewControllerClient(pw.Bag, logger)
	appdController.PostMetrics(ml)
}

func (pw *PodWorker) processObject(p *v1.Pod) m.PodSchema {
	podObject := m.NewPodObj()
	if p.ClusterName != "" {
		podObject.ClusterName = p.ClusterName
	} else {
		podObject.ClusterName = pw.Bag.AppName
	}
	podObject.Name = p.Name
	podObject.Namespace = p.Namespace
	podObject.NodeName = p.Spec.NodeName

	podMetricsObj := pw.GetPodMetricsSingle(p.Namespace, p.Name)

	//global metrics
	summary, ok := pw.SummaryMap[m.ALL]
	if !ok {
		summary = m.NewClusterPodMetrics(m.ALL, m.ALL)
		pw.SummaryMap[m.ALL] = summary
	}

	//namespace metrics
	summaryNode, ok := pw.SummaryMap[podObject.NodeName]
	if !ok {
		summaryNode = m.NewClusterPodMetrics(m.ALL, podObject.NodeName)
		pw.SummaryMap[podObject.NodeName] = summaryNode
	}

	//node metrics
	summaryNS, ok := pw.SummaryMap[podObject.Namespace]
	if !ok {
		summaryNS = m.NewClusterPodMetrics(podObject.Namespace, m.ALL)
		pw.SummaryMap[podObject.Namespace] = summaryNS
	}

	summary.PodsCount = summary.PodsCount + 1
	summaryNS.PodsCount++
	summaryNode.PodsCount++

	var sb strings.Builder
	for k, v := range p.Labels {
		fmt.Fprintf(&sb, "%s:%s;", k, v)
	}
	podObject.Labels = sb.String()
	sb.Reset()
	for k, v := range p.GetAnnotations() {
		fmt.Fprintf(&sb, "%s:%s;", k, v)
	}
	podObject.Annotations = sb.String()
	podObject.ContainerCount = len(p.Spec.Containers)

	var limitsDefined bool = false
	for _, c := range p.Spec.Containers {
		if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
			podObject.NumPrivileged++
		}

		if c.LivenessProbe == nil {
			podObject.LiveProbes++
		}

		if c.ReadinessProbe == nil {
			podObject.ReadyProbes++
		}

		if c.Resources.Requests != nil {
			cpuReq, ok := c.Resources.Requests.Cpu().AsInt64()
			if ok {
				podObject.CpuRequest = cpuReq
			}

			memReq, ok := c.Resources.Requests.Memory().AsInt64()
			if ok {
				podObject.MemRequest = memReq
			}
			limitsDefined = true
		}

		if c.Resources.Limits != nil {
			cpuLim, ok := c.Resources.Limits.Cpu().AsInt64()
			if ok {
				podObject.CpuLimit = cpuLim
			}

			memLim, ok := c.Resources.Limits.Memory().AsInt64()
			if ok {
				podObject.MemLimit = memLim
			}
			limitsDefined = true
		}

		if c.VolumeMounts != nil {
			sb.Reset()
			for _, vol := range c.VolumeMounts {
				fmt.Fprintf(&sb, "%s;", vol.MountPath)
			}
			podObject.Mounts = sb.String()
		}
	}
	if !limitsDefined {
		podObject.LimitsDefined = limitsDefined
	}

	podObject.InitContainerCount = len(p.Spec.InitContainers)

	if p.Spec.Priority != nil {
		podObject.Priority = *p.Spec.Priority
	}
	podObject.ServiceAccountName = p.Spec.ServiceAccountName
	if p.Spec.TerminationGracePeriodSeconds != nil {
		podObject.TerminationGracePeriodSeconds = *p.Spec.TerminationGracePeriodSeconds
	}

	podObject.RestartPolicy = string(p.Spec.RestartPolicy)

	for i, t := range p.Spec.Tolerations {
		if i == 0 {
			podObject.Tolerations = fmt.Sprintf("%s;", t.String())
		} else {
			podObject.Tolerations = fmt.Sprintf("%s;%s", podObject.Tolerations, t.String())
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

		}

		if p.Spec.Affinity.PodAntiAffinity != nil {

		}
	}

	podObject.HostIP = p.Status.HostIP

	podObject.Phase = string(p.Status.Phase)

	switch podObject.Phase {
	case "Pending":
		summary.PodPending++
		summaryNS.PodPending++
		summaryNode.PodPending++
		break
	case "Failed":
		summary.PodFailed++
		summaryNS.PodFailed++
		summaryNode.PodFailed++
		break
	case "Running":
		summary.PodRunning++
		summaryNS.PodRunning++
		summaryNode.PodRunning++
		break
	}

	podObject.PodIP = p.Status.PodIP
	podObject.Reason = p.Status.Reason

	if podObject.Reason != "" && podObject.Reason == "Evicted" {
		summary.Evictions++
		summaryNS.Evictions++
		summaryNode.Evictions++
	}

	if p.Status.StartTime != nil {
		podObject.StartTime = p.Status.StartTime.Time
	}

	if p.Status.Conditions != nil && len(p.Status.Conditions) > 0 {
		pcond := p.Status.Conditions[len(p.Status.Conditions)-1]
		podObject.ReasonCondition = pcond.Reason
		podObject.StatusCondition = string(pcond.Status)
		podObject.TypeCondition = string(pcond.Type)
		podObject.LastTransitionTimeCondition = pcond.LastTransitionTime.Time
	}

	if p.Status.ContainerStatuses != nil {
		var imBuilder strings.Builder
		var waitBuilder strings.Builder
		var termBuilder strings.Builder
		for _, st := range p.Status.ContainerStatuses {
			imBuilder.WriteString(fmt.Sprintf("%s;", st.Image))

			podObject.PodRestarts += st.RestartCount

			if st.State.Waiting != nil {
				waitBuilder.WriteString(fmt.Sprintf("%s;", st.State.Waiting.Reason))
			}

			if st.State.Terminated != nil {
				termBuilder.WriteString(fmt.Sprintf("%s;", st.State.Terminated.Reason))
				podObject.TerminationTime = st.State.Terminated.FinishedAt.Time
			}

			if st.State.Running != nil {
				podObject.RunningStartTime = st.State.Running.StartedAt.Time
			}
		}
		podObject.Images = imBuilder.String()
		podObject.WaitReasons = waitBuilder.String()
		podObject.TermReasons = termBuilder.String()

	}

	//metrics
	if podMetricsObj != nil {
		usageObj := podMetricsObj.GetPodUsage()

		podObject.CpuUse = usageObj.CPU
		podObject.MemUse = usageObj.Memory

	}

	pw.SummaryMap[m.ALL] = summary
	//	fmt.Printf("Pod Object:\n%s\n", podObject.ToString())
	return podObject
}

//func (pw PodWorker) BuildPodsSnapshot() m.AppDMetricList {
//	fmt.Println("Pods Worker: Started")
//	var metricsMap *map[string]m.UsageStats = pw.GetPodMetrics()

//	api := pw.Client.CoreV1()
//	pods, err := api.Pods("").List(metav1.ListOptions{})
//	if err != nil {
//		fmt.Printf("Issues getting pods %s\n", err)
//		return m.NewAppDMetricList()
//	}
//	objList := m.NewPodObjList()

//	for _, p := range pods.Items {

//	}
//	fmt.Println("Pods Worker: Finished")

//	return pw.builAppDMetricsList()
//}

func (pw PodWorker) GetPodMetrics() *map[string]m.UsageStats {
	metricsDonePods := make(chan *m.PodMetricsObjList)
	go metricsWorkerPods(metricsDonePods, pw.Client)

	metricsDataPods := <-metricsDonePods
	objMap := metricsDataPods.PrintPodList()
	return &objMap
}

func (pw PodWorker) GetPodMetricsSingle(namespace string, podName string) *m.PodMetricsObj {
	metricsDone := make(chan *m.PodMetricsObj)
	go metricsWorkerSingle(metricsDone, pw.Client, namespace, podName)

	metricsData := <-metricsDone
	return metricsData
}

func metricsWorkerPods(finished chan *m.PodMetricsObjList, client *kubernetes.Clientset) {
	fmt.Println("Metrics Worker Pods: Started")
	var path string = "apis/metrics.k8s.io/v1beta1/pods"

	data, err := client.RESTClient().Get().AbsPath(path).DoRaw()
	if err != nil {
		fmt.Printf("Issues when requesting metrics from metrics with path %s from server %s\n", path, err.Error())
	}

	var list m.PodMetricsObjList
	merde := json.Unmarshal(data, &list)
	if merde != nil {
		fmt.Printf("Unmarshal issues. %v\n", merde)
	}

	fmt.Println("Metrics Worker Pods: Finished. Metrics records: %d", len(list.Items))
	fmt.Println(&list)
	finished <- &list
}

func metricsWorkerSingle(finished chan *m.PodMetricsObj, client *kubernetes.Clientset, namespace string, podName string) {
	var path string = ""
	var metricsObj m.PodMetricsObj
	if namespace != "" && podName != "" {
		path = fmt.Sprintf("apis/metrics.k8s.io/v1beta1/namespaces/%s/pods/%s", namespace, podName)

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

func (pw PodWorker) builAppDMetricsList() m.AppDMetricList {
	ml := m.NewAppDMetricList()
	var list []m.AppDMetric
	for _, value := range pw.SummaryMap {
		p := &value
		objMap := structs.Map(p)
		for fieldName, fieldValue := range objMap {
			if fieldName != "Nodename" && fieldName != "Namespace" && fieldName != "Path" && fieldName != "Metadata" {
				appdMetric := m.NewAppDMetric(fieldName, fieldValue.(int64), value.Path)
				list = append(list, appdMetric)
				ml.AddMetrics(appdMetric)
			}
		}
	}
	ml.Items = list
	return ml
}
