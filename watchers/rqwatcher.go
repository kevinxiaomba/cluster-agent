package watchers

import (
	"encoding/json"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/appdynamics/cluster-agent/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	app "github.com/appdynamics/cluster-agent/appd"
	m "github.com/appdynamics/cluster-agent/models"
	"github.com/appdynamics/cluster-agent/utils"
)

var lockRQ = sync.RWMutex{}

type RQWatcher struct {
	Client       *kubernetes.Clientset
	RQCache      map[string]v1.ResourceQuota
	ConfManager  *config.MutexConfigManager
	UpdatedCache map[string]m.RqSchema
	Logger       *log.Logger
}

func NewRQWatcher(client *kubernetes.Clientset, cm *config.MutexConfigManager, cache *map[string]v1.ResourceQuota, l *log.Logger) *RQWatcher {
	epw := RQWatcher{Client: client, RQCache: *cache, ConfManager: cm, UpdatedCache: make(map[string]m.RqSchema), Logger: l}
	return &epw
}

func (pw *RQWatcher) qualifies(p *v1.ResourceQuota) bool {
	return (len((*pw.ConfManager).Get().NsToMonitor) == 0 ||
		utils.StringInSlice(p.Namespace, (*pw.ConfManager).Get().NsToMonitor)) &&
		!utils.StringInSlice(p.Namespace, (*pw.ConfManager).Get().NsToMonitorExclude)
}

func (pw *RQWatcher) startEventQueueWorker(stopCh <-chan struct{}) {
	bag := (*pw.ConfManager).Get()
	pw.eventQueueTicker(stopCh, time.NewTicker(time.Duration(bag.SnapshotSyncInterval)*time.Second))
}

func (pw *RQWatcher) eventQueueTicker(stop <-chan struct{}, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			pw.postRQRecords()
		case <-stop:
			ticker.Stop()
			return
		}
	}
}

//quotas
func (pw RQWatcher) WatchResourceQuotas() {
	api := pw.Client.CoreV1()
	listOptions := metav1.ListOptions{}
	pw.Logger.Info("Starting Quota Watcher...")

	stop := make(chan struct{})
	go pw.startEventQueueWorker(stop)

	watcher, err := api.ResourceQuotas(metav1.NamespaceAll).Watch(listOptions)
	if err != nil {
		pw.Logger.WithField("error", err).Error("Issues when setting up Quota watcher. Aborting...")
	} else {

		ch := watcher.ResultChan()

		for ev := range ch {
			rq, ok := ev.Object.(*v1.ResourceQuota)
			if !ok {
				pw.Logger.Warn("Expected ResourceQuota, but received an object of an unknown type. ")
				continue
			}
			switch ev.Type {
			case watch.Added:
				pw.onNewResourceQuota(rq)
				break

			case watch.Deleted:
				pw.onDeleteResourceQuota(rq)
				break

			case watch.Modified:
				pw.onUpdateResourceQuota(rq)
				break
			}

		}
	}
	pw.Logger.Info("Exiting quota watcher.")
}

func (pw RQWatcher) onNewResourceQuota(rq *v1.ResourceQuota) {
	if !pw.qualifies(rq) {
		return
	}
	pw.updateMap(rq)
}

func (pw RQWatcher) onDeleteResourceQuota(rq *v1.ResourceQuota) {
	if !pw.qualifies(rq) {
		return
	}
	key := utils.GetKey(rq.Namespace, rq.Name)
	_, ok := pw.RQCache[key]
	if ok {
		lockRQ.Lock()
		defer lockRQ.Unlock()
		delete(pw.RQCache, key)
		delete(pw.UpdatedCache, key)
	}
}

func (pw RQWatcher) onUpdateResourceQuota(rq *v1.ResourceQuota) {
	if !pw.qualifies(rq) {
		return
	}
	pw.updateMap(rq)
}

func (pw RQWatcher) updateMap(rq *v1.ResourceQuota) {
	lockRQ.Lock()
	defer lockRQ.Unlock()
	key := utils.GetKey(rq.Namespace, rq.Name)
	pw.RQCache[key] = *rq
	schema := m.NewRQSchema(rq)
	pw.UpdatedCache[key] = schema
}

func (pw RQWatcher) CloneMap() map[string]v1.ResourceQuota {
	lockRQ.RLock()
	defer lockRQ.RUnlock()
	m := make(map[string]v1.ResourceQuota)
	for key, val := range pw.RQCache {
		m[key] = val
	}

	return m
}

func (pw *RQWatcher) postRQRecords() {
	if len(pw.UpdatedCache) == 0 {
		return
	}

	pw.Logger.WithField("count", len(pw.UpdatedCache)).Info("About to send Quota records")
	bag := (*pw.ConfManager).Get()
	count := 0

	objList := []m.RqSchema{}
	for _, schema := range pw.UpdatedCache {
		objList = append(objList, schema)
		if count == bag.EventAPILimit {
			pw.postRQBatchRecords(&objList)
			count = 0
			objList = objList[:0]
		}
		count++
	}

	if count > 0 {
		pw.postRQBatchRecords(&objList)
	}
}

func (pw *RQWatcher) postRQBatchRecords(objList *[]m.RqSchema) {
	bag := (*pw.ConfManager).Get()
	rc := app.NewRestClient(bag, pw.Logger)

	schemaDefObj := m.NewRqSchemaDefWrapper()

	err := rc.EnsureSchema(bag.RqSchemaName, &schemaDefObj)
	if err != nil {
		pw.Logger.WithFields(log.Fields{"name": bag.RqSchemaName, "error": err}).Error("Issues when ensuring %s schema.")
	} else {
		data, err := json.Marshal(objList)
		if err != nil {
			pw.Logger.WithFields(log.Fields{"error": err}).Error("Problems when serializing array of resource quota schemas")
		}
		rc.PostAppDEvents(bag.RqSchemaName, data)
		pw.UpdatedCache = make(map[string]m.RqSchema)
	}

}
