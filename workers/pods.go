package workers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"time"

	"github.com/fatih/structs"
	m "github.com/sjeltuhin/clusterAgent/models"
	"k8s.io/client-go/rest"

	//	"github.com/sjeltuhin/clusterAgent/watchers"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	app "github.com/sjeltuhin/clusterAgent/appd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	//	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"
)

type PodWorker struct {
	informer       cache.SharedIndexInformer
	Client         *kubernetes.Clientset
	Bag            *m.AppDBag
	SummaryMap     map[string]m.ClusterPodMetrics
	WQ             workqueue.RateLimitingInterface
	AppdController *app.ControllerClient
	K8sConfig      *rest.Config
}

func NewPodWorker(client *kubernetes.Clientset, bag *m.AppDBag, controller *app.ControllerClient, config *rest.Config) PodWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	pw := PodWorker{Client: client, Bag: bag, SummaryMap: make(map[string]m.ClusterPodMetrics), WQ: queue, AppdController: controller, K8sConfig: config}
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

	if podObj.Namespace == "dev" && podObj.Status.Phase == "Running" {
		go pw.instrument(podObj)
	}
}

func (pw *PodWorker) instrument(podObj *v1.Pod) {
	injector := NewAgentInjector(pw.Client, pw.K8sConfig, pw.Bag)
	injector.EnsureInstrumentation(podObj)
}

func (pw *PodWorker) startEventQueueWorker(stopCh <-chan struct{}) {
	//wait.Until(pw.flushQueue, 30*time.Second, stopCh)
	pw.eventQueueTicker(stopCh, time.NewTicker(15*time.Second))
}

func (pw *PodWorker) flushQueue() {
	count := pw.WQ.Len()
	fmt.Printf("Flushing the queue of %d records", count)
	if count == 0 {
		return
	}

	var objList []m.PodSchema
	var podRecord *m.PodSchema
	var ok bool = true

	for count >= 0 {

		podRecord, ok = pw.getNextQueueItem()
		count = count - 1
		if ok {
			objList = append(objList, *podRecord)
		} else {
			fmt.Println("Queue shut down")
		}
		if count == 0 || len(objList) >= pw.Bag.EventAPILimit {
			fmt.Printf("Sending %d records to AppD events API\n", len(objList))
			pw.postPodRecords(&objList)
			return
		}
	}

}

func (pw *PodWorker) postPodRecords(objList *[]m.PodSchema) {
	logger := log.New(os.Stdout, "[APPD_CLUSTER_MONITOR]", log.Lshortfile)
	rc := app.NewRestClient(pw.Bag, logger)
	data, err := json.Marshal(objList)
	schemaDefObj := m.NewPodSchemaDefWrapper()
	schemaDef, e := json.Marshal(schemaDefObj)
	fmt.Printf("Schema def: %s\n", string(schemaDef))
	if err == nil && e == nil {
		if rc.SchemaExists(pw.Bag.PodSchemaName) == false {
			fmt.Printf("Creating schema. %s\n", pw.Bag.PodSchemaName)
			if rc.CreateSchema(pw.Bag.PodSchemaName, schemaDef) != nil {
				fmt.Printf("Schema %s created\n", pw.Bag.PodSchemaName)
			}
		} else {
			fmt.Printf("Schema %s exists\n", pw.Bag.PodSchemaName)
		}
		fmt.Println("About to post records")
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
	podRecord := pw.processObject(podObj)
	pw.WQ.Add(&podRecord)
}

func (pw *PodWorker) onUpdatePod(objOld interface{}, objNew interface{}) {
	podObj := objNew.(*v1.Pod)
	podOldObj := objOld.(*v1.Pod)

	podRecord := pw.processObject(podObj)
	podOldRecord := pw.processObject(podOldObj)

	if !podRecord.Equals(&podOldRecord) {
		fmt.Printf("Pod changed: %s %s\n", podObj.Namespace, podObj.Name)
		pw.WQ.Add(&podRecord)
	}
}

func (pw PodWorker) Observe(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	defer pw.WQ.ShutDown()
	wg.Add(1)
	go pw.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, pw.HasSynced) {
		fmt.Errorf("Timed out waiting for caches to sync")
	}
	fmt.Println("Cache syncronized. Starting the processing...")

	wg.Add(1)
	go pw.startMetricsWorker(stopCh)

	wg.Add(1)
	go pw.startEventQueueWorker(stopCh)

	<-stopCh
}

