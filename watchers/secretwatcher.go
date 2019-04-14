package watchers

import (
	"sync"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	log "github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"github.com/appdynamics/cluster-agent/config"
	"github.com/appdynamics/cluster-agent/utils"
)

type SecretWathcer struct {
	Client      *kubernetes.Clientset
	SecretCache map[string]v1.Secret
	ConfManager *config.MutexConfigManager
	Listener    *WatchListener
	UpdateDelay bool
	Logger      *log.Logger
}

var lockSecrets = sync.RWMutex{}

func NewSecretWathcer(client *kubernetes.Clientset, secret *config.MutexConfigManager, cache *map[string]v1.Secret, listener WatchListener, l *log.Logger) *SecretWathcer {
	sw := SecretWathcer{Client: client, SecretCache: *cache, ConfManager: secret, Listener: &listener, Logger: l}
	sw.UpdateDelay = true
	return &sw
}

func (pw SecretWathcer) WatchSecrets() {
	api := pw.Client.CoreV1()
	listOptions := metav1.ListOptions{}
	pw.Logger.Info("Starting Secrets Watcher...")

	bag := (*pw.ConfManager).Get()
	dashTimer := time.NewTimer(time.Second * time.Duration(bag.SnapshotSyncInterval))
	go func() {
		<-dashTimer.C
		pw.UpdateDelay = false
		pw.Logger.Info("Secret Update Delay lifted.")
	}()

	watcher, err := api.Secrets(metav1.NamespaceAll).Watch(listOptions)
	if err != nil {
		pw.Logger.WithField("error", err).Error("Issues when setting up Secret watcher. Aborting...")
	} else {

		ch := watcher.ResultChan()

		for ev := range ch {
			secret, ok := ev.Object.(*v1.Secret)
			if !ok {
				pw.Logger.Warn("Expected Secrets, but received an object of an unknown type. ")
				continue
			}
			switch ev.Type {
			case watch.Added:
				pw.onNewConfig(secret)
				break

			case watch.Deleted:
				pw.onDeleteConfig(secret)
				break

			case watch.Modified:
				pw.onUpdateConfig(secret)
				break
			}

		}
	}
	pw.Logger.Info("Exiting secret watcher.")
}

func (pw *SecretWathcer) qualifies(secret *v1.Secret) bool {
	bag := pw.ConfManager.Get()
	return utils.NSQualifiesForMonitoring(secret.Namespace, bag)
}

func (pw SecretWathcer) onNewConfig(secret *v1.Secret) {
	if !pw.qualifies(secret) {
		return
	}
	pw.updateMap(secret)
}

func (pw SecretWathcer) onDeleteConfig(secret *v1.Secret) {
	if !pw.qualifies(secret) {
		return
	}
	_, ok := pw.SecretCache[utils.GetSecretKey(secret)]
	if ok {
		lockSecrets.Lock()
		defer lockSecrets.Unlock()
		delete(pw.SecretCache, utils.GetSecretKey(secret))
		pw.notifyListener(secret.Namespace)
	}
}

func (pw SecretWathcer) onUpdateConfig(secret *v1.Secret) {
	if !pw.qualifies(secret) {
		return
	}
	pw.updateMap(secret)
}

func (pw SecretWathcer) notifyListener(namespace string) {
	if pw.Listener != nil && !pw.UpdateDelay {
		(*pw.Listener).CacheUpdated(namespace)
	}
}

func (pw SecretWathcer) updateMap(secret *v1.Secret) {
	lockSecrets.Lock()
	defer lockSecrets.Unlock()
	pw.SecretCache[utils.GetSecretKey(secret)] = *secret
	pw.notifyListener(secret.Namespace)
}

func (pw SecretWathcer) CloneMap() map[string]v1.Secret {
	lockSecrets.RLock()
	defer lockSecrets.RUnlock()
	m := make(map[string]v1.Secret)
	for key, val := range pw.SecretCache {
		m[key] = val
	}
	return m
}
