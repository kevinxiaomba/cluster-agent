package workers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	app "github.com/sjeltuhin/clusterAgent/appd"
	m "github.com/sjeltuhin/clusterAgent/models"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type MainController struct {
	Bag       *m.AppDBag
	K8sClient *kubernetes.Clientset
	Logger    *log.Logger
}

func NewController(bag *m.AppDBag, client *kubernetes.Clientset, l *log.Logger) MainController {
	return MainController{bag, client, l}
}

func (c *MainController) Run(stopCh <-chan struct{}, wg *sync.WaitGroup) {

	appdController := app.NewControllerClient(c.Bag, c.Logger)

	wg.Add(1)
	go c.eventsWorker(stopCh, c.K8sClient, wg)

	wg.Add(1)
	go c.podsWorker(stopCh, c.K8sClient, wg, appdController)

	wg.Add(1)
	go c.startDashboardWorker(stopCh)

	//	<-stopCh
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

func (c *MainController) startDashboardWorker(stopCh <-chan struct{}) {
	c.dashboardTicker(stopCh, time.NewTicker(1*time.Minute))
}

func (c *MainController) dashboardTicker(stop <-chan struct{}, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			c.ensureDashboard()
		case <-stop:
			ticker.Stop()
			return
		}
	}
}

func (c *MainController) ensureDashboard() {
	fmt.Println("Checking the dashboard")
	rc := app.NewRestClient(c.Bag, c.Logger)
	data, err := rc.CallAppDController("restui/dashboards/getAllDashboardsByType/false", "GET", nil)
	if err != nil {
		fmt.Printf("Unable to get the list of dashaboard. %v\n", err)
		return
	}
	var list []m.Dashboard
	e := json.Unmarshal(data, &list)
	if e != nil {
		fmt.Printf("Unable to deserialize the list of dashaboards. %v\n", e)
		return
	}
	dashName := fmt.Sprintf("%s-%s-%s", c.Bag.AppName, c.Bag.TierName, c.Bag.DashboardSuffix)
	exists := false
	for _, d := range list {
		if d.Name == dashName {
			exists = true
			fmt.Printf("Dashaboard %s exists. No action required\n", dashName)
			break
		}
	}
	if !exists {
		fmt.Printf("Dashaboard %s does not exist. Creating...\n", dashName)
		c.createDashboard(dashName)
	}
}

func (c *MainController) createDashboard(dashName string) {

	jsonFile, err := os.Open(c.Bag.DashboardTemplatePath)
	// if we os.Open returns an error then handle it
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Successfully Opened Dashboard template")
	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)

	var result map[string]interface{}
	json.Unmarshal([]byte(byteValue), &result)

	result["name"] = dashName

	generated, errMar := json.Marshal(result)
	if errMar != nil {
		fmt.Println(errMar)
		return
	}
	dir := filepath.Dir(c.Bag.DashboardTemplatePath)
	fileForUpload := dir + "/GENERATED.json"
	fmt.Printf("About to upload file %s...\n", fileForUpload)
	errSave := ioutil.WriteFile(fileForUpload, generated, 0644)
	if errSave != nil {
		fmt.Printf("Issues when writing generated file. %v\n", errSave)
	}
	rc := app.NewRestClient(c.Bag, c.Logger)
	rc.CreateDashboard(fileForUpload)

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

func (c *MainController) podsWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup, appdController *app.ControllerClient) {
	fmt.Println("Starting Pods worker")
	defer wg.Done()
	pw := NewPodWorker(client, c.Bag, appdController)
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
