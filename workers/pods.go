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
	AppSummaryMap  map[string]m.ClusterAppMetrics
	WQ             workqueue.RateLimitingInterface
	AppdController *app.ControllerClient
	K8sConfig      *rest.Config
	PodCache       map[string]m.PodSchema
}

func NewPodWorker(client *kubernetes.Clientset, bag *m.AppDBag, controller *app.ControllerClient, config *rest.Config) PodWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	pw := PodWorker{Client: client, Bag: bag, SummaryMap: make(map[string]m.ClusterPodMetrics), AppSummaryMap: make(map[string]m.ClusterAppMetrics), WQ: queue, AppdController: controller, K8sConfig: config}
	pw.initPodInformer(client)
	pw.PodCache = make(map[string]m.PodSchema)
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

func (pw *PodWorker) getPodKey(obj m.PodSchema) string {
	return fmt.Sprintf("%s_%s", obj.Namespace, obj.Name)
}

func (pw *PodWorker) onNewPod(obj interface{}) {
	podObj := obj.(*v1.Pod)
	initTime := time.Now()
	fmt.Printf("Added Pod: %s %s\n", podObj.Namespace, podObj.Name)
	podRecord, _ := pw.processObject(podObj, nil)
	podRecord.PodInitTime = initTime
	pw.WQ.Add(&podRecord)
	pw.PodCache[pw.getPodKey(podRecord)] = podRecord
	if podObj.Status.Phase == "Running" {
		go pw.instrument(podObj)
	}
}

func (pw *PodWorker) instrument(podObj *v1.Pod) {
	bth := pw.AppdController.StartBT("InstrumentAppD")
	injector := NewAgentInjector(pw.Client, pw.K8sConfig, pw.Bag)
	injector.EnsureInstrumentation(podObj)
	pw.AppdController.StopBT(bth)
}

func (pw *PodWorker) startEventQueueWorker(stopCh <-chan struct{}) {
	//wait.Until(pw.flushQueue, 30*time.Second, stopCh)
	pw.eventQueueTicker(stopCh, time.NewTicker(15*time.Second))
}

func (pw *PodWorker) flushQueue() {
	bth := pw.AppdController.StartBT("FlushPodDataQueue")
	count := pw.WQ.Len()
	fmt.Printf("Flushing the queue of %d records\n", count)
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
			fmt.Println("Queue shut down")
		}
		if count == 0 || len(objList) >= pw.Bag.EventAPILimit {
			fmt.Printf("Sending %d records to AppD events API\n", len(objList))
			pw.postPodRecords(&objList)
			pw.postContainerRecords(containerList)
			pw.AppdController.StopBT(bth)
			return
		}
	}
	pw.AppdController.StopBT(bth)
}

