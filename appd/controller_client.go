package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/appdynamics/cluster-agent/config"
	m "github.com/appdynamics/cluster-agent/models"

	appd "appdynamics"
)

var lockMap = sync.RWMutex{}

type ControllerClient struct {
	logger       *log.Logger
	ConfManager  *config.MutexConfigManager
	regMetrics   map[string]bool
	MetricsCache map[string]float64
}

func NewControllerClient(cm *config.MutexConfigManager, logger *log.Logger) (*ControllerClient, error) {
	cfg := appd.Config{}

	bag := (*cm).Get()

	cfg.AppName = bag.AppName
	cfg.TierName = bag.TierName
	cfg.NodeName = bag.NodeName
	cfg.Controller.Host = bag.ControllerUrl
	cfg.Controller.Port = bag.ControllerPort
	cfg.Controller.UseSSL = bag.SSLEnabled
	if bag.SSLEnabled {
		err := writeSSLFromEnv(bag, logger)
		if err != nil {
			logger.Errorf("Unable to set SSL certificates. Using system certs %s", bag.SystemSSLCert)
			cfg.Controller.CertificateFile = bag.SystemSSLCert

		} else {
			logger.Errorf("Setting agent certs to %s", bag.AgentSSLCert)
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
		logger.WithField("error", err).Error("Error initializing the AppDynamics SDK.")
		return nil, err
	} else {
		logger.Info("Initialized AppDynamics SDK successfully")
	}
	logger.Debugf("AppD Controller info: %v", &cfg.Controller)

	controller := ControllerClient{ConfManager: cm, logger: logger, regMetrics: make(map[string]bool), MetricsCache: make(map[string]float64)}

	appID, tierID, nodeID, errAppd := controller.DetermineNodeID(bag.AppName, bag.TierName, bag.NodeName)
	if errAppd != nil {
		logger.Printf("Enable to fetch component IDs. Error: %v\n", errAppd)
	} else {
		bag.AppID = appID
		bag.TierID = tierID
		bag.NodeID = nodeID
		(*cm).Set(bag)
	}

	return &controller, nil
}

func (c *ControllerClient) RegisterMetrics(metrics m.AppDMetricList) error {
	c.logger.Println("Registering Metrics with the agent:")
	bt := appd.StartBT("RegMetrics", "")
	for _, metric := range metrics.Items {

		metric.MetricPath = fmt.Sprintf(metric.MetricPath, (*c.ConfManager).Get().TierName)
		exists := c.checkMetricCache(metric)
		if !exists {
			//		c.logger.Println(metric)
			appd.AddCustomMetric("", metric.MetricPath,
				metric.MetricTimeRollUpType,
				metric.MetricClusterRollUpType,
				appd.APPD_HOLEHANDLING_TYPE_REGULAR_COUNTER)
			appd.ReportCustomMetric("", metric.MetricPath, 0)
			c.saveMetricInCache(metric)
		}
	}
	appd.EndBT(bt)
	c.logger.Println("Done registering Metrics with the agent")

	return nil
}

func (c *ControllerClient) checkMetricCache(metric m.AppDMetric) bool {
	lockMap.RLock()
	defer lockMap.RUnlock()
	_, exists := c.regMetrics[metric.MetricPath]
	return exists
}

func (c *ControllerClient) saveMetricInCache(metric m.AppDMetric) {
	lockMap.Lock()
	defer lockMap.Unlock()
	c.regMetrics[metric.MetricPath] = true
}

func (c *ControllerClient) registerMetric(metric m.AppDMetric) error {
	exists := c.checkMetricCache(metric)
	if !exists {
		bt := appd.StartBT("RegSingleMetric", "")
		appd.AddCustomMetric("", metric.MetricPath,
			metric.MetricTimeRollUpType,
			metric.MetricClusterRollUpType,
			appd.APPD_HOLEHANDLING_TYPE_REGULAR_COUNTER)
		appd.ReportCustomMetric("", metric.MetricPath, 0)
		c.saveMetricInCache(metric)
		appd.EndBT(bt)
	}

	return nil
}

func (c *ControllerClient) PostMetrics(metrics m.AppDMetricList) error {
	bt := appd.StartBT("PostMetrics", "")
	for _, metric := range metrics.Items {
		metric.MetricPath = fmt.Sprintf(metric.MetricPath, (*c.ConfManager).Get().TierName)
		c.registerMetric(metric)
		appd.ReportCustomMetric("", metric.MetricPath, metric.MetricValue)
	}
	appd.EndBT(bt)

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
	rc := NewRestClient((*c.ConfManager).Get(), c.logger)
	data, err := rc.CallAppDController(path, "GET", nil)
	if err != nil {
		c.logger.Errorf("App by name response: %v %v\n", data, err)
		return appID, fmt.Errorf("Unable to find appID\n")
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
	c.logger.Debugf("Loaded App ID = %d \n", appID)
	return appID, nil
}

func (c *ControllerClient) FindNodeID(appID int, tierName string, nodeName string) (int, int, error) {
	var nodeID int = 0
	var tierID int = 0
	path := "restui/tiers/list/health"
	jsonData := fmt.Sprintf(`{"requestFilter": {"queryParams": {"applicationId": %d, "performanceDataFilter": "REPORTING"}, "filters": []}, "resultColumns": ["TIER_NAME"], "columnSorts": [{"column": "TIER_NAME", "direction": "ASC"}], "searchFilters": [], "limit": -1, "offset": 0}`, appID)
	d := []byte(jsonData)

	//	fmt.Printf("Node ID JSON data: %s", jsonData)

	rc := NewRestClient((*c.ConfManager).Get(), c.logger)
	data, err := rc.CallAppDController(path, "POST", d)
	if err != nil {
		c.logger.Errorf("Unable to find nodeID. %v\n", err)
		return tierID, nodeID, fmt.Errorf("Unable to find nodeID. %v\n", err)
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
				if nodeName == "" {
					return tierID, 0, nil
				}
				for k, prop := range tier {
					if k == "children" {
						for _, node := range prop.([]interface{}) {
							nodeObj := node.(map[string]interface{})
							for i, p := range nodeObj {
								if i == "nodeName" && p == nodeName {
									nodeID = int(nodeObj["nodeId"].(float64))
									c.logger.Debugf("Obtained TierID: %d, nodeID: %d\n", tierID, nodeID)
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

func (c *ControllerClient) GetMetricID(appID int, metricPath string) (float64, error) {

	path := fmt.Sprintf("restui/metricBrowser/%d", appID)
	arr := strings.Split(metricPath, "|")
	arrPath := arr[:len(arr)-1]
	metricName := arr[len(arr)-1]
	basePath := strings.Join(arrPath, "|")
	baseKey := fmt.Sprintf("%d_%s", appID, basePath)
	mk := fmt.Sprintf("%s|%s", baseKey, metricName)
	if id, ok := c.MetricsCache[mk]; ok {
		return id, nil
	} else {
		fmt.Printf("Key %s does not exist in metrics cache\n", mk)
	}

	arr = arr[:len(arr)-1]
	//	logger.Printf("Asking for metrics with payload: %s\n", strings.Join(arr, ","))

	body, eM := json.Marshal(arr)
	if eM != nil {
		fmt.Printf("Unable to serialize metrics path. %v \n", eM)
	}

	rc := NewRestClient((*c.ConfManager).Get(), c.logger)
	//	logger.Printf("Calling metrics update: %s. %s\n", path, string(body))
	data, err := rc.CallAppDController(path, "POST", body)
	if err != nil {
		return 0, fmt.Errorf("Unable to find metric ID")
	}
	var list []map[string]interface{}
	errJson := json.Unmarshal(data, &list)
	if errJson != nil {
		return 0, fmt.Errorf("Unable to deserialize metrics list\n")
	}

	var metricID float64 = 0
	for _, metricObj := range list {
		id, ok := metricObj["metricId"]
		mn := metricObj["name"]
		mp := fmt.Sprintf("%s|%s", baseKey, mn)
		//		logger.Printf("Adding metric key %s\n", mp)
		c.MetricsCache[mp] = id.(float64)
		if ok && metricObj["name"] == metricName {
			fmt.Printf("Metrics ID = %f \n", id.(float64))
			metricID = id.(float64)
		}
	}

	return metricID, nil
}

func (c *ControllerClient) EnableAppAnalytics(appID int, appName string) error {
	path := "restui/analyticsConfigTxnAnalyticsUiService/enableAnalyticsForApplication?enabled=true"
	jsonBody := fmt.Sprintf(`{"appId": %d, "name": "%s"}`, appID, appName)

	body := []byte(jsonBody)

	rc := NewRestClient((*c.ConfManager).Get(), c.logger)
	_, err := rc.CallAppDController(path, "POST", body)
	if err != nil && !strings.Contains(err.Error(), "204") {
		return fmt.Errorf("Cannot enable analytics for App %s. %v\n", appName, err)
	}
	fmt.Printf("Successfully enabled analytics for app %s\n", appName)

	//	//get BTs
	//	path = fmt.Sprintf("restui/analyticsConfigTxnAnalyticsUiService/enableAnalyticsDataCollectionForBTs/%d", appID)
	//	bts, errBT := rc.CallAppDController(path, "GET", nil)
	//	if errBT != nil {
	//		return fmt.Errorf("Unable to get the list of BTs for App %s. %v", appName, errBT)
	//	}
	//	var list []map[string]interface{}
	//	errJson := json.Unmarshal(bts, &list)
	//	if errJson != nil {
	//		return fmt.Errorf("Unable to deserialize metrics list")
	//	}
	//	btIDs := []int{}
	//	for _, obj := range list {
	//		for k, val := range obj {
	//			if k == "id" {
	//				btIDs = append(btIDs, val.(int))
	//			}
	//		}
	//	}

	//	//save BTs for analytics
	//	b, eM := json.Marshal(btIDs)
	//	if eM != nil {
	//		return fmt.Errorf("Unable to serialize list of BT ids. %v ", eM)
	//	}

	//	path = "restui/analyticsConfigTxnAnalyticsUiService/enableAnalyticsDataCollectionForBTs"

	//	_, errSave := rc.CallAppDController(path, "POST", b)
	//	if errSave != nil && !strings.Contains(errSave.Error(), "204") {
	//		return fmt.Errorf("Unable to save BTs for analytics. %v ", errSave)
	//	}

	//	fmt.Printf("Successfully enabled analytics for all BTs in app %s\n", appName)

	return nil

}

func (c *ControllerClient) DeleteAppDNode(nodeID int) error {
	path := fmt.Sprintf("restui/nodeUiService/deleteNode/%d", nodeID)

	rc := NewRestClient((*c.ConfManager).Get(), c.logger)
	_, err := rc.CallAppDController(path, "DELETE", nil)
	if err != nil {
		return fmt.Errorf("Unable to delete node %d. %v", nodeID, err)
	}

	return nil
}

func (c *ControllerClient) MarkNodeHistorical(nodeID int) {
	rc := NewRestClient((*c.ConfManager).Get(), c.logger)
	err := rc.MarkNodeHistorical(nodeID)
	if err != nil {
		c.logger.Error(err)
	} else {
		c.logger.Infof("Node %d marked as historical", nodeID)
	}
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
