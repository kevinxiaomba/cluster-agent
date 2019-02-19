package watchers

import (
	"fmt"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"github.com/sjeltuhin/clusterAgent/utils"
	//	m "github.com/sjeltuhin/clusterAgent/models"
)

type ServiceWatcher struct {
	Client   *kubernetes.Clientset
	SvcCache map[string]v1.Service
}

func NewServiceWatcher(client *kubernetes.Clientset) *ServiceWatcher {
	sw := ServiceWatcher{Client: client, SvcCache: make(map[string]v1.Service)}
	return &sw
}

func (pw ServiceWatcher) WatchServices() {
	api := pw.Client.CoreV1()
	listOptions := metav1.ListOptions{}
	fmt.Println("Starting Service Watcher...")

	watcher, err := api.Services(metav1.NamespaceAll).Watch(listOptions)
	if err != nil {
		fmt.Printf("Issues when setting up svc watcher. %v", err)
	}

	ch := watcher.ResultChan()

	for ev := range ch {
		svc, ok := ev.Object.(*v1.Service)
		if !ok {
			fmt.Printf("Expected Service, but received an object of an unknown type. ")
			continue
		}
		switch ev.Type {
		case watch.Added:
			pw.onNewService(svc)
			break

		case watch.Deleted:
			pw.onDeleteService(svc)
			break

		case watch.Modified:
			pw.onUpdateService(svc)
			break
		}

	}
	fmt.Println("Exiting svc watcher.")
}

func (pw ServiceWatcher) onNewService(svc *v1.Service) {
	pw.SvcCache[utils.GetK8sServiceKey(svc)] = *svc
}

func (pw ServiceWatcher) onDeleteService(svc *v1.Service) {
	_, ok := pw.SvcCache[utils.GetK8sServiceKey(svc)]
	if ok {
		delete(pw.SvcCache, utils.GetK8sServiceKey(svc))
	}
}

func (pw ServiceWatcher) onUpdateService(svc *v1.Service) {
	pw.SvcCache[utils.GetK8sServiceKey(svc)] = *svc
}
