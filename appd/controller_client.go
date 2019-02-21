package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	m "github.com/sjeltuhin/clusterAgent/models"

	appd "appdynamics"
)

type ControllerClient struct {
	logger     *log.Logger
	Bag        *m.AppDBag
	regMetrics map[string]bool
}

func NewControllerClient(bag *m.AppDBag, logger *log.Logger) *ControllerClient {
	cfg := appd.Config{}

	cfg.AppName = bag.AppName
	cfg.TierName = bag.TierName
	cfg.NodeName = bag.NodeName
	cfg.Controller.Host = bag.ControllerUrl
	cfg.Controller.Port = bag.ControllerPort
	cfg.Controller.UseSSL = bag.SSLEnabled
	if bag.SSLEnabled {
		err := writeSSLFromEnv(bag, logger)
		if err != nil {
			logger.Printf("Unable to set SSL certificates. Using system certs %s", bag.SystemSSLCert)
			cfg.Controller.CertificateFile = bag.SystemSSLCert

		} else {
			logger.Printf("Setting agent certs to %s", bag.AgentSSLCert)
			cfg.Controller.CertificateFile = bag.AgentSSLCert
		}
	}
	cfg.Controller.Account = bag.Account
	cfg.Controller.AccessKey = bag.AccessKey
	cfg.UseConfigFromEnv = false
	cfg.InitTimeoutMs = 1000
	//	cfg.Logging.BaseDir = "__console__"
	cfg.Logging.MinimumLevel = appd.APPD_LOG_LEVEL_DEBUG
	if err := appd.InitSDK(&cfg); err != nil {
		logger.Printf("Error initializing the AppDynamics SDK. %v\n", err)
	} else {
		logger.Printf("Initialized AppDynamics SDK successfully\n")
	}
	logger.Println(&cfg.Controller)

	controller := ControllerClient{Bag: bag, logger: logger, regMetrics: make(map[string]bool)}

	appID, tierID, nodeID, errAppd := controller.DetermineNodeID(bag.AppName, bag.TierName, bag.NodeName)
	if errAppd != nil {
		logger.Printf("Enable to fetch component IDs. Error: %v\n", errAppd)
	} else {
		bag.AppID = appID
		bag.TierID = tierID
		bag.NodeID = nodeID
	}

	return &controller
}

func (c *ControllerClient) RegisterMetrics(metrics m.AppDMetricList) error {
	c.logger.Println("Registering Metrics with the agent:")
	bt := appd.StartBT("RegMetrics", "")
	for _, metric := range metrics.Items {

		metric.MetricPath = fmt.Sprintf(metric.MetricPath, c.Bag.TierName)
		_, exists := c.regMetrics[metric.MetricPath]
		if !exists {
			//		c.logger.Println(metric)
			appd.AddCustomMetric("", metric.MetricPath,
				metric.MetricTimeRollUpType,
				metric.MetricClusterRollUpType,
				appd.APPD_HOLEHANDLING_TYPE_REGULAR_COUNTER)
			appd.ReportCustomMetric("", metric.MetricPath, 0)
			c.regMetrics[metric.MetricPath] = true
		}
	}
	appd.EndBT(bt)
	c.logger.Println("Done registering Metrics with the agent")

	return nil
}

func (c *ControllerClient) registerMetric(metric m.AppDMetric) error {
	_, exists := c.regMetrics[metric.MetricPath]
	if !exists {
		bt := appd.StartBT("RegSingleMetric", "")
		appd.AddCustomMetric("", metric.MetricPath,
			metric.MetricTimeRollUpType,
			metric.MetricClusterRollUpType,
			appd.APPD_HOLEHANDLING_TYPE_REGULAR_COUNTER)
		appd.ReportCustomMetric("", metric.MetricPath, 0)
		c.regMetrics[metric.MetricPath] = true
		appd.EndBT(bt)
	}

	return nil
}

func (c *ControllerClient) PostMetrics(metrics m.AppDMetricList) error {
	c.logger.Println("Pushing Metrics through the agent:")
	bt := appd.StartBT("PostMetrics", "")
	for _, metric := range metrics.Items {
		metric.MetricPath = fmt.Sprintf(metric.MetricPath, c.Bag.TierName)
		c.registerMetric(metric)
		appd.ReportCustomMetric("", metric.MetricPath, metric.MetricValue)
	}
	appd.EndBT(bt)
	c.logger.Println("Done pushing Metrics through the agent")

	return nil
}

func writeSSLFromEnv(bag *m.AppDBag, logger *log.Logger) error {
	from, err := os.Open(bag.SystemSSLCert)
	if err != nil {
		logger.Println(err)
		return err
	}
	defer from.Close()

	logger.Printf("Copying system certificates from %s to %s \n", bag.SystemSSLCert, bag.AgentSSLCert)
	to, err := os.OpenFile(bag.AgentSSLCert, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		logger.Println(err)
		return err
	}
	defer to.Close()

	_, err = io.Copy(to, from)
	if err != nil {
		logger.Println(err)
		return err
	}

	logger.Println("Writing Trusted certificates to %s", bag.AgentSSLCert)
	trustedCerts := os.Getenv("APPD_TRUSTED_CERTS")
	if trustedCerts != "NOTSET" {
		if _, err = to.Write([]byte(trustedCerts)); err != nil {
			logger.Println(err)
			return err
		}
	} else {
		logger.Println("Trusted certificates not found, skipping")
	}

	return nil
}

