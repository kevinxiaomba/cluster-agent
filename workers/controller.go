package workers

import (
	"encoding/json"
	"fmt"

	//	"log"
	//	"os"
	"sync"

	//	app "github.com/sjeltuhin/clusterAgent/appd"
	m "github.com/sjeltuhin/clusterAgent/models"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type MainController struct {
	Bag       *m.AppDBag
	K8sClient *kubernetes.Clientset
}

func NewController(bag *m.AppDBag, client *kubernetes.Clientset) MainController {
	return MainController{bag, client}
}

func (c *MainController) Run(stopCh <-chan struct{}, wg *sync.WaitGroup) {

	//	logger := log.New(os.Stdout, "[APPD_CLUSTER_MONITOR]", log.Lshortfile)
	//	appdController := app.NewControllerClient(c.Bag, logger)
	//	wg.Add(1)
	//	nsDone := make(chan *v1.NamespaceList)
	//	go nsWorker(nsDone, c.K8sClient, wg)

	//	wg.Add(1)
	//	nodesDone := make(chan *v1.NodeList)
	//	go nodesWorker(nodesDone, c.K8sClient, wg)

	//	wg.Add(1)
	//	metricsDoneNodes := make(chan *m.NodeMetricsObjList)
	//	go metricsWorkerNodes(metricsDoneNodes, c.K8sClient, wg)

	wg.Add(1)
	//	eventsDone := make(chan *m.AppDMetricList)
	go c.eventsWorker(stopCh, c.K8sClient, wg)

	wg.Add(1)
	go c.podsWorker(stopCh, c.K8sClient, wg)
	//	podsDone := make(chan *m.AppDMetricList)
	//	go c.podsWorker(stopCh, c.K8sClient, wg)

	//	nsList := <-nsDone
	//	recordNamespaces(nsList)

	//	nodesList := <-nodesDone
	//	recordNodes(nodesList)

	//	podMetricsList := <-podsDone
	//	recordMetrics("Pods", podMetricsList)
	//	appdController.PostMetrics(*podMetricsList)

	//	metricsDataNodes := <-metricsDoneNodes
	//	metricsDataNodes.PrintNodeList()

	//	eventMetrics := <-eventsDone
	//	recordMetrics("Events", eventMetrics)

	<-stopCh
}

func recordNamespaces(nsList *v1.NamespaceList) {
	fmt.Println("Namespaces:")
	template := "%s\n"
	for _, ns := range nsList.Items {
		fmt.Printf(template, ns.Name)
	}
}

func recordNodes(nodeList *v1.NodeList) {
	fmt.Println("Nodes:")
	template := "%s\n"
	for _, pd := range nodeList.Items {
		fmt.Printf(template, pd.Name)
	}
}

func recordMetrics(entity string, metrics *m.AppDMetricList) {
	fmt.Printf("%s:\n", entity)
	template := "%s\n"
	for _, pd := range metrics.Items {
		fmt.Printf(template, pd.MetricName)
	}
}

func nsWorker(finished chan *v1.NamespaceList, client *kubernetes.Clientset, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Println("Namepace Worker: Started")
	api := client.CoreV1()
	ns, err := api.Namespaces().List(metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Issues getting namespaces %s\n", err)
	}
	fmt.Println("Namepace Worker: Finished")
	finished <- ns
}

func (c *MainController) podsWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup) {
	fmt.Println("Starting Pods worker")
	defer wg.Done()
	pw := NewPodWorker(client, c.Bag)
	pw.Observe(stopCh, wg)
	<-stopCh
}

func (c *MainController) eventsWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup) {
	fmt.Println("Starting events worker")
	defer wg.Done()
	ew := NewEventWorker(client, c.Bag.AppName)
	ew.Observe(stopCh, wg)
}

func nodesWorker(finished chan *v1.NodeList, client *kubernetes.Clientset, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Println("Nodes Worker: Started")
	api := client.CoreV1()
	nodes, err := api.Nodes().List(metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Issues getting Nodes %s\n", err)
	}
	fmt.Println("Nodes Worker: Finished")
	finished <- nodes
}

func metricsWorkerNodes(finished chan *m.NodeMetricsObjList, client *kubernetes.Clientset, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Println("Metrics Worker  Nodes: Started")

	data, err := client.RESTClient().Get().AbsPath("apis/metrics.k8s.io/v1beta1/nodes").DoRaw()
	if err != nil {
		fmt.Printf("Issues when requesting metrics from metrics server %s\n", err.Error())
	}

	var list m.NodeMetricsObjList
	merde := json.Unmarshal(data, &list)
	if merde != nil {
		fmt.Printf("Unmarshal issues. %v\n", merde)
	}

	fmt.Println("Metrics Worker Nodes: Finished")
	finished <- &list
}
