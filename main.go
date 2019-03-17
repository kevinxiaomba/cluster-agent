package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"path/filepath"
	"sync"

	"github.com/sjeltuhin/clusterAgent/config"
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
	var method string
	params := Flags{}

	flag.StringVar(&params.Kubeconfig, "kubeconfig", getKubeConfigPath(), "(optional) absolute path to the kubeconfig file")
	flag.StringVar(&params.Bag.AgentNamespace, "agent-namespace", getAgentNamespace(), "Agent namespace")
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
	flag.StringVar(&params.Bag.PodSchemaName, "schema-pods", "kube_pod_snapshots", "Pod schema name")
	flag.StringVar(&params.Bag.NodeSchemaName, "schema-nodes", "kube_node_snapshots", "Node schema name")
	flag.StringVar(&params.Bag.EventSchemaName, "schema-events", "kube_event_snapshots", "Event schema name")
	flag.StringVar(&params.Bag.DeploySchemaName, "schema-deploys", "kube_deploy_snapshots", "Deployment schema name")
	flag.StringVar(&params.Bag.RSSchemaName, "schema-rs", "kube_rs_snapshots", "Replica set schema name")
	flag.StringVar(&params.Bag.DaemonSchemaName, "schema-daemon", "kube_daemon_snapshots", "Daemon set schema name")
	flag.StringVar(&params.Bag.ContainerSchemaName, "schema-containers", "kube_container_snapshots", "Container schema name")
	flag.StringVar(&params.Bag.LogSchemaName, "schema-logs", "kube_logs", "Log schema name")
	flag.StringVar(&params.Bag.DashboardTemplatePath, "template-path", getTemplatePath(), "Dashboard template path")
	flag.StringVar(&params.Bag.DashboardSuffix, "dash-name", getDashboardSuffix(), "Dashboard name")
	flag.IntVar(&params.Bag.DashboardDelayMin, "dash-delay", getDashboardDelayMin(), "Dashboard delay (min)")
	flag.IntVar(&params.Bag.EventAPILimit, "event-batch-size", 100, "Max number of AppD events record to send in a batch")
	flag.IntVar(&params.Bag.MetricsSyncInterval, "metrics-sync-interval", 60, "Frequency of metrics pushes to the controller, sec")
	flag.IntVar(&params.Bag.SnapshotSyncInterval, "snapshot-sync-interval", 15, "Frequency of snapshot pushes to events api, sec")
	flag.StringVar(&params.Bag.JavaAgentVersion, "java-agent-version", getJavaAgentVersion(), "AppD Java Agent Version")
	flag.StringVar(&params.Bag.AppDJavaAttachImage, "java-attach-image", getJavaAttachImage(), "Java Attach Image")
	flag.StringVar(&params.Bag.AppDDotNetAttachImage, "dotnet-attach-image", getDotNetAttachImage(), "DotNet Attach Image")
	flag.StringVar(&params.Bag.AgentLabel, "agent-label", "appd-agent", "AppD Agent Label")
	flag.StringVar(&params.Bag.AgentEnvVar, "agent-envvar", getAgentEnvvar(), "AppD Agent Env Var for instrumentation")
	flag.StringVar(&params.Bag.AppDAppLabel, "appd-app", "appd-app", "AppD App Label")
	flag.StringVar(&params.Bag.AppDTierLabel, "appd-tier", "appd-tier", "AppD Tier Label")
	flag.StringVar(&params.Bag.AppDAnalyticsLabel, "appd-biq", "appd-biq", "AppD Analytics Label")
	flag.StringVar(&params.Bag.AppLogMountName, "log-mount-name", "appd-volume", "App Log Mount Name")
	flag.StringVar(&params.Bag.AppLogMountPath, "log-mount-path", "/opt/appdlogs", "App Log Mount Path")
	flag.StringVar(&params.Bag.AgentMountName, "agent-mount-name", "appd-agent-repo", "AppD Agent Mount Name")
	flag.StringVar(&params.Bag.AgentMountPath, "mount-path", "/opt/appd", "AppD Agent Mount Path")
	flag.StringVar(&params.Bag.JDKMountName, "jdkmount-name", "jdk-repo", "JDK Mount Name")
	flag.IntVar(&params.Bag.AgentServerPort, "ws-port", 8989, "Agent Web Server port number")
	flag.StringVar(&params.Bag.JDKMountPath, "jdkmount-path", "$JAVA_HOME/lib", "JDK Mount Path")
	flag.StringVar(&params.Bag.NodeNamePrefix, "node-prefix", params.Bag.TierName, "Node name prefix. Used when node reuse is set to true")
	flag.StringVar(&params.Bag.AnalyticsAgentUrl, "analytics-agent-url", getAnalyticsAgentUrl(), "Analytics Agent Url")
	flag.StringVar(&params.Bag.AnalyticsAgentImage, "analytics-agent-image", getAnalyticsAgentImage(), "Analytics Agent Image")
	flag.StringVar(&params.Bag.AnalyticsAgentContainerName, "analytics-agent-container-name", "appd-analytics-agent", "Analytics Agent Container Name")
	flag.StringVar(&params.Bag.AppDInitContainerName, "appd-init-container-name", "appd-agent-attach", "AppD Init Container Name")
	flag.StringVar(&method, "appd-instrument-method", getAgentInstrumentationMethod(), "AppD Agent Instrumentation Method (copy, mount)")
	params.Bag.InstrumentationMethod = m.InstrumentationMethod(method)
	flag.StringVar(&params.Bag.InitContainerDir, "init-container-dir", "/opt/temp/.", "Directory with artifacts in the init container")
	var dash string
	flag.StringVar(&dash, "deploys-to-dash", getDeploysToDashboard(), "List of deployments to dashboard")
	params.Bag.DeploysToDashboard = strings.Split(dash, ",")
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

	configManager := config.NewMutexConfigManager(&params.Bag)

	defer func() {
		configManager.Close()
	}()

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
	l := log.New(os.Stdout, "[APPD_CLUSTER_MONITOR]", log.Lshortfile)
	controller := w.NewController(configManager, clientset, l, config)
	validationErr := controller.ValidateParameters()
	if validationErr != nil {
		log.Printf("Cluster Agent parameters are invalid. %v. Terminating...", validationErr)
		return
	}
	controller.Run(stop, &wg)

	<-sigs

	fmt.Println("Shutting down...")

	close(stop)

	wg.Wait()

}