func (pw *PodWorker) postContainerRecords(objList []m.ContainerSchema) {
	count := 0
	var containerList []m.ContainerSchema
	for _, c := range objList {
		containerList = append(containerList, c)
		if count == pw.Bag.EventAPILimit {
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
	logger := log.New(os.Stdout, "[APPD_CLUSTER_MONITOR]", log.Lshortfile)
	rc := app.NewRestClient(pw.Bag, logger)
	data, err := json.Marshal(objList)
	schemaDefObj := m.NewContainerSchemaDefWrapper()
	schemaDef, e := json.Marshal(schemaDefObj)
	//	fmt.Printf("Schema def: %s\n", string(schemaDef))
	if err == nil && e == nil {
		if rc.SchemaExists(pw.Bag.ContainerSchemaName) == false {
			fmt.Printf("Creating Container schema. %s\n", pw.Bag.ContainerSchemaName)
			if rc.CreateSchema(pw.Bag.ContainerSchemaName, schemaDef) != nil {
				fmt.Printf("Schema %s created\n", pw.Bag.ContainerSchemaName)
			}
		} else {
			fmt.Printf("Schema %s exists\n", pw.Bag.ContainerSchemaName)
		}
		fmt.Println("About to post records")
		rc.PostAppDEvents(pw.Bag.ContainerSchemaName, data)
	} else {
		fmt.Printf("Problems when serializing array of pod schemas. %v", err)
	}
}

func (pw *PodWorker) postPodRecords(objList *[]m.PodSchema) {
	logger := log.New(os.Stdout, "[APPD_CLUSTER_MONITOR]", log.Lshortfile)
	rc := app.NewRestClient(pw.Bag, logger)
	data, err := json.Marshal(objList)
	schemaDefObj := m.NewPodSchemaDefWrapper()
	schemaDef, e := json.Marshal(schemaDefObj)
	//	fmt.Printf("Schema def: %s\n", string(schemaDef))
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
	podRecord, _ := pw.processObject(podObj, nil)
	pw.WQ.Add(&podRecord)
	delete(pw.PodCache, pw.getPodKey(podRecord))
}

func (pw *PodWorker) onUpdatePod(objOld interface{}, objNew interface{}) {
	podObj := objNew.(*v1.Pod)
	podOldObj := objOld.(*v1.Pod)

	podRecord, changed := pw.processObject(podObj, podOldObj)

	if changed {
		fmt.Printf("Pod changed: %s %s\n", podObj.Namespace, podObj.Name)
		pw.WQ.Add(&podRecord)
	}

	pw.PodCache[pw.getPodKey(podRecord)] = podRecord

	if podObj.Status.Phase == "Running" {
		go pw.instrument(podObj)
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
	pw.appMetricTicker(stopCh, time.NewTicker(60*time.Second))

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
	bth := pw.AppdController.StartBT("PostPodMetrics")
	pw.SummaryMap = make(map[string]m.ClusterPodMetrics)
	pw.AppSummaryMap = make(map[string]m.ClusterAppMetrics)
	var count int = 0
	//	for _, obj := range pw.informer.GetStore().List() {
	//		podObject := obj.(*v1.Pod)
	//		podSchema := pw.processObject(podObject)
	//		pw.summarize(&podSchema)
	//		count++
	//	}

	for _, podSchema := range pw.PodCache {
		pw.summarize(&podSchema)
		count++
	}
	fmt.Printf("Total metrics: %d\n", count)

	ml := pw.builAppDMetricsList()

	fmt.Printf("Ready to push %d metrics\n", len(ml.Items))

	pw.AppdController.PostMetrics(ml)
	pw.AppdController.StopBT(bth)
}

func (pw *PodWorker) summarize(podObject *m.PodSchema) {
	//global metrics
	summary, okSum := pw.SummaryMap[m.ALL]
	if !okSum {
		summary = m.NewClusterPodMetrics(pw.Bag, m.ALL, m.ALL)
		pw.SummaryMap[m.ALL] = summary
	}

	//namespace metrics
	summaryNode, okNode := pw.SummaryMap[podObject.NodeName]
	if !okNode {
		summaryNode = m.NewClusterPodMetrics(pw.Bag, m.ALL, podObject.NodeName)
		pw.SummaryMap[podObject.NodeName] = summaryNode
	}

	//node metrics
	summaryNS, okNS := pw.SummaryMap[podObject.Namespace]
	if !okNS {
		summaryNS = m.NewClusterPodMetrics(pw.Bag, podObject.Namespace, m.ALL)
		pw.SummaryMap[podObject.Namespace] = summaryNS
	}

	//app/tier metrics
	summaryApp, okApp := pw.AppSummaryMap[podObject.Owner]
	if !okApp {
		summaryApp = m.NewClusterAppMetrics(pw.Bag, podObject.Namespace, podObject.Owner, "")
		pw.AppSummaryMap[podObject.Owner] = summaryApp
		summaryApp.ContainerCount = int64(podObject.ContainerCount)
		summaryApp.InitContainerCount = int64(podObject.InitContainerCount)
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
	summary.PendingTime += podObject.PendingTime
	summaryNS.PendingTime += podObject.PendingTime
	summaryNode.PendingTime += podObject.PendingTime

	summary.PodRestarts += int64(podObject.PodRestarts)
	summaryNS.PodRestarts += int64(podObject.PodRestarts)
	summaryNode.PodRestarts += int64(podObject.PodRestarts)
	summaryApp.PodRestarts += int64(podObject.PodRestarts)

	summary.UseCpu += podObject.CpuUse
	summary.UseMemory += podObject.MemUse

	summaryNS.UseCpu += podObject.CpuUse
	summaryNS.UseMemory += podObject.MemUse

	summaryNode.UseCpu += podObject.CpuUse
	summaryNode.UseMemory += podObject.MemUse

	pw.SummaryMap[m.ALL] = summary
	pw.SummaryMap[podObject.Namespace] = summaryNS
	pw.SummaryMap[podObject.NodeName] = summaryNode

	//app summary for containers
	div := podObject.ContainerCount
	for _, c := range podObject.Containers {
		key := fmt.Sprintf("%s_%s_%s", podObject.Namespace, podObject.Owner, c.Name)
		summaryContainer, okCont := pw.AppSummaryMap[key]
		if !okCont {
			summaryContainer = m.NewClusterAppMetrics(pw.Bag, podObject.Namespace, podObject.Owner, c.Name)
			pw.AppSummaryMap[key] = summaryContainer
		}
		summaryContainer.LimitCpu += c.CpuLimit / int64(div)
		summaryContainer.LimitMemory += c.MemLimit / int64(div)
		summaryContainer.RequestCpu += c.CpuRequest / int64(div)
		summaryContainer.RequestMemory += c.MemRequest / int64(div)
		summaryContainer.UseCpu += c.CpuUse / int64(div)
		summaryContainer.UseMemory += c.MemUse / int64(div)
		pw.AppSummaryMap[key] = summaryContainer
	}
	pw.AppSummaryMap[podObject.Owner] = summaryApp
}

func (pw *PodWorker) processObject(p *v1.Pod, old *v1.Pod) (m.PodSchema, bool) {
	changed := true
	fmt.Printf("Pod: %s\n", p.Name)

	podObject := m.NewPodObj()
	var sb strings.Builder
	for k, v := range p.Labels {
		fmt.Fprintf(&sb, "%s:%s;", k, v)
		if k == "name" {
			podObject.Owner = v
		}
	}
	if podObject.Owner == "" && len(p.OwnerReferences) > 0 {
		podObject.Owner = p.OwnerReferences[0].Name
	}
	podObject.Labels = sb.String()
	sb.Reset()

	podObject.Name = p.Name
	podObject.Namespace = p.Namespace
	podObject.NodeName = p.Spec.NodeName

	var oldObject *m.PodSchema = nil
	if old != nil {
		cached, exists := pw.PodCache[pw.getPodKey(podObject)]
		if exists {
			oldObject = &cached
		}
	}

	if p.ClusterName != "" {
		podObject.ClusterName = p.ClusterName
	} else {
		podObject.ClusterName = pw.Bag.AppName
	}

	if p.Status.StartTime != nil {
		podObject.StartTime = p.Status.StartTime.Time
	}

	for k, v := range p.GetAnnotations() {
		fmt.Fprintf(&sb, "%s:%s;", k, v)
	}
	podObject.Annotations = sb.String()

	podObject.InitContainerCount = len(p.Spec.InitContainers)
	podObject.ContainerCount = len(p.Spec.Containers)

	if podObject.ContainerCount+podObject.InitContainerCount > 0 {
		podObject.Containers = make(map[string]m.ContainerSchema, podObject.ContainerCount+podObject.InitContainerCount)
	}

	var limitsDefined bool = false
	for _, c := range p.Spec.Containers {
		containerObj := m.NewContainerObj()
		containerObj.Name = c.Name
		containerObj.Namespace = podObject.Namespace
		containerObj.NodeName = podObject.NodeName
		containerObj.PodName = podObject.Name
		containerObj.Init = false
		containerObj.PodInitTime = podObject.PodInitTime

		if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
			podObject.NumPrivileged++
			containerObj.Privileged = 1
		}

		if c.LivenessProbe == nil {
			podObject.LiveProbes++
			containerObj.LiveProbes = 1
		}

		if c.ReadinessProbe == nil {
			podObject.ReadyProbes++
			containerObj.ReadyProbes = 1
		}

		if c.Resources.Requests != nil {
			cpuReq, ok := c.Resources.Requests.Cpu().AsInt64()
			if ok {
				podObject.CpuRequest += cpuReq
				containerObj.CpuRequest = cpuReq
			}

			memReq, ok := c.Resources.Requests.Memory().AsInt64()
			if ok {
				podObject.MemRequest += memReq
				containerObj.MemRequest = memReq
			}
			limitsDefined = true
		}

		if c.Resources.Limits != nil {
			cpuLim, ok := c.Resources.Limits.Cpu().AsInt64()
			if ok {
				podObject.CpuLimit += cpuLim
				containerObj.CpuLimit = cpuLim
			}

			memLim, ok := c.Resources.Limits.Memory().AsInt64()
			if ok {
				podObject.MemLimit += memLim
				containerObj.MemLimit = memLim
			}
			limitsDefined = true
		}

		if c.VolumeMounts != nil {
			sb.Reset()
			for _, vol := range c.VolumeMounts {
				fmt.Fprintf(&sb, "%s;", vol.MountPath)
			}
			containerObj.Mounts = sb.String()
		}

		podObject.Containers[c.Name] = containerObj
	}
	podObject.LimitsDefined = limitsDefined

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

	//check PendingTime
	if old == nil || (old.Status.Phase == "Pending" && (podObject.Phase == "Running" || podObject.Phase == "Failed")) {
		podObject.PendingTime = int64(time.Now().Sub(podObject.PodInitTime).Seconds() * 1000)
	}

	podObject.PodIP = p.Status.PodIP
	podObject.Reason = p.Status.Reason

	if p.Status.Conditions != nil && len(p.Status.Conditions) > 0 {
		var latestCondition *v1.PodCondition = nil
		timePoint := podObject.PodInitTime
		for _, cn := range p.Status.Conditions {
			if cn.LastTransitionTime.Time.After(timePoint) {
				timePoint = cn.LastTransitionTime.Time
				latestCondition = &cn
			}
		}
		pcond := latestCondition //[len(p.Status.Conditions)-1]
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
			containerObj := findContainer(&podObject, st.Name)
			if containerObj != nil {
				containerObj.Restarts = st.RestartCount
				containerObj.Image = st.Image
			}

			imBuilder.WriteString(fmt.Sprintf("%s;", st.Image))

			podObject.PodRestarts += st.RestartCount

			if st.State.Waiting != nil {
				waitBuilder.WriteString(fmt.Sprintf("%s;", st.State.Waiting.Reason))
				if containerObj != nil {
					containerObj.WaitReason = st.State.Waiting.Reason
				}
			}

			if st.State.Terminated != nil {
				termBuilder.WriteString(fmt.Sprintf("%s;", st.State.Terminated.Reason))
				podObject.TerminationTime = st.State.Terminated.FinishedAt.Time
				if containerObj != nil {
					containerObj.TermReason = st.State.Terminated.Reason
					containerObj.TerminationTime = st.State.Terminated.FinishedAt.Time
				}
			}

			if st.State.Running != nil {
				podObject.RunningStartTime = st.State.Running.StartedAt.Time
				if containerObj != nil {
					containerObj.StartTime = st.State.Running.StartedAt.Time
				}
			}
		}

		podObject.Images = imBuilder.String()
		podObject.WaitReasons = waitBuilder.String()
		podObject.TermReasons = termBuilder.String()

	}

	//metrics
	podMetricsObj := pw.GetPodMetricsSingle(p, p.Namespace, p.Name)
	if podMetricsObj != nil {
		usageObj := podMetricsObj.GetPodUsage()

		podObject.CpuUse = usageObj.CPU
		podObject.MemUse = usageObj.Memory

		for cname, c := range podObject.Containers {
			contUsageObj := podMetricsObj.GetContainerUsage(cname)
			c.CpuUse = contUsageObj.CPU
			c.MemUse = contUsageObj.Memory
			podObject.Containers[cname] = c
		}

	}

	//check for changes
	if oldObject != nil {
		changed = compareString(podObject.Phase, oldObject.Phase, changed)
		changed = compareInt64(podObject.CpuUse, oldObject.CpuUse, changed)
		changed = compareInt64(podObject.MemUse, oldObject.MemUse, changed)
	}

	return podObject, changed
}

func findContainer(podObject *m.PodSchema, containerName string) *m.ContainerSchema {
	if c, ok := podObject.Containers[containerName]; ok {
		return &c
	}
	return nil
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
	for _, metricPod := range pw.SummaryMap {
		objMap := structs.Map(metricPod)
		pw.addMetricToList(objMap, metricPod, &list)
	}
	for _, metricApp := range pw.AppSummaryMap {
		objMap := structs.Map(metricApp)
		pw.addMetricToList(objMap, metricApp, &list)
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
