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
	flag.StringVar(&params.Bag.AppName, "app-name", getAppName(), "Application name")
	flag.StringVar(&params.Bag.TierName, "tier-name", getTierName(), "Tier name")
	flag.StringVar(&params.Bag.NodeName, "node-name", getNodeName(), "Node name")
	flag.StringVar(&params.Bag.ControllerUrl, "controller-dns", getControllerUrl(), "Controller DNS")
	flag.StringVar(&params.Bag.SystemSSLCert, "system-ssl", getSystemSSL(), "System SSL Certificate File")
	flag.StringVar(&params.Bag.AgentSSLCert, "agent-ssl", getAgentSSL(), "Agent SSL Certificate File")
	flag.StringVar(&params.Bag.AccessKey, "access-key", getAccessKey(), "AppD Controller Access Key")
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

	fmt.Printf("Parameters:\n %s", params)
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
	return os.Getenv("ACCESS_KEY") //"468b8634-6c42-443d-99e1-6b1e52d37b75"
}

func getSystemSSL() string {
	return os.Getenv("SYSTEM_SSL")
}

func getAgentSSL() string {
	return os.Getenv("AGENT_SSL")
}

func getControllerPort() uint {
	port := os.Getenv("CONTROLLER_PORT")
	if port != "" {
		var base = 16
		var size = 16
		num, err := strconv.ParseUint(port, base, size)
		if err != nil {
			fmt.Printf("Invalid controller port: %s", port)
			return 0
		} else {
			return uint(num)
		}
	} else {
		return 0
	}
}
