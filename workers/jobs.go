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

	"github.com/sjeltuhin/clusterAgent/config"
	m "github.com/sjeltuhin/clusterAgent/models"
	"github.com/sjeltuhin/clusterAgent/utils"

	app "github.com/sjeltuhin/clusterAgent/appd"
	batchTypes "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	batch "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type JobsWorker struct {
	informer       cache.SharedIndexInformer
	Client         *kubernetes.Clientset
	ConfigManager  *config.MutexConfigManager
	SummaryMap     map[string]m.ClusterJobMetrics
	WQ             workqueue.RateLimitingInterface
	AppdController *app.ControllerClient
	K8sConfig      *rest.Config
}

func NewJobsWorker(client *kubernetes.Clientset, cm *config.MutexConfigManager, controller *app.ControllerClient, config *rest.Config) JobsWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	pw := JobsWorker{Client: client, ConfigManager: cm, SummaryMap: make(map[string]m.ClusterJobMetrics), WQ: queue, AppdController: controller, K8sConfig: config}
	pw.initJobInformer(client)
	return pw
}

func (nw *JobsWorker) initJobInformer(client *kubernetes.Clientset) cache.SharedIndexInformer {
	batchClient, err := batch.NewForConfig(nw.K8sConfig)
	if err != nil {
		fmt.Printf("Issues when initializing Batch API client/ %v", err)
		return nil
	}

	i := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return batchClient.Jobs(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return batchClient.Jobs(metav1.NamespaceAll).Watch(options)
			},
		},
		&v1.Node{},
		0,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	i.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    nw.onNewJob,
		DeleteFunc: nw.onDeleteJob,
		UpdateFunc: nw.onUpdateJob,
	})
	nw.informer = i

	return i
}

func (pw *JobsWorker) qualifies(p *batchTypes.Job) bool {
	bag := (*pw.ConfigManager).Get()
	return (len(bag.NsToMonitor) == 0 ||
		utils.StringInSlice(p.Namespace, bag.NsToMonitor)) &&
		!utils.StringInSlice(p.Namespace, bag.NsToMonitorExclude)
}

func (nw *JobsWorker) onNewJob(obj interface{}) {
	jobObj := obj.(*batchTypes.Job)
	if !nw.qualifies(jobObj) {
		return
	}
	fmt.Printf("Added Job: %s\n", jobObj.Name)

}

func (nw *JobsWorker) onDeleteJob(obj interface{}) {
	jobObj := obj.(*batchTypes.Job)
	if !nw.qualifies(jobObj) {
		return
	}
	fmt.Printf("Deleted Job: %s\n", jobObj.Name)
}

func (nw *JobsWorker) onUpdateJob(objOld interface{}, objNew interface{}) {
	jobObj := objOld.(*batchTypes.Job)
	if !nw.qualifies(jobObj) {
		return
	}
	fmt.Printf("Updated Job: %s\n", jobObj.Name)
}

