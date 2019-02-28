package workers

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sjeltuhin/clusterAgent/config"
	m "github.com/sjeltuhin/clusterAgent/models"
	"github.com/sjeltuhin/clusterAgent/web"
	"k8s.io/api/core/v1"

	app "github.com/sjeltuhin/clusterAgent/appd"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	AGENT_EVENT_SECRET string = "appd-event-secret"
	AGENT_EVENT_KEY    string = "apikey"
)

type MainController struct {
	ConfManager *config.MutexConfigManager
	K8sClient   *kubernetes.Clientset
	Logger      *log.Logger
	K8sConfig   *rest.Config
	PodsWorker  *PodWorker
}

func NewController(cm *config.MutexConfigManager, client *kubernetes.Clientset, l *log.Logger, config *rest.Config) MainController {
	return MainController{ConfManager: cm, K8sClient: client, Logger: l, K8sConfig: config}
}

func (c *MainController) ValidateParameters() error {
	bag := c.ConfManager.Get()
	//validate controller URL
	if bag.EventServiceUrl == "" || strings.Contains(bag.ControllerUrl, "http") {
		arr := strings.Split(bag.ControllerUrl, ":")
		if len(arr) != 3 {
			return fmt.Errorf("Controller Url is invalid. Use this format: protocol://url:port")
		}
		protocol := arr[0]
		controllerUrl := strings.TrimLeft(arr[1], "//")
		port, errPort := strconv.Atoi(arr[2])
		if errPort != nil {
			return fmt.Errorf("Controller port is invalid. %v", errPort)
		}
		bag.ControllerUrl = controllerUrl
		bag.ControllerPort = uint16(port)
		bag.SSLEnabled = strings.Contains(protocol, "s")
	}

	//events API url
	if bag.EventServiceUrl == "" {
		if strings.Contains(bag.ControllerUrl, "appdynamics.com") {
			//saas
			bag.EventServiceUrl = "https://analytics.api.appdynamics.com"
		} else {
			protocol := "http"
			if bag.SSLEnabled {
				protocol = "https"
			}
			bag.EventServiceUrl = fmt.Sprintf("%s://%s:9080", protocol, bag.ControllerUrl)
		}
	}
	c.Logger.Printf("Controller URL: %s, Controller port: %d, Event URL: %s", bag.ControllerUrl, bag.ControllerPort, bag.EventServiceUrl)
	//validate keys
	if bag.RestAPICred == "" {
		return fmt.Errorf("Rest API user account is required. Create an account and pass it to the cluster agent in this form <user>@<account>:<pass>")
	}
	if bag.AccessKey == "" || bag.EventKey == "" {

		path := "restui/user/account"

		c.Logger.Printf("Loading account info... \n")

		rc := app.NewRestClient(bag, c.Logger)
		data, err := rc.CallAppDController(path, "GET", nil)
		if err != nil {
			return fmt.Errorf("Unable to get the AppDynamics account information. %v", err)
		}
		var accountObj map[string]interface{}
		errJson := json.Unmarshal(data, &accountObj)
		if errJson != nil {
			return fmt.Errorf("Unable to deserialize AppDynamics account object. %v", errJson)
		}
		for k, v := range accountObj {
			if k == "account" {
				obj := v.(map[string]interface{})
				bag.AccessKey = obj["accessKey"].(string)
				bag.Account = obj["name"].(string)
				bag.GlobalAccount = obj["globalAccountName"].(string)
				break
			}
		}
	}
	if bag.EventKey == "" {
		c.Logger.Printf("Event API key not specified. Trying to obtain an existing key...\n")
		key, e := c.EnsureEventAPIKey(bag)
		if e != nil {
			return fmt.Errorf("Unable to generate key for AppDynamics Event API. %v", e)
		}
		bag.EventKey = key
	}
	c.ConfManager.Set(bag)
	c.Logger.Printf("Account info: %s %s\n", bag.AccessKey, bag.GlobalAccount)
	return nil
}

func (c *MainController) Run(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	bag := c.ConfManager.Get()
	ws := web.NewAgentWebServer(bag)
	wg.Add(1)
	go ws.RunServer()

	appdController := app.NewControllerClient(c.ConfManager, c.Logger)

	wg.Add(1)
	go c.startNodeWorker(stopCh, c.K8sClient, wg, appdController)

	wg.Add(2)
	go c.startPodsWorker(stopCh, c.K8sClient, wg, appdController)

	wg.Add(1)
	go c.startDeployWorker(stopCh, c.K8sClient, wg, appdController)

	//	<-stopCh
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
	pw := NewPodWorker(client, c.ConfManager, appdController, c.K8sConfig, c.Logger)
	c.PodsWorker = &pw
	go c.startEventsWorker(stopCh, c.K8sClient, wg, appdController)
	c.PodsWorker.Observe(stopCh, wg)
	<-stopCh
}