func authFromConfig(params *Flags) (*rest.Config, error) {
	if params.Kubeconfig != "" {
		fmt.Printf("Kube config = %s\n", params.Kubeconfig)
		return clientcmd.BuildConfigFromFlags("", params.Kubeconfig)
	} else {
		fmt.Printf("Using in-cluster auth")
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
			//check if a valid path
			if _, err := os.Stat(path); err != nil {
				fmt.Printf("Default path to  %s does not exist", path)
				path = ""
			}
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

func getDashboardSuffix() string {
	dn := os.Getenv("DASH_NAME")
	if dn == "" {
		dn = "SUMMARY"
	}

	return dn
}

func getDashboardDelayMin() int {
	delay := os.Getenv("DASH_DELAY")
	if delay == "" {
		return 0
	} else {
		d, err := strconv.Atoi(delay)
		if err != nil {
			return 0
		}

		return d
	}
}

func getAgentNamespace() string {
	return os.Getenv("AGENT_NAMESPACE")
}

func getJavaAgentVersion() string {
	return os.Getenv("JAVA_AGENT_VERSION")
}

func getAnalyticsAgentUrl() string {
	return os.Getenv("ANALYTICS_AGENT_URL")
}

func getAnalyticsAgentImage() string {
	return os.Getenv("ANALYTICS_AGENT_IMAGE")
}

func getDotNetAttachImage() string {
	return os.Getenv("DOTNET_ATTACH_IMAGE")
}

func getAgentInstrumentationMethod() string {
	method := os.Getenv("AGENT_INSTRUMENTATION_METHOD")
	if method == "" {
		method = string(m.Copy)
	}
	return method
}

func getAgentEnvvar() string {
	envvar := os.Getenv("APPD_AGENT_ENV_VAR")
	if envvar == "" {
		envvar = "$JAVA_OPTS"
	}

	return envvar
}

func getJavaAttachImage() string {
	return os.Getenv("JAVA_ATTACH_IMAGE")
}

func getDeploysToDashboard() string {
	return os.Getenv("DEPLOYS_TO_DASH")
}

func getTemplatePath() string {
	templPath := os.Getenv("DASH_TEMPLATE__PATH")
	if templPath == "" {
		absPath, err := filepath.Abs("templates/k8s_dashboard_template.json")
		if err != nil {
			fmt.Printf("Cannot find dashbaord template. %v\n", err)
		}
		templPath = absPath
	}
	return templPath
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
