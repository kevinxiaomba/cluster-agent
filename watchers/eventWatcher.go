package watchers

import (
	"fmt"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

type EventWatcher struct {
	Client *kubernetes.Clientset
}

func NewEventWatcher(c *kubernetes.Clientset) EventWatcher {
	w := EventWatcher{c}
	return w
}

func (w EventWatcher) Observe() {
	api := w.Client.CoreV1()
	listOptions := metav1.ListOptions{}

	watcher, err := api.Events(metav1.NamespaceAll).Watch(listOptions)
	if err != nil {
		fmt.Printf("Issues when setting up events watcher. %v", err)
	}

	ch := watcher.ResultChan()

	for ev := range ch {
		eventObj, ok := ev.Object.(*v1.Event)
		if !ok {
			fmt.Printf("Expected Event, but received an object of an unknown type. ")
		}

		fmt.Printf("Received event: %s %s %s\n", eventObj.Namespace, eventObj.Message, eventObj.Reason)

		//push event to AppD
	}
	fmt.Println("Exiting event eventWatcher")
}
