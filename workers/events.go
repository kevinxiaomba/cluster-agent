package workers

import (
	"fmt"
	"sync"

	//	"time"

	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	"github.com/sjeltuhin/clusterAgent/watchers"
	//	"github.com/fatih/structs"
	m "github.com/sjeltuhin/clusterAgent/models"
	"k8s.io/client-go/tools/cache"
)

type EventWorker struct {
	informer cache.SharedIndexInformer
	Client   *kubernetes.Clientset
	AppName  string
}

func NewEventWorker(client *kubernetes.Clientset, appName string) EventWorker {
	ew := EventWorker{initInformer(client), client, appName}
	return ew
}

func initInformer(client *kubernetes.Clientset) cache.SharedIndexInformer {
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
		AddFunc: onNewEvent,
	})
	return i
}

func onNewEvent(obj interface{}) {
	eventObj := obj.(*v1.Event)
	fmt.Printf("Received event: %s %s %s\n", eventObj.Namespace, eventObj.Message, eventObj.Reason)
}

func (ew EventWorker) Observe(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	wg.Add(1)
	go ew.informer.Run(stopCh)

	<-stopCh
}

func (ew EventWorker) BuildEventSnapshot() m.AppDMetricList {
	fmt.Println("Event Worker: Started")

	api := ew.Client.CoreV1()
	_, err := api.Events("").List(metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Issues getting events %s\n", err)
		return m.NewAppDMetricList()
	}

	fmt.Println("Events Worker: Finished. Watching...")
	//start watching
	eventWatcher := watchers.NewEventWatcher(ew.Client)
	eventWatcher.Observe()
	fmt.Println("Done Watching...")
	return ew.builAppDMetricsList()
}

func (ew EventWorker) builAppDMetricsList() m.AppDMetricList {
	ml := m.NewAppDMetricList()
	//	for _, value := range ew.SummaryMap {
	//		p := &value
	//		objMap := structs.Map(p)
	//		for fieldName, fieldValue := range objMap {
	//			if fieldName != "NodeName" && fieldName != "Namespace" && fieldName != "Path" && fieldName != "Metadata" {
	//				appdMetric := m.NewAppDMetric(fieldName, fieldValue.(int64), value.Path)
	//				ml.AddMetrics(appdMetric)
	//				fmt.Printf("Adding metric %s", appdMetric.ToString())
	//			}
	//		}
	//	}
	return ml
}
