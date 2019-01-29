package workers

import (
	"fmt"
	"log"
	"sync"

	app "github.com/sjeltuhin/clusterAgent/appd"
	m "github.com/sjeltuhin/clusterAgent/models"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type MainController struct {
	Bag       *m.AppDBag
	K8sClient *kubernetes.Clientset
	Logger    *log.Logger
	K8sConfig *rest.Config
}

func NewController(bag *m.AppDBag, client *kubernetes.Clientset, l *log.Logger, config *rest.Config) MainController {
	return MainController{bag, client, l, config}
}

func (c *MainController) Run(stopCh <-chan struct{}, wg *sync.WaitGroup) {

	appdController := app.NewControllerClient(c.Bag, c.Logger)

	//	wg.Add(1)
	//	go c.startEventsWorker(stopCh, c.K8sClient, wg)

	wg.Add(1)
	go c.startDeployWorker(stopCh, c.K8sClient, wg, appdController)

	wg.Add(1)
	go c.startPodsWorker(stopCh, c.K8sClient, wg, appdController)

	//	wg.Add(1)
	//	go c.startDashboardWorker(stopCh)

	//	<-stopCh
}

func (c *MainController) startDashboardWorker(stopCh <-chan struct{}) {
	dw := NewDashboardWorker(c.Bag, c.Logger)
	dw.Run(stopCh)
	<-stopCh
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

func (c *MainController) startPodsWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup, appdController *app.ControllerClient) {
	fmt.Println("Starting Pods worker")
	defer wg.Done()
	pw := NewPodWorker(client, c.Bag, appdController, c.K8sConfig)
	pw.Observe(stopCh, wg)
	<-stopCh
}

func (c *MainController) startDeployWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup, appdController *app.ControllerClient) {
	fmt.Println("Starting Deployment worker")
	defer wg.Done()
	pw := NewDeployWorker(client, c.Bag, appdController)
	pw.Observe(stopCh, wg)
	<-stopCh
}

func (c *MainController) startEventsWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup) {
	fmt.Println("Starting events worker")
	defer wg.Done()
	ew := NewEventWorker(client, c.Bag)
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
