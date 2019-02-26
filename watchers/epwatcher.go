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

type EndpointWatcher struct {
	Client        *kubernetes.Clientset
	EndpointCache map[string]v1.Endpoints
	ConfManager   *config.MutexConfigManager
}

var lockEP = sync.RWMutex{}

func NewEndpointWatcher(client *kubernetes.Clientset, cm *config.MutexConfigManager) *EndpointWatcher {
	epw := EndpointWatcher{Client: client, EndpointCache: make(map[string]v1.Endpoints), ConfManager: cm}
	return &epw
}

func (pw *EndpointWatcher) qualifies(p *v1.Endpoints) bool {
	return (len((*pw.ConfManager).Get().IncludeNsToInstrument) == 0 ||
		utils.StringInSlice(p.Namespace, (*pw.ConfManager).Get().IncludeNsToInstrument)) &&
		!utils.StringInSlice(p.Namespace, (*pw.ConfManager).Get().ExcludeNsToInstrument)
}

//end points
func (pw EndpointWatcher) WatchEndpoints() {
	api := pw.Client.CoreV1()
	listOptions := metav1.ListOptions{}
	fmt.Println("Starting Endpoint Watcher...")

	watcher, err := api.Endpoints(metav1.NamespaceAll).Watch(listOptions)
	if err != nil {
		fmt.Printf("Issues when setting up endpoint watcher. %v", err)
	}

	ch := watcher.ResultChan()

	for ev := range ch {
		ep, ok := ev.Object.(*v1.Endpoints)
		if !ok {
			fmt.Printf("Expected endpoints, but received an object of an unknown type. ")
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
	fmt.Println("Exiting endpoint watcher.")
}

func (pw EndpointWatcher) onNewEndpoint(ep *v1.Endpoints) {
	if !pw.qualifies(ep) {
		return
	}
	pw.updateMap(ep)
}

func (pw EndpointWatcher) onDeleteEndpoint(ep *v1.Endpoints) {
	if !pw.qualifies(ep) {
		return
	}
	_, ok := pw.EndpointCache[utils.GetEndpointKey(ep)]
	if ok {
		lockEP.Lock()
		defer lockEP.Unlock()
		delete(pw.EndpointCache, utils.GetEndpointKey(ep))
	}
}

func (pw EndpointWatcher) onUpdateEndpoint(ep *v1.Endpoints) {
	if !pw.qualifies(ep) {
		return
	}
	pw.updateMap(ep)
}

func (pw EndpointWatcher) updateMap(ep *v1.Endpoints) {
	lockEP.Lock()
	defer lockEP.Unlock()
	pw.EndpointCache[utils.GetEndpointKey(ep)] = *ep
}

func (pw EndpointWatcher) CloneMap() map[string]v1.Endpoints {
	lockEP.RLock()
	defer lockEP.RUnlock()
	m := make(map[string]v1.Endpoints)
	for key, val := range pw.EndpointCache {
		pw.EndpointCache[key] = val
	}

	return m
}
