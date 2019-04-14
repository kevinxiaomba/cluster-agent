package watchers

import (
	"sync"

	"github.com/appdynamics/cluster-agent/config"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	"github.com/appdynamics/cluster-agent/utils"
)

type PVCWatcher struct {
	Client      *kubernetes.Clientset
	PVCCache    map[string]v1.PersistentVolumeClaim
	ConfManager *config.MutexConfigManager
	Logger      *log.Logger
}

var lockPVC = sync.RWMutex{}

func NewPVCWatcher(client *kubernetes.Clientset, cm *config.MutexConfigManager, cache *map[string]v1.PersistentVolumeClaim, l *log.Logger) *PVCWatcher {
	epw := PVCWatcher{Client: client, PVCCache: *cache, ConfManager: cm, Logger: l}
	return &epw
}

func (pw *PVCWatcher) qualifies(p *v1.PersistentVolumeClaim) bool {
	return (len((*pw.ConfManager).Get().NsToMonitor) == 0 ||
		utils.StringInSlice(p.Namespace, (*pw.ConfManager).Get().NsToMonitor)) &&
		!utils.StringInSlice(p.Namespace, (*pw.ConfManager).Get().NsToMonitorExclude)
}

func (pw PVCWatcher) WatchPVC() {
	api := pw.Client.CoreV1()
	listOptions := metav1.ListOptions{}
	pw.Logger.Info("Starting Persistent Volume Claim Watcher...")

	watcher, err := api.PersistentVolumeClaims(metav1.NamespaceAll).Watch(listOptions)
	if err != nil {
		pw.Logger.WithField("error", err).Error("Issues when setting up PVC watcher. Aborting...")
	} else {

		ch := watcher.ResultChan()

		for ev := range ch {
			pvc, ok := ev.Object.(*v1.PersistentVolumeClaim)
			if !ok {
				pw.Logger.Warn("Expected PVC, but received an object of an unknown type.")
				continue
			}
			switch ev.Type {
			case watch.Added:
				pw.onNewPVC(pvc)
				break

			case watch.Deleted:
				pw.onDeletePVC(pvc)
				break

			case watch.Modified:
				pw.onUpdatePVC(pvc)
				break
			}

		}
	}
	pw.Logger.Info("Exiting PVC watcher.")
}

func (pw PVCWatcher) onNewPVC(pvc *v1.PersistentVolumeClaim) {
	if !pw.qualifies(pvc) {
		return
	}
	pw.Logger.WithField("name", pvc.Name).Debug("Added PVC")
	pw.updateMap(pvc)
}

func (pw PVCWatcher) onDeletePVC(pvc *v1.PersistentVolumeClaim) {
	if !pw.qualifies(pvc) {
		return
	}
	_, ok := pw.PVCCache[utils.GetKey(pvc.Namespace, pvc.Name)]
	if ok {
		lockPVC.Lock()
		defer lockPVC.Unlock()
		delete(pw.PVCCache, utils.GetKey(pvc.Namespace, pvc.Name))
		pw.Logger.WithField("name", pvc.Name).Debug("Deleted PVC")
	}
}

func (pw PVCWatcher) onUpdatePVC(pvc *v1.PersistentVolumeClaim) {
	if !pw.qualifies(pvc) {
		return
	}
	pw.updateMap(pvc)
	pw.Logger.WithField("name", pvc.Name).Debug("Updated PVC")
}

func (pw PVCWatcher) updateMap(pvc *v1.PersistentVolumeClaim) {
	lockPVC.Lock()
	defer lockPVC.Unlock()
	pw.PVCCache[utils.GetKey(pvc.Namespace, pvc.Name)] = *pvc
}

func (pw PVCWatcher) CloneMap() map[string]v1.PersistentVolumeClaim {
	lockPVC.RLock()
	defer lockPVC.RUnlock()
	m := make(map[string]v1.PersistentVolumeClaim)
	for key, val := range pw.PVCCache {
		m[key] = val
	}

	return m
}
