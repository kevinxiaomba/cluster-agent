package watchers

import (
	"fmt"
	"sync"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/sjeltuhin/clusterAgent/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	m "github.com/sjeltuhin/clusterAgent/models"
	"github.com/sjeltuhin/clusterAgent/utils"
)

var lockNS = sync.RWMutex{}

type NSWatcher struct {
	Client      *kubernetes.Clientset
	NSCache     map[string]m.NsSchema
	ConfManager *config.MutexConfigManager
}

func NewNSWatcher(client *kubernetes.Clientset, cm *config.MutexConfigManager, cache *map[string]m.NsSchema) *NSWatcher {
	epw := NSWatcher{Client: client, NSCache: *cache, ConfManager: cm}
	return &epw
}

func (pw *NSWatcher) qualifies(p *v1.Namespace) bool {
	return (len((*pw.ConfManager).Get().NsToMonitor) == 0 ||
		utils.StringInSlice(p.Name, (*pw.ConfManager).Get().NsToMonitor)) &&
		!utils.StringInSlice(p.Name, (*pw.ConfManager).Get().NsToMonitorExclude)
}

//quotas
func (pw NSWatcher) WatchNamespaces() {
	api := pw.Client.CoreV1()
	listOptions := metav1.ListOptions{}
	fmt.Println("Starting Namespace Watcher...")

	watcher, err := api.Namespaces().Watch(listOptions)
	if err != nil {
		fmt.Printf("Issues when setting up Namespace watcher. %v", err)
	}

	ch := watcher.ResultChan()

	for ev := range ch {
		ns, ok := ev.Object.(*v1.Namespace)
		if !ok {
			fmt.Printf("Expected Namespace, but received an object of an unknown type. ")
			continue
		}
		switch ev.Type {
		case watch.Added:
			pw.onNewNamespace(ns)
			break

		case watch.Deleted:
			pw.onDeleteNamespace(ns)
			break

		case watch.Modified:
			pw.onUpdateNamespace(ns)
			break
		}

	}
	fmt.Println("Exiting Namespace watcher.")
}

func (pw NSWatcher) onNewNamespace(ns *v1.Namespace) {
	if !pw.qualifies(ns) {
		return
	}

	pw.updateMap(ns)
}

func (pw NSWatcher) onDeleteNamespace(ns *v1.Namespace) {
	if !pw.qualifies(ns) {
		return
	}
	key := ns.Name
	_, ok := pw.NSCache[key]
	if ok {
		lockNS.Lock()
		defer lockNS.Unlock()
		delete(pw.NSCache, key)
	}
}

func (pw NSWatcher) onUpdateNamespace(ns *v1.Namespace) {
	if !pw.qualifies(ns) {
		return
	}
	pw.updateMap(ns)
}

func (pw NSWatcher) updateMap(ns *v1.Namespace) {
	lockNS.Lock()
	defer lockNS.Unlock()
	nsSchema := m.NewNsSchema(ns, (*pw.ConfManager).Get())
	pw.NSCache[ns.Name] = nsSchema
}

func (pw NSWatcher) CloneMap() map[string]m.NsSchema {
	lockNS.RLock()
	defer lockNS.RUnlock()
	m := make(map[string]m.NsSchema)
	for key, val := range pw.NSCache {
		m[key] = val
	}

	return m
}
