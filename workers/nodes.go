package workers

import (
	"encoding/json"
	"fmt"

	m "github.com/sjeltuhin/clusterAgent/models"

	app "github.com/sjeltuhin/clusterAgent/appd"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type NodesWorker struct {
	informer       cache.SharedIndexInformer
	Client         *kubernetes.Clientset
	Bag            *m.AppDBag
	SummaryMap     map[string]m.ClusterPodMetrics
	WQ             workqueue.RateLimitingInterface
	AppdController *app.ControllerClient
}

func NewNodesWorker(client *kubernetes.Clientset, bag *m.AppDBag, controller *app.ControllerClient) PodWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	pw := PodWorker{Client: client, Bag: bag, SummaryMap: make(map[string]m.ClusterPodMetrics), WQ: queue, AppdController: controller}
	pw.initPodInformer(client)
	return pw
}

func (nw *NodesWorker) initPodInformer(client *kubernetes.Clientset) cache.SharedIndexInformer {
	i := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.Core().Nodes().List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.Core().Nodes().Watch(options)
			},
		},
		&v1.Node{},
		0,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	i.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    nw.onNewNode,
		DeleteFunc: nw.onDeleteNode,
		UpdateFunc: nw.onUpdateNode,
	})
	nw.informer = i

	return i
}

func (nw *NodesWorker) onNewNode(obj interface{}) {
	nodeObj := obj.(*v1.Node)
	fmt.Printf("Added Node: %s\n", nodeObj.Name)

}

func (nw *NodesWorker) onDeleteNode(obj interface{}) {
	nodeObj := obj.(*v1.Node)
	fmt.Printf("Deleted Node: %s\n", nodeObj.Name)
}

func (nw *NodesWorker) onUpdateNode(objOld interface{}, objNew interface{}) {
	nodeObj := objOld.(*v1.Node)
	fmt.Printf("Updated Node: %s\n", nodeObj.Name)

}

func metricsWorkerNodes(finished chan *m.NodeMetricsObjList, client *kubernetes.Clientset) {
	fmt.Println("Metrics Worker Pods: Started")
	var path string = "apis/metrics.k8s.io/v1beta1/nodes"

	data, err := client.RESTClient().Get().AbsPath(path).DoRaw()
	if err != nil {
		fmt.Printf("Issues when requesting metrics from metrics with path %s from server %s\n", path, err.Error())
	}

	var list m.NodeMetricsObjList
	merde := json.Unmarshal(data, &list)
	if merde != nil {
		fmt.Printf("Unmarshal issues. %v\n", merde)
	}

	fmt.Println("Metrics Worker Pods: Finished. Metrics records: %d", len(list.Items))
	fmt.Println(&list)
	finished <- &list
}

func metricsWorkerSingleNode(finished chan *m.NodeMetricsObj, client *kubernetes.Clientset, nodeName string) {
	var path string = ""
	var metricsObj m.NodeMetricsObj
	if nodeName != "" {
		path = fmt.Sprintf("apis/metrics.k8s.io/v1beta1/nodes/%s", nodeName)

		data, err := client.RESTClient().Get().AbsPath(path).DoRaw()
		if err != nil {
			fmt.Printf("Issues when requesting metrics from metrics with path %s from server %s\n", path, err.Error())
		}

		merde := json.Unmarshal(data, &metricsObj)
		if merde != nil {
			fmt.Printf("Unmarshal issues. %v\n", merde)
		}

	}

	finished <- &metricsObj
}