func (pw JobsWorker) Observe(stopCh <-chan struct{}, wg *sync.WaitGroup) {
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

func (pw *JobsWorker) HasSynced() bool {
	return pw.informer.HasSynced()
}

func (pw *JobsWorker) startMetricsWorker(stopCh <-chan struct{}) {
	pw.appMetricTicker(stopCh, time.NewTicker(45*time.Second))

}

func (pw *JobsWorker) appMetricTicker(stop <-chan struct{}, ticker *time.Ticker) {
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

func (pw *JobsWorker) buildAppDMetrics() {
	bth := pw.AppdController.StartBT("SendJobMetrics")
	pw.SummaryMap = make(map[string]m.ClusterJobMetrics)
	fmt.Println("Time to send job metrics. Current cache:")
	var count int = 0
	for _, obj := range pw.informer.GetStore().List() {
		jobObject := obj.(*batchTypes.Job)
		jobSchema := pw.processObject(jobObject)
		pw.summarize(&jobSchema)
		count++
	}
	fmt.Printf("Total: %d\n", count)

	ml := pw.builAppDMetricsList()

	fmt.Printf("Ready to push %d metrics\n", len(ml.Items))

	pw.AppdController.PostMetrics(ml)
	pw.AppdController.StopBT(bth)
}

func (pw *JobsWorker) processObject(j *batchTypes.Job) m.JobSchema {
	bag := (*pw.ConfigManager).Get()
	jobObject := m.NewJobObj()

	if j.ClusterName != "" {
		jobObject.ClusterName = j.ClusterName
	} else {
		jobObject.ClusterName = bag.AppName
	}
	jobObject.Name = j.Name
	jobObject.Namespace = j.Namespace

	var sb strings.Builder
	for k, v := range j.GetLabels() {
		fmt.Fprintf(&sb, "%s:%s;", k, v)
	}
	jobObject.Labels = sb.String()

	sb.Reset()
	for k, v := range j.GetAnnotations() {
		fmt.Fprintf(&sb, "%s:%s;", k, v)
	}
	jobObject.Annotations = sb.String()

	jobObject.Active = j.Status.Active

	jobObject.Success = j.Status.Succeeded

	jobObject.Failed = j.Status.Failed

	jobObject.StartTime = j.Status.StartTime.Time

	if j.Status.CompletionTime != nil {
		jobObject.EndTime = j.Status.CompletionTime.Time
		jobObject.Duration = jobObject.EndTime.Sub(jobObject.StartTime).Seconds()
	} else {
		jobObject.Duration = time.Since(jobObject.StartTime).Seconds()
	}

	if j.Spec.ActiveDeadlineSeconds != nil {
		jobObject.ActiveDeadlineSeconds = *j.Spec.ActiveDeadlineSeconds
	}

	if j.Spec.Completions != nil {
		jobObject.Completions = *j.Spec.Completions
	}

	if j.Spec.BackoffLimit != nil {
		jobObject.BackoffLimit = *j.Spec.BackoffLimit
	}

	if j.Spec.BackoffLimit != nil {
		jobObject.Parallelism = *j.Spec.Parallelism
	}

	return jobObject
}

func (pw *JobsWorker) summarize(jobObject *m.JobSchema) {
	bag := (*pw.ConfigManager).Get()
	//global metrics
	summary, ok := pw.SummaryMap[m.ALL]
	if !ok {
		summary = m.NewClusterJobMetrics(bag, m.ALL, m.ALL)
		pw.SummaryMap[m.ALL] = summary
	}

	//namespace metrics
	summaryNS, ok := pw.SummaryMap[jobObject.Namespace]
	if !ok {
		summaryNS = m.NewClusterJobMetrics(bag, m.ALL, jobObject.Namespace)
		pw.SummaryMap[jobObject.Namespace] = summaryNS
	}

	summary.JobCount++
	summaryNS.JobCount++

	summary.JobActiveCount += int64(jobObject.Active)
	summary.JobFailedCount += int64(jobObject.Failed)
	summary.JobSuccessCount += int64(jobObject.Success)
	summary.JobDuration += int64(jobObject.Duration)

	summaryNS.JobActiveCount += int64(jobObject.Active)
	summaryNS.JobFailedCount += int64(jobObject.Failed)
	summaryNS.JobSuccessCount += int64(jobObject.Success)
	summaryNS.JobDuration += int64(jobObject.Duration)

	pw.SummaryMap[m.ALL] = summary
	pw.SummaryMap[jobObject.Namespace] = summaryNS
}

func (pw JobsWorker) builAppDMetricsList() m.AppDMetricList {
	ml := m.NewAppDMetricList()
	var list []m.AppDMetric
	for _, value := range pw.SummaryMap {
		pw.addMetricToList(&value, &list)
	}
	ml.Items = list
	return ml
}

func (pw JobsWorker) addMetricToList(metric *m.ClusterJobMetrics, list *[]m.AppDMetric) {
	objMap := structs.Map(metric)
	for fieldName, fieldValue := range objMap {
		if fieldName != "Namespace" && fieldName != "Path" && fieldName != "Metadata" {
			appdMetric := m.NewAppDMetric(fieldName, fieldValue.(int64), metric.Path)
			*list = append(*list, appdMetric)
		}
	}
}

//queue

func (pw *JobsWorker) startEventQueueWorker(stopCh <-chan struct{}) {
	pw.eventQueueTicker(stopCh, time.NewTicker(15*time.Second))
}

func (pw *JobsWorker) eventQueueTicker(stop <-chan struct{}, ticker *time.Ticker) {
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

func (pw *JobsWorker) flushQueue() {
	bag := (*pw.ConfigManager).Get()
	bth := pw.AppdController.StartBT("FlushJobEventsQueue")
	count := pw.WQ.Len()
	fmt.Printf("Flushing the queue of %d records\n", count)
	if count == 0 {
		return
	}

	var objList []m.JobSchema
	var jobRecord *m.JobSchema
	var ok bool = true

	for count >= 0 {

		jobRecord, ok = pw.getNextQueueItem()
		count = count - 1
		if ok {
			objList = append(objList, *jobRecord)
		} else {
			fmt.Println("Queue shut down")
		}
		if count == 0 || len(objList) >= bag.EventAPILimit {
			fmt.Printf("Sending %d records to AppD events API\n", len(objList))
			pw.postJobRecords(&objList)
			return
		}
	}
	pw.AppdController.StopBT(bth)
}

func (pw *JobsWorker) postJobRecords(objList *[]m.JobSchema) {
	bag := (*pw.ConfigManager).Get()
	logger := log.New(os.Stdout, "[APPD_CLUSTER_MONITOR]", log.Lshortfile)
	rc := app.NewRestClient(bag, logger)

	schemaDefObj := m.NewJobSchemaDefWrapper()

	err := rc.EnsureSchema(bag.JobSchemaName, &schemaDefObj)
	if err != nil {
		fmt.Printf("Issues when ensuring %s schema. %v\n", bag.JobSchemaName, err)
	} else {
		data, err := json.Marshal(objList)
		if err != nil {
			fmt.Printf("Problems when serializing array of job schemas. %v", err)
		}
		rc.PostAppDEvents(bag.JobSchemaName, data)
	}

}

func (pw *JobsWorker) getNextQueueItem() (*m.JobSchema, bool) {
	podRecord, quit := pw.WQ.Get()

	if quit {
		return podRecord.(*m.JobSchema), false
	}
	defer pw.WQ.Done(podRecord)
	pw.WQ.Forget(podRecord)

	return podRecord.(*m.JobSchema), true
}
