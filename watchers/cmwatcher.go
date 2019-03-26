package watchers

import (
	"fmt"
	"sync"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"github.com/sjeltuhin/clusterAgent/config"
	"github.com/sjeltuhin/clusterAgent/utils"
)

type ConfigWatcher struct {
	Client      *kubernetes.Clientset
	CMCache     map[string]v1.ConfigMap
	ConfManager *config.MutexConfigManager
	Listener    *WatchListener
	UpdateDelay bool
}

var lockConfigs = sync.RWMutex{}

func NewConfigWatcher(client *kubernetes.Clientset, cm *config.MutexConfigManager, cache *map[string]v1.ConfigMap, l WatchListener) *ConfigWatcher {
	sw := ConfigWatcher{Client: client, CMCache: *cache, ConfManager: cm, Listener: &l}
	sw.UpdateDelay = true
	return &sw
}

func (pw ConfigWatcher) WatchConfigs() {
	api := pw.Client.CoreV1()
	listOptions := metav1.ListOptions{}
	fmt.Println("Starting Config Watcher...")
	bag := (*pw.ConfManager).Get()
	dashTimer := time.NewTimer(time.Second * time.Duration(bag.SnapshotSyncInterval))
	go func() {
		<-dashTimer.C
		pw.UpdateDelay = false
		fmt.Println("CM UpdateDelay lifted.")
	}()

	watcher, err := api.ConfigMaps(metav1.NamespaceAll).Watch(listOptions)
	if err != nil {
		fmt.Printf("Issues when setting up cm watcher. %v", err)
	}

	ch := watcher.ResultChan()

	for ev := range ch {
		cm, ok := ev.Object.(*v1.ConfigMap)
		if !ok {
			fmt.Printf("Expected Config, but received an object of an unknown type. ")
			continue
		}
		switch ev.Type {
		case watch.Added:
			pw.onNewConfig(cm)
			break

		case watch.Deleted:
			pw.onDeleteConfig(cm)
			break

		case watch.Modified:
			pw.onUpdateConfig(cm)
			break
		}

	}
	fmt.Println("Exiting cm watcher.")
}

func (pw *ConfigWatcher) qualifies(cm *v1.ConfigMap) bool {
	bag := pw.ConfManager.Get()
	return utils.NSQualifiesForMonitoring(cm.Namespace, bag)
}

func (pw *ConfigWatcher) notifyListener(namespace string) {
	fmt.Printf("NotifyListener is active:  %t\n", pw.Listener != nil)
	if pw.Listener != nil && !pw.UpdateDelay {
		fmt.Printf("Sending update for namespace %s\n", namespace)
		(*pw.Listener).CacheUpdated(namespace)
	}
}

func (pw ConfigWatcher) onNewConfig(cm *v1.ConfigMap) {
	if !pw.qualifies(cm) {
		fmt.Printf("Config %s/%s is not qualified\n", cm.Name, cm.Namespace)
		return
	}
	pw.updateMap(cm)
	pw.notifyListener(cm.Namespace)
}

func (pw ConfigWatcher) onDeleteConfig(cm *v1.ConfigMap) {
	fmt.Printf("Config Map deleted %s/%s\n", cm.Name, cm.Namespace)
	if !pw.qualifies(cm) {
		return
	}
	_, ok := pw.CMCache[utils.GetConfigMapKey(cm)]
	if ok {
		lockConfigs.Lock()
		defer lockConfigs.Unlock()
		delete(pw.CMCache, utils.GetConfigMapKey(cm))
		pw.notifyListener(cm.Namespace)
	}
}

func (pw ConfigWatcher) onUpdateConfig(cm *v1.ConfigMap) {
	if !pw.qualifies(cm) {
		return
	}
	pw.updateMap(cm)
}

func (pw ConfigWatcher) updateMap(cm *v1.ConfigMap) {
	lockConfigs.Lock()
	defer lockConfigs.Unlock()
	pw.CMCache[utils.GetConfigMapKey(cm)] = *cm
}

func (pw ConfigWatcher) CloneMap() map[string]v1.ConfigMap {
	lockConfigs.RLock()
	defer lockConfigs.RUnlock()
	m := make(map[string]v1.ConfigMap)
	for key, val := range pw.CMCache {
		fmt.Printf("Cloned configmap with key %s\n", key)
		m[key] = val
	}
	fmt.Printf("Cloned %d Configs\n", len(m))
	return m
}
