package watchers

import (
	"fmt"
	"sync"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"github.com/sjeltuhin/clusterAgent/utils"
)

type PVCWatcher struct {
	Client   *kubernetes.Clientset
	PVCCache map[string]v1.PersistentVolumeClaim
}

var lockPVC = sync.RWMutex{}

func NewPVCWatcher(client *kubernetes.Clientset) *PVCWatcher {
	epw := PVCWatcher{Client: client, PVCCache: make(map[string]v1.PersistentVolumeClaim)}
	return &epw
}

//PVCs
func (pw PVCWatcher) WatchPVC() {
	api := pw.Client.CoreV1()
	listOptions := metav1.ListOptions{}
	fmt.Println("Starting Persistent Volume Claim Watcher...")

	watcher, err := api.PersistentVolumeClaims(metav1.NamespaceAll).Watch(listOptions)
	if err != nil {
		fmt.Printf("Issues when setting up PVC watcher. %v", err)
	}

	ch := watcher.ResultChan()

	for ev := range ch {
		pvc, ok := ev.Object.(*v1.PersistentVolumeClaim)
		//		quant := pvc.Spec.Resources.Requests[v1.ResourceStorage]
		if !ok {
			fmt.Printf("Expected PVC, but received an object of an unknown type. ")
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
	fmt.Println("Exiting PVC watcher.")
}

func (pw PVCWatcher) onNewPVC(pvc *v1.PersistentVolumeClaim) {
	fmt.Printf("Added PVC: %s\n", pvc.Name)
	pw.updateMap(pvc)
}

func (pw PVCWatcher) onDeletePVC(pvc *v1.PersistentVolumeClaim) {
	_, ok := pw.PVCCache[utils.GetKey(pvc.Namespace, pvc.Name)]
	if ok {
		lockPVC.Lock()
		defer lockPVC.Unlock()
		delete(pw.PVCCache, utils.GetKey(pvc.Namespace, pvc.Name))
		fmt.Printf("PVC %s deleted \n", pvc.Name)
	}
}

func (pw PVCWatcher) onUpdatePVC(pvc *v1.PersistentVolumeClaim) {
	pw.updateMap(pvc)
	fmt.Printf("PVC updated: %s\n", pvc.Name)

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
		pw.PVCCache[key] = val
	}

	return m
}
