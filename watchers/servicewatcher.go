package watchers

import (
	"fmt"
	"sync"
	"time"

	"strings"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"github.com/sjeltuhin/clusterAgent/config"
	m "github.com/sjeltuhin/clusterAgent/models"
	"github.com/sjeltuhin/clusterAgent/utils"
)

type ServiceWatcher struct {
	Client      *kubernetes.Clientset
	SvcCache    map[string]m.ServiceSchema
	ConfManager *config.MutexConfigManager
	Listener    *WatchListener
	UpdateDelay bool
}

var lockServices = sync.RWMutex{}

func NewServiceWatcher(client *kubernetes.Clientset, cm *config.MutexConfigManager, cache *map[string]m.ServiceSchema, l WatchListener) *ServiceWatcher {
	sw := ServiceWatcher{Client: client, SvcCache: *cache, ConfManager: cm, Listener: &l}
	sw.UpdateDelay = true
	return &sw
}

func (pw ServiceWatcher) WatchServices() {
	api := pw.Client.CoreV1()
	listOptions := metav1.ListOptions{}
	fmt.Println("Starting Service Watcher...")

	bag := (*pw.ConfManager).Get()
	dashTimer := time.NewTimer(time.Second * time.Duration(bag.SnapshotSyncInterval))
	go func() {
		<-dashTimer.C
		pw.UpdateDelay = false
		fmt.Println("Svc UpdateDelay lifted.")
	}()

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

func (pw *ServiceWatcher) notifyListener(namespace string) {
	if pw.Listener != nil && !pw.UpdateDelay {
		(*pw.Listener).CacheUpdated(namespace)
	}
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
		pw.notifyListener(svc.Namespace)
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
	key := utils.GetK8sServiceKey(svc)
	svcSchema := m.NewServiceSchema(svc)
	pw.SvcCache[key] = *svcSchema
	pw.notifyListener(svc.Namespace)
}

func (pw ServiceWatcher) CloneMap() map[string]m.ServiceSchema {
	lockServices.RLock()
	defer lockServices.RUnlock()
	m := make(map[string]m.ServiceSchema)
	for key, val := range pw.SvcCache {
		m[key] = val
	}
	fmt.Printf("Cloned %d services\n", len(m))
	return m
}

func (pw *ServiceWatcher) UpdateServiceCache() {
	lockServices.RLock()
	defer lockServices.RUnlock()

	external := make(map[string]m.ServiceSchema)
	for key, svcSchema := range pw.SvcCache {
		if svcSchema.HasExternalService {
			external[key] = svcSchema
		}
	}

	//check validity of external services
	for k, headless := range external {
		for kk, svcObj := range pw.SvcCache {
			path := fmt.Sprintf("%s.%s", svcObj.Name, svcObj.Namespace)
			if strings.Contains(headless.ExternalName, path) {
				headless.ExternalSvcValid = true
				pw.SvcCache[k] = headless
				svcObj.ExternallyReferenced(true)
				pw.SvcCache[kk] = svcObj
				break
			}
		}
	}
}