func (c *MainController) startDeployWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup, appdController *app.ControllerClient) {
	fmt.Println("Starting Deployment worker")
	defer wg.Done()
	pw := NewDeployWorker(client, c.ConfManager, appdController)
	pw.Observe(stopCh, wg)
	<-stopCh
}

func (c *MainController) startEventsWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup, appdController *app.ControllerClient) {
	fmt.Println("Starting events worker")
	defer wg.Done()
	ew := NewEventWorker(client, c.ConfManager, appdController, c.PodsWorker)
	ew.Observe(stopCh, wg)
}

func (c *MainController) startNodeWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup, appdController *app.ControllerClient) {
	fmt.Println("Starting nodes worker")
	defer wg.Done()
	ew := NewNodesWorker(client, c.ConfManager, appdController)
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

func (c *MainController) EnsureEventAPIKey(bag *m.AppDBag) (string, error) {
	key := ""
	//check if the key is already in the known secret
	api := c.K8sClient.CoreV1()
	listOptions := metav1.ListOptions{}

	secrets, err := api.Secrets(bag.AgentNamespace).List(listOptions)
	if err != nil {
		c.Logger.Printf("WARNING. Unable to load secrets in namespace %s. Proceeding to generate new key. %v", bag.AgentNamespace, err)
	} else {
		for _, s := range secrets.Items {
			if s.Name == AGENT_EVENT_SECRET {
				keyData := s.Data[AGENT_EVENT_KEY]
				key = string(keyData)
				c.Logger.Printf("Saved secret data: %s\n", key)
				break
			}
		}
	}
	if key != "" {
		//sanity check if the key still exists and enabled
		p := "restui/analyticsApiKeyGen/listApiKeys"
		rc := app.NewRestClient(bag, c.Logger)
		found := false
		data, err := rc.CallAppDController(p, "GET", nil)
		if err != nil {
			c.Logger.Printf("WARNING. Unable to load list of API keys from AppD Analytics. Proceeding to generate new key. %v", err)
		} else {
			var list []map[string]interface{}
			eList := json.Unmarshal(data, &list)
			if eList != nil {
				c.Logger.Printf("WARNING. Unable to deserialize the list of API keys from AppD Analytics. Proceeding to generate new key. %v", err)
			} else {
				last4 := key[len(key)-4:]
				c.Logger.Printf("Last 4 digits of the key: %s", last4)
				for _, obj := range list {
					c.Logger.Printf("Suffix of the key: %s", obj["suffix"])
					if obj["enabled"] == true &&
						obj["suffix"] == last4 {
						found = true
						break
					}

				}
			}
		}
		if !found {
			key = ""
		}
	}

	if key == "" {
		keyName := fmt.Sprintf("%s-%d", bag.AppName, time.Now().Unix())
		jsonStr := fmt.Sprintf(`{"enabled": "true", "eventAccessFilters": [], "name": "%s", "permissions": {"±CUSTOM_EVENTS±": ["MANAGE_SCHEMA", "QUERY", "PUBLISH"]}}`, keyName)
		body := []byte(jsonStr)
		path := "restui/analyticsApiKeyGen/create"

		c.Logger.Printf("Generating event API key... \n")

		rc := app.NewRestClient(bag, c.Logger)
		data, err := rc.CallAppDController(path, "POST", body)
		c.Logger.Printf("Call to generating event API key: %v %s \n", err, data)
		if err != nil {
			return key, fmt.Errorf("Unable to generate event API key. %v", err)
		}
		key = string(data)
		c.Logger.Printf("Generated event API key: %s", key)

		keyData := make(map[string]string)
		keyData[AGENT_EVENT_KEY] = key
		//save in a secret for future use
		secret := &v1.Secret{
			Type:       v1.SecretTypeOpaque,
			StringData: keyData,
			ObjectMeta: metav1.ObjectMeta{
				Name: AGENT_EVENT_SECRET,
				Labels: map[string]string{
					"owner": "clusteragent",
				},
			},
		}
		_, errCreate := api.Secrets(bag.AgentNamespace).Create(secret)
		if errCreate != nil {
			c.Logger.Printf("WARNING. Unable to save secret with event API key in namespace %s. %v", bag.AgentNamespace, errCreate)
		} else {
			c.Logger.Printf("Persisted event API key in secret: %s", AGENT_EVENT_KEY)
		}
	}
	return key, nil
}
