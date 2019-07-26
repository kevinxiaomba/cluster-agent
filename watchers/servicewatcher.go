package watchers

import (
	"fmt"
	"sync"
	"time"

	"strings"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"github.com/appdynamics/cluster-agent/config"
	m "github.com/appdynamics/cluster-agent/models"
	"github.com/appdynamics/cluster-agent/utils"
)

type ServiceWatcher struct {
	Client       *kubernetes.Clientset
	LockServices *sync.RWMutex
	SvcCache     map[string]m.ServiceSchema
	ConfManager  *config.MutexConfigManager
	Listener     *WatchListener
	UpdateDelay  bool
	Logger       *log.Logger
}

func NewServiceWatcher(client *kubernetes.Clientset, cm *config.MutexConfigManager, cache *map[string]m.ServiceSchema, listener WatchListener, l *log.Logger, lock *sync.RWMutex) *ServiceWatcher {
	sw := ServiceWatcher{Client: client, SvcCache: *cache, ConfManager: cm, Listener: &listener, Logger: l, LockServices: lock}
	sw.UpdateDelay = true
	return &sw
}

func (pw ServiceWatcher) WatchServices() {
	api := pw.Client.CoreV1()
	listOptions := metav1.ListOptions{}
	pw.Logger.Info("Starting Service Watcher...")

	bag := (*pw.ConfManager).Get()
	dashTimer := time.NewTimer(time.Second * time.Duration(bag.SnapshotSyncInterval))
	go func() {
		<-dashTimer.C
		pw.UpdateDelay = false
		pw.Logger.Info("Service  Update delay lifted.")
	}()

	watcher, err := api.Services(metav1.NamespaceAll).Watch(listOptions)
	if err != nil {
		pw.Logger.WithField("error", err).Error("Issues when setting up Service watcher. Aborting...")
	} else {

		ch := watcher.ResultChan()

		for ev := range ch {
			svc, ok := ev.Object.(*v1.Service)
			if !ok {
				pw.Logger.Warn("Expected Service, but received an object of an unknown type. ")
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
	}
	pw.Logger.Info("Exiting Service watcher.")
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
		pw.Logger.Debugf("Service %s/%s is not qualified\n", svc.Name, svc.Namespace)
		return
	}
	pw.updateMap(svc)
}

func (pw ServiceWatcher) onDeleteService(svc *v1.Service) {
	if !pw.qualifies(svc) {
		return
	}
	pw.LockServices.RLock()
	_, ok := pw.SvcCache[utils.GetK8sServiceKey(svc)]
	pw.LockServices.RUnlock()
	if ok {
		pw.LockServices.Lock()
		defer pw.LockServices.Unlock()
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
	pw.LockServices.Lock()
	defer pw.LockServices.Unlock()
	key := utils.GetK8sServiceKey(svc)
	svcSchema := m.NewServiceSchema(svc)
	pw.SvcCache[key] = *svcSchema
	pw.notifyListener(svc.Namespace)
}

func (pw ServiceWatcher) CloneMap() map[string]m.ServiceSchema {
	pw.LockServices.RLock()
	defer pw.LockServices.RUnlock()
	m := make(map[string]m.ServiceSchema)
	for key, val := range pw.SvcCache {
		m[key] = val
	}
	return m
}

func (pw *ServiceWatcher) UpdateServiceCache() {
	pw.LockServices.RLock()

	external := make(map[string]m.ServiceSchema)
	for key, svcSchema := range pw.SvcCache {
		if svcSchema.HasExternalService {
			external[key] = svcSchema
		}
	}
	pw.LockServices.RUnlock()

	//check validity of external services
	for k, headless := range external {
		svcObj, kk := pw.findExternalService(headless.ExternalName)
		if svcObj != nil {
			pw.LockServices.Lock()
			headless.ExternalSvcValid = true
			pw.SvcCache[k] = headless
			svcObj.ExternallyReferenced(true)
			pw.SvcCache[kk] = *svcObj
			pw.LockServices.Unlock()
		}
	}
}

func (pw *ServiceWatcher) findExternalService(headlessName string) (*m.ServiceSchema, string) {
	pw.LockServices.RLock()
	defer pw.LockServices.RUnlock()
	for kk, svcObj := range pw.SvcCache {
		path := fmt.Sprintf("%s.%s", svcObj.Name, svcObj.Namespace)
		if strings.Contains(headlessName, path) {
			return &svcObj, kk
		}
	}
	return nil, ""
}
