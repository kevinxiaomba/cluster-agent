package watchers

import (
	"fmt"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"github.com/sjeltuhin/clusterAgent/utils"
)

type RQWatcher struct {
	Client  *kubernetes.Clientset
	RQCache map[string]v1.ResourceQuota
}

func NewRQWatcher(client *kubernetes.Clientset) *RQWatcher {
	epw := RQWatcher{Client: client, RQCache: make(map[string]v1.ResourceQuota)}
	return &epw
}

//quotas
func (pw RQWatcher) WatchResourceQuotas() {
	api := pw.Client.CoreV1()
	listOptions := metav1.ListOptions{}
	fmt.Println("Starting Quota Watcher...")

	watcher, err := api.ResourceQuotas(metav1.NamespaceAll).Watch(listOptions)
	if err != nil {
		fmt.Printf("Issues when setting up resource quota watcher. %v", err)
	}

	ch := watcher.ResultChan()

	for ev := range ch {
		rq, ok := ev.Object.(*v1.ResourceQuota)
		if !ok {
			fmt.Printf("Expected ResourceQuota, but received an object of an unknown type. ")
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
	fmt.Println("Exiting quota watcher.")
}

func (pw RQWatcher) onNewResourceQuota(rq *v1.ResourceQuota) {
	pw.RQCache[utils.GetKey(rq.Namespace, rq.Name)] = *rq
}

func (pw RQWatcher) onDeleteResourceQuota(rq *v1.ResourceQuota) {
	key := utils.GetKey(rq.Namespace, rq.Name)
	_, ok := pw.RQCache[key]
	if ok {
		delete(pw.RQCache, key)
	}
}

func (pw RQWatcher) onUpdateResourceQuota(rq *v1.ResourceQuota) {
	pw.RQCache[utils.GetKey(rq.Namespace, rq.Name)] = *rq
}
