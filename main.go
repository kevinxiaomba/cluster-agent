package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"path/filepath"
	"sync"

	m "github.com/sjeltuhin/clusterAgent/models"
	w "github.com/sjeltuhin/clusterAgent/workers"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Flags struct {
	Kubeconfig string
	Bag        m.AppDBag
}

func buildParams() Flags {
	params := Flags{}

	flag.StringVar(&params.Kubeconfig, "kubeconfig", getKubeConfigPath(), "(optional) absolute path to the kubeconfig file")
	flag.StringVar(&params.Bag.Account, "account-name", getAccountName(), "Account name")
	flag.StringVar(&params.Bag.GlobalAccount, "global-account-name", getGLobalAccountName(), "Global Account name")
	flag.StringVar(&params.Bag.AppName, "app-name", getAppName(), "Application name")
	flag.StringVar(&params.Bag.TierName, "tier-name", getTierName(), "Tier name")
	flag.StringVar(&params.Bag.NodeName, "node-name", getNodeName(), "Node name")
	flag.StringVar(&params.Bag.ControllerUrl, "controller-dns", getControllerUrl(), "Controller DNS")
	flag.StringVar(&params.Bag.EventServiceUrl, "events-url", getEventServiceURL(), "Event API service URL")
	flag.StringVar(&params.Bag.SystemSSLCert, "system-ssl", getSystemSSL(), "System SSL Certificate File")
	flag.StringVar(&params.Bag.AgentSSLCert, "agent-ssl", getAgentSSL(), "Agent SSL Certificate File")
	flag.StringVar(&params.Bag.AccessKey, "access-key", getAccessKey(), "AppD Controller Access Key")
	flag.StringVar(&params.Bag.EventKey, "event-key", getEventKey(), "Event API Key")
	flag.StringVar(&params.Bag.RestAPICred, "rest-api-creds", getRestAPICred(), "Rest API Credentials")
	flag.BoolVar(&params.Bag.SSLEnabled, "use-ssl", false, "Controller uses SSL connection")
	flag.StringVar(&params.Bag.PodSchemaName, "schema-pods", "k8s_pod_snapshots", "Pod schema name")
	flag.IntVar(&params.Bag.EventAPILimit, "event-batch-size", 100, "Max number of AppD events record to send in a batch")
	var tempPort uint
	flag.UintVar(&tempPort, "controller-port", getControllerPort(), "Controller Port")
	params.Bag.ControllerPort = uint16(tempPort)

	flag.Parse()

	return params
}

func main() {
	// Set logging output to standard console out
	log.SetOutput(os.Stdout)

	sigs := make(chan os.Signal, 1) // Create channel to receive OS signals
	stop := make(chan struct{})     // Create channel to receive stop signal

	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT) // Register the sigs channel to receieve SIGTERM

	params := buildParams()

	fmt.Printf("Parameters:\n %s and NodeName = %s", params, params.Bag.NodeName)
	config, err := authFromConfig(&params)
	if err != nil {
		log.Printf("Issues getting kube config. %s. Terminating...", err.Error())
		return
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("Issues authenticating with the cluster. %s. Terminating...", err.Error())
		return
	}

	var wg sync.WaitGroup
	controller := w.NewController(&params.Bag, clientset)
	controller.Run(stop, &wg)

	<-sigs

	fmt.Println("Shutting down...")

	close(stop)

	wg.Wait()

}

func authFromConfig(params *Flags) (*rest.Config, error) {
	if params.Kubeconfig != "" {
		fmt.Printf("Kube config = %s", params.Kubeconfig)
		return clientcmd.BuildConfigFromFlags("", params.Kubeconfig)
	} else {
		return rest.InClusterConfig()
	}

}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func getKubeConfigPath() string {
	//try env var first
	var path string = os.Getenv("KUBE_CONFIG_PATH")
	if path == "" {
		//try the default location
		if home := homeDir(); home != "" {
			path = filepath.Join(home, ".kube", "config")
		}
	}
	return path
}

func getAccountName() string {
	return os.Getenv("ACCOUNT_NAME")
}

func getGLobalAccountName() string {
	return os.Getenv("GLOBAL_ACCOUNT_NAME")
}

func getAppName() string {
	return os.Getenv("APPLICATION_NAME")
}

func getTierName() string {
	return os.Getenv("TIER_NAME")
}

func getNodeName() string {
	return os.Getenv("NODE_NAME")
}

func getControllerUrl() string {
	return os.Getenv("CONTROLLER_URL")
}

func getAccessKey() string {
	return os.Getenv("ACCESS_KEY")
}

func getRestAPICred() string {
	return os.Getenv("REST_API_CREDENTIALS")
}

func getEventServiceURL() string {
	return os.Getenv("EVENTS_API_URL")
}

func getEventKey() string {
	return os.Getenv("EVENT_ACCESS_KEY")
}

func getSystemSSL() string {
	return os.Getenv("SYSTEM_SSL")
}

func getAgentSSL() string {
	return os.Getenv("AGENT_SSL")
}

func getControllerPort() uint {
	port := os.Getenv("CONTROLLER_PORT")
	val, err := strconv.Atoi(port)
	if err != nil {
		fmt.Printf("Cannot convert port value %s to int\n", port)
		return 8090
	}
	return uint(val)
}
