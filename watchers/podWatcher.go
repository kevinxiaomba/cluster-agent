package watchers

import (
	"fmt"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

type PodWatcher struct {
	Client *kubernetes.Clientset
}

func NewPodWatcher(c *kubernetes.Clientset) PodWatcher {
	w := PodWatcher{c}
	return w
}

func (w PodWatcher) Observe() {
	api := w.Client.CoreV1()
	listOptions := metav1.ListOptions{}
	fmt.Println("Starting PodWatcher.")

	watcher, err := api.Pods(metav1.NamespaceAll).Watch(listOptions)
	if err != nil {
		fmt.Printf("Issues when setting up pod watcher. %v", err)
	}

	ch := watcher.ResultChan()

	for ev := range ch {
		podObj, ok := ev.Object.(*v1.Pod)
		if !ok {
			fmt.Printf("Expected Pod, but received an object of an unknown type. ")
			continue
		}
		switch ev.Type {
		case watch.Added:
			fmt.Printf("Added Pod: %s %s\n", podObj.Namespace, podObj.Name)
			break

		case watch.Deleted:
			fmt.Printf("Deleted Pod: %s %s\n", podObj.Namespace, podObj.Name)
			break

		case watch.Modified:
			fmt.Printf("Pod changed: %s %s\n", podObj.Namespace, podObj.Name)
			break
		}
		//push event to AppD
	}
	fmt.Println("Exiting event PodWatcher.")
}
