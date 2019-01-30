package workers

import (
	"fmt"
	"sync"

	"time"

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
	Bag      *m.AppDBag
}

func NewEventWorker(client *kubernetes.Clientset, bag *m.AppDBag) EventWorker {
	ew := EventWorker{initInformer(client), client, bag}
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

func EmitInstrumentationEvent(pod *v1.Pod, client *kubernetes.Clientset, reason, message, eventType string) error {
	fmt.Printf("About to emit event: %s %s %s for pod %s-%s\n", reason, message, eventType, pod.Namespace, pod.Name)
	event := eventFromPod(pod, reason, message, eventType)
	_, err := client.Core().Events(pod.Namespace).Create(event)
	if err != nil {
		fmt.Printf("Issues when emitting instrumentation event %v\n", err)
	}
	return err
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

func (ew EventWorker) processObject(e *v1.Event) m.EventSchema {
	eventObject := m.NewEventObj()

	if e.ClusterName != "" {
		eventObject.ClusterName = e.ClusterName
	} else {
		eventObject.ClusterName = ew.Bag.AppName
	}

	eventObject.Count = e.Count
	eventObject.CreationTimestamp = e.CreationTimestamp.Time
	eventObject.DeletionTimestamp = e.DeletionTimestamp.Time

	eventObject.GenerateName = e.GenerateName
	eventObject.Generation = e.Generation

	eventObject.LastTimestamp = e.LastTimestamp.Time
	eventObject.Message = e.Message
	eventObject.Name = e.Name
	eventObject.Namespace = e.Namespace
	eventObject.ObjectKind = e.GetObjectKind().GroupVersionKind().Kind
	eventObject.ObjectName = e.GetObjectMeta().GetName()
	eventObject.ObjectNamespace = e.ObjectMeta.GetNamespace()
	eventObject.ObjectResourceVersion = e.ObjectMeta.GetResourceVersion()
	eventObject.ObjectUid = string(e.GetObjectMeta().GetUID())
	for _, r := range e.GetOwnerReferences() {
		eventObject.OwnerReferences += r.Name + ";"
	}

	eventObject.Reason = e.Reason
	eventObject.ResourceVersion = e.GetResourceVersion()
	eventObject.SelfLink = e.GetSelfLink()
	eventObject.SourceComponent = e.Source.Component
	eventObject.SourceHost = e.Source.Host

	return eventObject
}

func (ew EventWorker) builAppDMetricsList() m.AppDMetricList {
	ml := m.NewAppDMetricList()

	return ml
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