func (c *ControllerClient) StartBT(name string) appd.BtHandle {
	return appd.StartBT(name, "")
}

func (c *ControllerClient) StopBT(bth appd.BtHandle) {
	appd.EndBT(bth)
}

func (c *ControllerClient) DetermineNodeID(appName string, tierName string, nodeName string) (int, int, int, error) {
	appID, err := c.FindAppID(appName)
	if err != nil {
		return appID, 0, 0, err
	}

	tierID, nodeID, e := c.FindNodeID(appID, tierName, nodeName)

	if e != nil {
		return appID, tierID, nodeID, e
	}

	return appID, tierID, nodeID, nil
}

func (c *ControllerClient) FindAppID(appName string) (int, error) {
	var appID int = 0
	path := fmt.Sprintf("restui/applicationManagerUiBean/applicationByName?applicationName=%s", appName)
	fmt.Printf("App by name path %s", path)
	logger := log.New(os.Stdout, "[APPD_CLUSTER_MONITOR]", log.Lshortfile)
	rc := NewRestClient(c.Bag, logger)
	data, err := rc.CallAppDController(path, "GET", nil)
	if err != nil {
		fmt.Printf("App by name response: %v %v", data, err)
		return appID, fmt.Errorf("Unable to find appID")
	}
	var appObj map[string]interface{}
	errJson := json.Unmarshal(data, &appObj)
	if errJson != nil {
		return appID, fmt.Errorf("Unable to deserialize app object")
	}
	for key, val := range appObj {
		if key == "id" {
			appID = int(val.(float64))
			break
		}
	}
	fmt.Printf("App ID = %d \n", appID)
	return appID, nil
}

func (c *ControllerClient) FindNodeID(appID int, tierName string, nodeName string) (int, int, error) {
	var nodeID int = 0
	var tierID int = 0
	path := "restui/tiers/list/health"
	jsonData := fmt.Sprintf(`{"requestFilter": {"queryParams": {"applicationId": %d, "performanceDataFilter": "REPORTING"}, "filters": []}, "resultColumns": ["TIER_NAME"], "columnSorts": [{"column": "TIER_NAME", "direction": "ASC"}], "searchFilters": [], "limit": -1, "offset": 0}`, appID)
	d := []byte(jsonData)

	fmt.Printf("Node ID JSON data: %s", jsonData)

	logger := log.New(os.Stdout, "[APPD_CLUSTER_MONITOR]", log.Lshortfile)
	rc := NewRestClient(c.Bag, logger)
	data, err := rc.CallAppDController(path, "POST", d)
	if err != nil {
		fmt.Printf("Unable to find nodeID. %v", err)
		return tierID, nodeID, fmt.Errorf("Unable to find nodeID. %v", err)
	}
	var tierObj map[string]interface{}
	errJson := json.Unmarshal(data, &tierObj)
	if errJson != nil {
		return tierID, nodeID, fmt.Errorf("Unable to deserialize tier object")
	}
	for key, val := range tierObj {
		if key == "data" {
			s := val.([]interface{})
			for _, t := range s {
				tier := t.(map[string]interface{})
				name := tier["name"].(string)
				if name != tierName {
					continue
				}
				tierID = int(tier["id"].(float64))
				for k, prop := range tier {
					if k == "children" {
						for _, node := range prop.([]interface{}) {
							nodeObj := node.(map[string]interface{})
							for i, p := range nodeObj {
								if i == "nodeName" && p == nodeName {
									nodeID = int(nodeObj["nodeId"].(float64))
									fmt.Printf("TierID: %d, nodeID: %d\n", tierID, nodeID)
									return tierID, nodeID, nil
								}
							}
						}
					}
				}
			}
		}
	}
	return tierID, nodeID, nil
}

func (c *ControllerClient) GetMetricID(metricPath string) (float64, error) {
	logger := log.New(os.Stdout, "[APPD_CLUSTER_MONITOR]", log.Lshortfile)
	path := "restui/metricBrowser/88"
	arr := strings.Split(metricPath, "|")
	metricName := arr[len(arr)-1]
	arr = arr[:len(arr)-1]
	logger.Printf("Asking for metrics with payload: %s", strings.Join(arr, ","))

	body, eM := json.Marshal(arr)
	if eM != nil {
		fmt.Printf("Unable to serialize metrics path. %v ", eM)
	}

	rc := NewRestClient(c.Bag, logger)
	data, err := rc.CallAppDController(path, "POST", body)
	if err != nil {
		return 0, fmt.Errorf("Unable to find metric ID")
	}
	var list []map[string]interface{}
	errJson := json.Unmarshal(data, &list)
	if errJson != nil {
		return 0, fmt.Errorf("Unable to deserialize metrics list")
	}

	for _, metricObj := range list {
		id, ok := metricObj["metricId"]
		if ok && metricObj["name"] == metricName {
			fmt.Printf("Metrics ID = %d ", id.(float64))
			return id.(float64), nil
		}
	}

	return 0, nil
}

// enable analytics
// POST controller/restui/analyticsConfigTxnAnalyticsUiService/enableAnalyticsForApplication?enabled=true
// appId: 12
// name: "RemoteBiQ"

// all business transactions
// restui/bt/allBusinessTransactions/<appID>

// save = enable BTs for analytics
// POST controller/restui/analyticsConfigTxnAnalyticsUiService/enableAnalyticsDataCollectionForBTs
// [1, 2, 3]
