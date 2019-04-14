package workers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/appdynamics/cluster-agent/config"
	m "github.com/appdynamics/cluster-agent/models"
	"github.com/appdynamics/cluster-agent/utils"
	"github.com/appdynamics/cluster-agent/web"
	"k8s.io/api/core/v1"

	app "github.com/appdynamics/cluster-agent/appd"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	AGENT_EVENT_SECRET string = "appd-event-secret"
	AGENT_EVENT_KEY    string = "apikey"
)

type MainController struct {
	ConfManager    *config.MutexConfigManager
	K8sClient      *kubernetes.Clientset
	Logger         *log.Logger
	K8sConfig      *rest.Config
	PodsWorker     *PodWorker
	NodesWorker    *NodesWorker
	AppdController *app.ControllerClient
}

func NewController(cm *config.MutexConfigManager, client *kubernetes.Clientset, l *log.Logger, config *rest.Config) MainController {
	return MainController{ConfManager: cm, K8sClient: client, Logger: l, K8sConfig: config}
}

func (c *MainController) ValidateParameters() error {
	bag := c.ConfManager.Get()
	//validate controller URL
	if strings.Contains(bag.ControllerUrl, "http") {
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
	} else {
		return fmt.Errorf("Controller Url is invalid. Use this format: protocol://url:port")
	}

	//build rest api url
	restApiUrl := bag.ControllerUrl
	if bag.SSLEnabled {
		restApiUrl = fmt.Sprintf("https://%s/controller/", restApiUrl)
	} else {
		restApiUrl = fmt.Sprintf("http://%s:%d/controller/", restApiUrl, bag.ControllerPort)
	}
	bag.RestAPIUrl = restApiUrl

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
	if bag.AccessKey == "" {

		path := "restui/user/account"

		c.Logger.Info("Loading account info...")

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
	if bag.ProxyUrl != "" {
		arr := strings.Split(bag.ProxyUrl, ":")
		if len(arr) != 3 {
			return fmt.Errorf("ProxyUrl Url is invalid. Use this format: protocol://url:port")
		}
		bag.ProxyHost = strings.TrimLeft(arr[1], "//")
		bag.ProxyPort = arr[2]
	}
	if bag.AnalyticsAgentUrl != "" {
		protocol, host, port, err := utils.SplitUrl(bag.AnalyticsAgentUrl)
		if err != nil {
			return fmt.Errorf("Analytics agent Url is invalid. Use this format: protocol://url:port")
		}
		bag.RemoteBiqProtocol = protocol
		bag.RemoteBiqHost = host
		bag.RemoteBiqPort = port
	}
	c.ConfManager.Set(bag)
	c.Logger.WithFields(log.Fields{"accessKey": bag.AccessKey, "global account": bag.GlobalAccount}).Debug("Account info")
	appdC, errInitSdk := app.NewControllerClient(c.ConfManager, c.Logger)
	if errInitSdk != nil {
		return fmt.Errorf("Unable to initialize AppDynamics Golang SDK. Metrics collection will not be possible")
	}
	c.AppdController = appdC
	return nil
}

func (c *MainController) Run(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	bag := c.ConfManager.Get()
	ws := web.NewAgentWebServer(bag)
	wg.Add(1)
	go ws.RunServer()

	wg.Add(3)
	go c.startNodeWorker(stopCh, c.K8sClient, wg, c.AppdController)

	wg.Add(1)
	go c.startDeployWorker(stopCh, c.K8sClient, wg, c.AppdController)

	wg.Add(1)
	go c.startDaemonWorker(stopCh, c.K8sClient, wg, c.AppdController)

	wg.Add(1)
	go c.startRsWorker(stopCh, c.K8sClient, wg, c.AppdController)

	wg.Add(1)
	go c.startJobsWorker(stopCh, c.K8sClient, wg, c.AppdController)

	//	<-stopCh
}

func (c *MainController) startNodeWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup, appdController *app.ControllerClient) {
	c.Logger.Info("Starting Nodes worker...")
	defer wg.Done()
	nw := NewNodesWorker(client, c.ConfManager, appdController, c.Logger)
	c.NodesWorker = &nw
	go c.startPodsWorker(stopCh, client, wg, appdController)
	nw.Observe(stopCh, wg)
	<-stopCh

}

func (c *MainController) startDeployWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup, appdController *app.ControllerClient) {
	c.Logger.Info("Starting Deployment worker...")
	defer wg.Done()
	pw := NewDeployWorker(client, c.ConfManager, appdController, c.Logger)
	pw.Observe(stopCh, wg)
	<-stopCh
}