func (pw *PodWorker) HasSynced() bool {
	return pw.informer.HasSynced()
}

func (pw *PodWorker) startMetricsWorker(stopCh <-chan struct{}) {
	//	wait.Until(pw.buildAppDMetrics, 45*time.Second, stopCh)
	pw.appMetricTicker(stopCh, time.NewTicker(45*time.Second))

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
		podSchema := pw.processObject(podObject)
		pw.summarize(&podSchema)
		count++
	}
	fmt.Printf("Total: %d\n", count)

	ml := pw.builAppDMetricsList()

	fmt.Printf("Ready to push %d metrics\n", len(ml.Items))

	pw.AppdController.PostMetrics(ml)
}

func (pw *PodWorker) summarize(podObject *m.PodSchema) {
	//global metrics
	summary, ok := pw.SummaryMap[m.ALL]
	if !ok {
		summary = m.NewClusterPodMetrics(pw.Bag, m.ALL, m.ALL)
		pw.SummaryMap[m.ALL] = summary
	}

	//namespace metrics
	summaryNode, ok := pw.SummaryMap[podObject.NodeName]
	if !ok {
		summaryNode = m.NewClusterPodMetrics(pw.Bag, m.ALL, podObject.NodeName)
		pw.SummaryMap[podObject.NodeName] = summaryNode
	}

	//node metrics
	summaryNS, ok := pw.SummaryMap[podObject.Namespace]
	if !ok {
		summaryNS = m.NewClusterPodMetrics(pw.Bag, podObject.Namespace, m.ALL)
		pw.SummaryMap[podObject.Namespace] = summaryNS
	}

	summary.PodsCount++
	summaryNS.PodsCount++
	summaryNode.PodsCount++

	summary.ContainerCount += int64(podObject.ContainerCount)
	summaryNS.ContainerCount += int64(podObject.ContainerCount)
	summaryNode.ContainerCount += int64(podObject.ContainerCount)

	if !podObject.LimitsDefined {
		summary.NoLimits++
		summaryNS.NoLimits++
		summaryNode.NoLimits++
	}

	summary.Privileged += int64(podObject.NumPrivileged)
	summaryNS.Privileged += int64(podObject.NumPrivileged)
	summaryNode.Privileged += int64(podObject.NumPrivileged)

	summary.NoLivenessProbe += int64(podObject.LiveProbes)
	summaryNS.NoLivenessProbe += int64(podObject.LiveProbes)
	summaryNode.NoLivenessProbe += int64(podObject.LiveProbes)

	summary.NoReadinessProbe += int64(podObject.ReadyProbes)
	summaryNS.NoReadinessProbe += int64(podObject.ReadyProbes)
	summaryNode.NoReadinessProbe += int64(podObject.ReadyProbes)

	summary.LimitCpu += int64(podObject.CpuLimit)
	summaryNS.LimitCpu += int64(podObject.CpuLimit)
	summaryNode.LimitCpu += int64(podObject.CpuLimit)

	summary.LimitMemory += int64(podObject.MemLimit)
	summaryNS.LimitMemory += int64(podObject.MemLimit)
	summaryNode.LimitMemory += int64(podObject.MemLimit)

	summary.RequestCpu += int64(podObject.CpuRequest)
	summaryNS.RequestCpu += int64(podObject.CpuRequest)
	summaryNode.RequestCpu += int64(podObject.CpuRequest)

	summary.RequestMemory += int64(podObject.MemRequest)
	summaryNS.RequestMemory += int64(podObject.MemRequest)
	summaryNode.RequestMemory += int64(podObject.MemRequest)

	summary.InitContainerCount += int64(podObject.InitContainerCount)
	summaryNS.InitContainerCount += int64(podObject.InitContainerCount)
	summaryNode.InitContainerCount += int64(podObject.InitContainerCount)

	if podObject.Tolerations != "" {
		summary.HasTolerations++
		summaryNS.HasTolerations++
		summaryNode.HasTolerations++
	}

	if podObject.NodeAffinityRequired != "" || podObject.NodeAffinityPreferred != "" {
		summary.HasNodeAffinity++
		summaryNS.HasNodeAffinity++
		summaryNode.HasNodeAffinity++
	}

	if podObject.PodAffinityPreferred != "" || podObject.PodAffinityRequired != "" {
		summary.HasPodAffinity++
		summaryNS.HasPodAffinity++
		summaryNode.HasPodAffinity++
	}

	if podObject.PodAntiAffinityPreferred != "" || podObject.PodAntiAffinityRequired != "" {
		summary.HasPodAntiAffinity++
		summaryNS.HasPodAntiAffinity++
		summaryNode.HasPodAntiAffinity++
	}

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

	if podObject.Reason != "" && podObject.Reason == "Evicted" {
		summary.Evictions++
		summaryNS.Evictions++
		summaryNode.Evictions++
	}

	summary.PodRestarts += int64(podObject.PodRestarts)
	summaryNS.PodRestarts += int64(podObject.PodRestarts)
	summaryNode.PodRestarts += int64(podObject.PodRestarts)

	summary.UseCpu += podObject.CpuUse
	summary.UseMemory += podObject.MemUse

	summaryNS.UseCpu += podObject.CpuUse
	summaryNS.UseMemory += podObject.MemUse

	summaryNode.UseCpu += podObject.CpuUse
	summaryNode.UseMemory += podObject.MemUse

	pw.SummaryMap[m.ALL] = summary
	pw.SummaryMap[podObject.Namespace] = summaryNS
	pw.SummaryMap[podObject.NodeName] = summaryNode
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

	podMetricsObj := pw.GetPodMetricsSingle(p, p.Namespace, p.Name)

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
	podObject.LimitsDefined = limitsDefined

	podObject.InitContainerCount = len(p.Spec.InitContainers)

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
				for _, term := range p.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
					sb.WriteString(fmt.Sprintf("%s %s %s;", term.TopologyKey, term.LabelSelector, term.Namespaces))
				}
				podObject.PodAntiAffinityRequired = sb.String()
			}
		}
	}

	podObject.HostIP = p.Status.HostIP

	podObject.Phase = string(p.Status.Phase)

	podObject.PodIP = p.Status.PodIP
	podObject.Reason = p.Status.Reason

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

	return podObject
}

func (pw PodWorker) getLogs(namespace, podName string, logOptions *v1.PodLogOptions, out io.Writer) error {
	req := pw.Client.RESTClient().Get().
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
		return err
	}

	defer readCloser.Close()
	_, err = io.Copy(out, readCloser)
	return err

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

func (pw PodWorker) GetPodMetricsSingle(p *v1.Pod, namespace string, podName string) *m.PodMetricsObj {
	if p.Status.Phase != "Running" {
		return nil
	}
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
		pw.addMetricToList(&value, &list)
	}
	ml.Items = list
	return ml
}

func (pw PodWorker) addMetricToList(metric *m.ClusterPodMetrics, list *[]m.AppDMetric) {
	objMap := structs.Map(metric)
	for fieldName, fieldValue := range objMap {
		if fieldName != "Nodename" && fieldName != "Namespace" && fieldName != "Path" && fieldName != "Metadata" {
			appdMetric := m.NewAppDMetric(fieldName, fieldValue.(int64), metric.Path)
			*list = append(*list, appdMetric)
		}
	}
}
