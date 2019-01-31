package workers

import (
	"fmt"
	"sync"

	app "github.com/sjeltuhin/clusterAgent/appd"
	m "github.com/sjeltuhin/clusterAgent/models"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type ServicesWorker struct {
	informer       cache.SharedIndexInformer
	Client         *kubernetes.Clientset
	K8sConfig      *rest.Config
	Bag            *m.AppDBag
	SummaryMap     map[string]m.ClusterPodMetrics
	WQ             workqueue.RateLimitingInterface
	AppdController *app.ControllerClient
}

func NewServicesWorker(client *kubernetes.Clientset, bag *m.AppDBag, controller *app.ControllerClient, config *rest.Config) ServicesWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	sw := ServicesWorker{Client: client, Bag: bag, SummaryMap: make(map[string]m.ClusterPodMetrics), WQ: queue,
		AppdController: controller, K8sConfig: config}
	sw.initServicesInformer(client)
	return sw
}

func (sw *ServicesWorker) initServicesInformer(client *kubernetes.Clientset) cache.SharedIndexInformer {
	i := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.Core().Services(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.Core().Services(metav1.NamespaceAll).Watch(options)
			},
		},
		&v1.Service{},
		0,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	i.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    sw.onNewService,
		DeleteFunc: sw.onDeleteService,
		UpdateFunc: sw.onUpdateService,
	})
	sw.informer = i

	return i
}

func (sw *ServicesWorker) Observe(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	fmt.Println("Starting services worker...")
	defer wg.Done()
	defer sw.WQ.ShutDown()
	wg.Add(1)
	go sw.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, sw.HasSynced) {
		fmt.Errorf("Timed out waiting for caches to sync")
	}
	fmt.Println("Cache syncronized. Starting the processing...")

	<-stopCh
}

func (sw *ServicesWorker) HasSynced() bool {
	return sw.informer.HasSynced()
}

func (sw *ServicesWorker) onNewService(obj interface{}) {
	svc := obj.(*v1.Service)
	fmt.Printf("Added Service: %s\n", svc.Name)
}

func (nw *ServicesWorker) onDeleteService(obj interface{}) {
	svc := obj.(*v1.Service)
	fmt.Printf("Deleted Service: %s\n", svc.Name)
}

func (nw *ServicesWorker) onUpdateService(objOld interface{}, objNew interface{}) {
	svc := objOld.(*v1.Service)
	fmt.Printf("Updated Service: %s\n", svc.Name)

}
