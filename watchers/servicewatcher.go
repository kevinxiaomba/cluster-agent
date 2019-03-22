package watchers

import (
	"fmt"
	"sync"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"github.com/sjeltuhin/clusterAgent/config"
	"github.com/sjeltuhin/clusterAgent/utils"
)

type ServiceWatcher struct {
	Client      *kubernetes.Clientset
	SvcCache    map[string]v1.Service
	ConfManager *config.MutexConfigManager
}

var lockServices = sync.RWMutex{}

func NewServiceWatcher(client *kubernetes.Clientset, cm *config.MutexConfigManager) *ServiceWatcher {
	sw := ServiceWatcher{Client: client, SvcCache: make(map[string]v1.Service), ConfManager: cm}
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

func (pw *ServiceWatcher) qualifies(svc *v1.Service) bool {
	bag := pw.ConfManager.Get()
	return utils.NSQualifiesForMonitoring(svc.Namespace, bag)
}

func (pw ServiceWatcher) onNewService(svc *v1.Service) {
	if !pw.qualifies(svc) {
		fmt.Printf("Service %s/%s is not qualified\n", svc.Name, svc.Namespace)
		return
	}
	pw.updateMap(svc)
}

func (pw ServiceWatcher) onDeleteService(svc *v1.Service) {
	if !pw.qualifies(svc) {
		return
	}
	_, ok := pw.SvcCache[utils.GetK8sServiceKey(svc)]
	if ok {
		lockServices.Lock()
		defer lockServices.Unlock()
		delete(pw.SvcCache, utils.GetK8sServiceKey(svc))
	}
}

func (pw ServiceWatcher) onUpdateService(svc *v1.Service) {
	if !pw.qualifies(svc) {
		return
	}
	pw.updateMap(svc)
}

func (pw ServiceWatcher) updateMap(svc *v1.Service) {
	lockServices.Lock()
	defer lockServices.Unlock()
	pw.SvcCache[utils.GetK8sServiceKey(svc)] = *svc
}

func (pw ServiceWatcher) CloneMap() map[string]v1.Service {
	lockServices.RLock()
	defer lockServices.RUnlock()
	m := make(map[string]v1.Service)
	for key, val := range pw.SvcCache {
		m[key] = val
	}
	fmt.Printf("Cloned %d services\n", len(m))
	return m
}