func (c *MainController) startDaemonWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup, appdController *app.ControllerClient) {
	c.Logger.Info("Starting Daemon worker...")
	defer wg.Done()
	pw := NewDaemonWorker(client, c.ConfManager, appdController, c.Logger)
	pw.Observe(stopCh, wg)
	<-stopCh
}

func (c *MainController) startRsWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup, appdController *app.ControllerClient) {
	c.Logger.Info("Starting ReplicaSet worker...")
	defer wg.Done()
	pw := NewRsWorker(client, c.ConfManager, appdController, c.Logger)
	pw.Observe(stopCh, wg)
	<-stopCh
}

func (c *MainController) startEventsWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup, appdController *app.ControllerClient) {
	c.Logger.Info("Starting Events worker...")
	defer wg.Done()
	ew := NewEventWorker(client, c.ConfManager, appdController, c.PodsWorker, c.Logger)
	ew.Observe(stopCh, wg)
	<-stopCh
}

func (c *MainController) startJobsWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup, appdController *app.ControllerClient) {
	c.Logger.Info("Starting Jobs worker...")
	defer wg.Done()
	ew := NewJobsWorker(client, c.ConfManager, appdController, c.K8sConfig, c.Logger)
	ew.Observe(stopCh, wg)
	<-stopCh
}

func (c *MainController) startPodsWorker(stopCh <-chan struct{}, client *kubernetes.Clientset, wg *sync.WaitGroup, appdController *app.ControllerClient) {
	c.Logger.Info("Starting Pods worker...")
	defer wg.Done()
	pw := NewPodWorker(client, c.ConfManager, appdController, c.K8sConfig, c.Logger, c.NodesWorker)
	c.PodsWorker = &pw
	go c.startEventsWorker(stopCh, c.K8sClient, wg, appdController)
	c.PodsWorker.Observe(stopCh, wg)
	<-stopCh
}

func (c *MainController) EnsureEventAPIKey(bag *m.AppDBag) (string, error) {
	key := ""
	//check if the key is already in the known secret
	api := c.K8sClient.CoreV1()
	listOptions := metav1.ListOptions{}

	secrets, err := api.Secrets(bag.AgentNamespace).List(listOptions)
	if err != nil {
		c.Logger.WithFields(log.Fields{"Namespace": bag.AgentNamespace, "Error": err}).
			Warn("Unable to load secrets in namespace. Proceeding to generate new key")
	} else {
		for _, s := range secrets.Items {
			if s.Name == AGENT_EVENT_SECRET {
				keyData := s.Data[AGENT_EVENT_KEY]
				key = string(keyData)
				c.Logger.Info("Saved secret data")
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
			c.Logger.WithField("Error", err).
				Warn("Unable to load list of API keys from AppD Analytics. Proceeding to generate new key")
		} else {
			var list []map[string]interface{}
			eList := json.Unmarshal(data, &list)
			if eList != nil {
				c.Logger.WithField("Error", eList).
					Warn("Unable to deserialize the list of API keys from AppD Analytics. Proceeding to generate new key")
			} else {
				last4 := key[len(key)-4:]
				c.Logger.WithField("key", last4).Debug("Last 4 digits of the key")
				for _, obj := range list {
					c.Logger.WithField("suffix", obj["suffix"]).Debug("Suffix of the key")
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

		c.Logger.Info("Generating event API key...")

		rc := app.NewRestClient(bag, c.Logger)
		data, err := rc.CallAppDController(path, "POST", body)
		if err != nil {
			return key, fmt.Errorf("Unable to generate event API key. %v", err)
		}
		key = string(data)
		c.Logger.WithField("key", key).Debug("Generated event API key")

		keyData := make(map[string]string)
		keyData[AGENT_EVENT_KEY] = key
		//save in a secret for future use
		secret := &v1.Secret{
			Type:       v1.SecretTypeOpaque,
			StringData: keyData,
			ObjectMeta: metav1.ObjectMeta{
				Name: AGENT_EVENT_SECRET,
				Labels: map[string]string{
					"owner": "cluster-agent",
				},
			},
		}
		_, errCreate := api.Secrets(bag.AgentNamespace).Create(secret)
		if errCreate != nil {
			c.Logger.WithFields(log.Fields{"namespace": bag.AgentNamespace, "error": errCreate}).
				Warn("Unable to save secret with event API key in namespace")
		} else {
			c.Logger.WithField("secret key", AGENT_EVENT_KEY).Info("Persisted event API key in secret")
		}
	}
	return key, nil
}
