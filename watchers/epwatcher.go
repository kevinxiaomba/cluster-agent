package watchers

import (
	"sync"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"github.com/appdynamics/cluster-agent/config"
	"github.com/appdynamics/cluster-agent/utils"
)

type EndpointWatcher struct {
	Client        *kubernetes.Clientset
	LockEP        *sync.RWMutex
	EndpointCache map[string]v1.Endpoints
	UpdatedCache  map[string]v1.Endpoints
	ConfManager   *config.MutexConfigManager
	Logger        *log.Logger
}

var lockUpdated = sync.RWMutex{}

func NewEndpointWatcher(client *kubernetes.Clientset, cm *config.MutexConfigManager, cache *map[string]v1.Endpoints, l *log.Logger, lock *sync.RWMutex) *EndpointWatcher {
	epw := EndpointWatcher{Client: client, EndpointCache: *cache, UpdatedCache: make(map[string]v1.Endpoints),
		ConfManager: cm, Logger: l, LockEP: lock}
	return &epw
}

func (pw *EndpointWatcher) qualifies(p *v1.Endpoints) bool {
	bag := (*pw.ConfManager).Get()
	return utils.NSQualifiesForMonitoring(p.Namespace, bag)
}

//end points
func (pw EndpointWatcher) WatchEndpoints() {
	api := pw.Client.CoreV1()
	listOptions := metav1.ListOptions{}
	pw.Logger.Info("Starting Endpoint Watcher...")

	watcher, err := api.Endpoints(metav1.NamespaceAll).Watch(listOptions)
	if err != nil {
		pw.Logger.WithField("error", err).Error("Issues when setting up Endpoint watcher. Aborting...")
	} else {

		ch := watcher.ResultChan()

		for ev := range ch {
			ep, ok := ev.Object.(*v1.Endpoints)
			if !ok {
				pw.Logger.Warn("Expected endpoints, but received an object of an unknown type. ")
				continue
			}
			switch ev.Type {
			case watch.Added:
				pw.onNewEndpoint(ep)
				break

			case watch.Deleted:
				pw.onDeleteEndpoint(ep)
				break

			case watch.Modified:
				pw.onUpdateEndpoint(ep)
				break
			}

		}
	}
	pw.Logger.Info("Exiting endpoint watcher.")
}

func (pw EndpointWatcher) onNewEndpoint(ep *v1.Endpoints) {
	if !pw.qualifies(ep) {
		return
	}
	pw.updateMap(ep)
	pw.updateChanged(ep)
}

func (pw EndpointWatcher) onDeleteEndpoint(ep *v1.Endpoints) {
	if !pw.qualifies(ep) {
		return
	}
	pw.LockEP.RLock()
	_, ok := pw.EndpointCache[utils.GetEndpointKey(ep)]
	pw.LockEP.RUnlock()

	if ok {
		pw.LockEP.Lock()
		defer pw.LockEP.Unlock()
		delete(pw.EndpointCache, utils.GetEndpointKey(ep))
	}
}

func (pw EndpointWatcher) onUpdateEndpoint(ep *v1.Endpoints) {
	if !pw.qualifies(ep) {
		return
	}
	pw.updateMap(ep)
	pw.updateChanged(ep)
}

func (pw EndpointWatcher) updateMap(ep *v1.Endpoints) {
	pw.LockEP.Lock()
	defer pw.LockEP.Unlock()
	pw.EndpointCache[utils.GetEndpointKey(ep)] = *ep
}

func (pw EndpointWatcher) updateChanged(ep *v1.Endpoints) {
	lockUpdated.Lock()
	defer lockUpdated.Unlock()
	pw.UpdatedCache[utils.GetEndpointKey(ep)] = *ep
}
func (pw EndpointWatcher) CloneMap() map[string]v1.Endpoints {
	pw.LockEP.RLock()
	defer pw.LockEP.RUnlock()
	m := make(map[string]v1.Endpoints)
	for key, val := range pw.EndpointCache {
		m[key] = val
	}
	return m
}

func (pw EndpointWatcher) GetUpdated() map[string]v1.Endpoints {
	lockUpdated.RLock()
	defer lockUpdated.RUnlock()
	m := make(map[string]v1.Endpoints)
	for key, val := range pw.UpdatedCache {
		m[key] = val
	}
	pw.UpdatedCache = make(map[string]v1.Endpoints)
	return m
}

func (pw EndpointWatcher) ResetUpdatedCache() {
	//reset
	lockUpdated.Lock()
	defer lockUpdated.Unlock()
	pw.UpdatedCache = make(map[string]v1.Endpoints)
}
