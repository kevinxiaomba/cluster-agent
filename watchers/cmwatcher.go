package watchers

import (
	"sync"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	log "github.com/sirupsen/logrus"

	"k8s.io/client-go/kubernetes"

	"github.com/appdynamics/cluster-agent/config"
	"github.com/appdynamics/cluster-agent/utils"
)

type ConfigWatcher struct {
	Client      *kubernetes.Clientset
	LockConfigs *sync.RWMutex
	CMCache     map[string]v1.ConfigMap
	ConfManager *config.MutexConfigManager
	Listener    *WatchListener
	UpdateDelay bool
	Logger      *log.Logger
}

func NewConfigWatcher(client *kubernetes.Clientset, cm *config.MutexConfigManager, cache *map[string]v1.ConfigMap, listener WatchListener, l *log.Logger, lock *sync.RWMutex) *ConfigWatcher {
	sw := ConfigWatcher{Client: client, CMCache: *cache, ConfManager: cm, Listener: &listener, Logger: l, LockConfigs: lock}
	sw.UpdateDelay = true
	return &sw
}

func (pw ConfigWatcher) WatchConfigs() {
	api := pw.Client.CoreV1()
	listOptions := metav1.ListOptions{}
	pw.Logger.Info("Starting Config Watcher...")
	bag := (*pw.ConfManager).Get()
	dashTimer := time.NewTimer(time.Second * time.Duration(bag.SnapshotSyncInterval))
	go func() {
		<-dashTimer.C
		pw.UpdateDelay = false
		pw.Logger.Info("ConfigMap Update delay lifted.")
	}()

	watcher, err := api.ConfigMaps(metav1.NamespaceAll).Watch(listOptions)
	if err != nil {
		pw.Logger.WithField("error", err).Error("Issues when setting up Config watcher. Aborting...")
	} else {

		ch := watcher.ResultChan()

		for ev := range ch {
			cm, ok := ev.Object.(*v1.ConfigMap)
			if !ok {
				pw.Logger.Warn("Expected ConfigMap, but received an object of an unknown type.")
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
	}
	pw.Logger.Info("Exiting Config watcher.")
}

func (pw *ConfigWatcher) qualifies(cm *v1.ConfigMap) bool {
	bag := pw.ConfManager.Get()
	return utils.NSQualifiesForMonitoring(cm.Namespace, bag)
}

func (pw *ConfigWatcher) notifyListener(namespace string) {
	if pw.Listener != nil && !pw.UpdateDelay {
		(*pw.Listener).CacheUpdated(namespace)
	}
}

func (pw ConfigWatcher) onNewConfig(cm *v1.ConfigMap) {
	if !pw.qualifies(cm) {
		return
	}
	pw.updateMap(cm)
	pw.notifyListener(cm.Namespace)
}

func (pw ConfigWatcher) onDeleteConfig(cm *v1.ConfigMap) {
	pw.Logger.WithFields(log.Fields{"name": cm.Name, "namespace": cm.Namespace}).Debug("Config Map deleted")
	if !pw.qualifies(cm) {
		return
	}
	pw.LockConfigs.RLock()
	_, ok := pw.CMCache[utils.GetConfigMapKey(cm)]
	pw.LockConfigs.RUnlock()

	if ok {
		pw.LockConfigs.Lock()
		defer pw.LockConfigs.Unlock()
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
	pw.LockConfigs.Lock()
	defer pw.LockConfigs.Unlock()
	pw.CMCache[utils.GetConfigMapKey(cm)] = *cm
}

func (pw ConfigWatcher) CloneMap() map[string]v1.ConfigMap {
	pw.LockConfigs.RLock()
	defer pw.LockConfigs.RUnlock()
	m := make(map[string]v1.ConfigMap)
	for key, val := range pw.CMCache {
		m[key] = val
	}
	return m
}
