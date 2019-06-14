package controller

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
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
		if bag.AgentSSLCert != "" {
			logger.Infof("Setting custom agent cert:  %s", bag.AgentSSLCert)
			cfg.Controller.CertificateFile = bag.AgentSSLCert
		} else {
			logger.Infof("Using default system cert: %s", bag.SystemSSLCert)
			cfg.Controller.CertificateFile = bag.SystemSSLCert
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

	compatErr := controller.GetControllerStatus(bag)
	if compatErr != nil {
		return nil, compatErr
	}

	appID, tierID, nodeID, errAppd := controller.DetermineNodeID(bag.AppName, bag.TierName, bag.NodeName)
	if errAppd != nil {
		logger.Errorf("Enable to fetch component IDs. Error: %v\n", errAppd)
	} else {
		bag.AppID = appID
		bag.TierID = tierID
		bag.NodeID = nodeID
		(*cm).Set(bag)
	}
	logger.Infof("Controller version: %s", bag.PrintControllerVersion())
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
		return appID, fmt.Errorf("Unable to find appID %s\n", appName)
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
			if val != nil {
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
							if prop != nil {
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
		}
	}
	return tierID, nodeID, nil
}

func (c *ControllerClient) GetMetricID(appID int, metricPath string) (float64, error) {

	bag := (*c.ConfManager).Get()
	// if controller version is less than 4.5.7.5 use an older restui call
	if bag.CompareControllerVersions(4, 5, 7, 5) < 0 {
		c.logger.Debug("Using older version of GetMetricID call")
		return c.GetMetricID457(appID, metricPath)
	}

	path := "restui/metricBrowser/async/metric-tree/root"
	arr := strings.Split(metricPath, "|")
	arrPath := arr[:len(arr)-1]
	metricName := arr[len(arr)-1]
	basePath := strings.Join(arrPath, "|")
	baseKey := fmt.Sprintf("%d_%s", appID, basePath)
	mk := fmt.Sprintf("%s|%s", baseKey, metricName)
	if id, ok := c.MetricsCache[mk]; ok {
		return id, nil
	} else {
		c.logger.Debugf("Key %s does not exist in metrics cache\n", mk)
	}

	arr = arr[:len(arr)-1]

	body, eM := json.Marshal(arr)
	if eM != nil {
		fmt.Printf("Unable to serialize metrics path. %v \n", eM)
	}

	jsonData := fmt.Sprintf(`{"applicationId": %d, "livenessStatus": "ALL", "pathData": %s}`, appID, string(body))
	d := []byte(jsonData)

	rc := NewRestClient((*c.ConfManager).Get(), c.logger)
	c.logger.Debugf("Calling metrics update: %s. %s\n", path, d)
	data, err := rc.CallAppDController(path, "POST", d)
	if err != nil {
		return 0, fmt.Errorf("Unable to find metric ID. %v", err)
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
		c.logger.Debugf("Adding metric key %s\n", mp)
		c.MetricsCache[mp] = id.(float64)
		if ok && metricObj["name"] == metricName {
			c.logger.Debugf("Metrics ID = %f \n", id.(float64))
			metricID = id.(float64)
		}
	}

	return metricID, nil
}

func (c *ControllerClient) GetMetricID457(appID int, metricPath string) (float64, error) {

	bag := (*c.ConfManager).Get()
	path := fmt.Sprintf("restui/metricBrowser/%d", appID)

	if bag.ControllerVer3 >= 7 {
		path = fmt.Sprintf("restui/metricBrowser/metricTreeRoots/%d", appID)
	}

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
	//	c.logger.Infof("Asking for metrics with payload: %s\n", strings.Join(arr, ","))

	body, eM := json.Marshal(arr)
	if eM != nil {
		fmt.Printf("Unable to serialize metrics path. %v \n", eM)
	}

	rc := NewRestClient((*c.ConfManager).Get(), c.logger)
	//	c.logger.Infof("Calling metrics update: %s. %s\n", path, string(body))
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

func (c *ControllerClient) GetControllerStatus(bag *m.AppDBag) error {
	rc := NewRestClient((*c.ConfManager).Get(), c.logger)
	data, err := rc.GetControllerVersion()
	if err != nil {
		c.logger.Error(err)
	}

	var controllerInfo ControllerInfo
	errJson := xml.Unmarshal(data, &controllerInfo)
	if errJson != nil {
		return fmt.Errorf("Unable to deserialize controller version\n")
	}

	c.logger.Infof("Controller version: %s", controllerInfo.Serverinfo.Serverversion)
	numAr := strings.Split(controllerInfo.Serverinfo.Serverversion, "-")
	if len(numAr) == 4 {
		bag.ControllerVer1, _ = strconv.Atoi(numAr[0])
		bag.ControllerVer2, _ = strconv.Atoi(numAr[1])
		bag.ControllerVer3, _ = strconv.Atoi(numAr[2])
		bag.ControllerVer4, _ = strconv.Atoi(numAr[3])
	}

	if bag.ControllerVer1 < 4 ||
		(bag.ControllerVer1 >= 4 && bag.ControllerVer2 < 5) ||
		(bag.ControllerVer1 == 4 && bag.ControllerVer2 == 5 && bag.ControllerVer3 < 5) {
		return fmt.Errorf("AppDynamics controller version %s is incompatible. 4.5.5+ is required", bag.PrintControllerVersion())
	}

	return nil

}

func ValidateAccount(bag *m.AppDBag, logger *log.Logger) error {
	path := "restui/user/account"

	logger.Info("Loading account info...")

	rc := NewRestClient(bag, logger)
	data, err := rc.CallAppDController(path, "GET", nil)
	if err != nil {
		return fmt.Errorf("Unable to get the AppDynamics account information. %v", err)
	}
	var accountObj map[string]interface{}
	errJson := json.Unmarshal(data, &accountObj)
	if errJson != nil {
		return fmt.Errorf("Unable to deserialize AppDynamics account object. %v", errJson)
	}
	accountID := 0
	for k, v := range accountObj {
		if k == "account" {
			obj := v.(map[string]interface{})
			accountID = int(obj["id"].(float64))
			bag.AccessKey = obj["accessKey"].(string)
			bag.Account = obj["name"].(string)
			bag.GlobalAccount = obj["globalAccountName"].(string)
			break
		}
	}
	if accountID > 0 {
		logger.Info("Account info loaded. Validating access...")
		valid, licErr := rc.validateLicense(accountID)
		if licErr != nil {
			return fmt.Errorf("Unable to get license information for the AppDynamics account. %v", licErr)
		}
		if !valid {
			return fmt.Errorf("AppDynamics account does not have the required Golang license")
		}
		logger.Info("Golang license validated...")

	} else {
		return fmt.Errorf("Failed to get the AppDynamics account information. Invalid response from server")
	}
	return nil
}
