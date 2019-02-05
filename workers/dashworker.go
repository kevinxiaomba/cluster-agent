package workers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	app "github.com/sjeltuhin/clusterAgent/appd"
	m "github.com/sjeltuhin/clusterAgent/models"
)

type DashboardWorker struct {
	Bag    *m.AppDBag
	Logger *log.Logger
}

func NewDashboardWorker(bag *m.AppDBag, l *log.Logger) DashboardWorker {
	return DashboardWorker{bag, l}
}

func (dw DashboardWorker) Run(stopCh <-chan struct{}) {
	dw.dashboardTicker(stopCh, time.NewTicker(15*time.Second))
	<-stopCh
}

func (dw *DashboardWorker) dashboardTicker(stop <-chan struct{}, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			dw.ensureClusterDashboard()
		case <-stop:
			ticker.Stop()
			return
		}
	}
}

func (dw *DashboardWorker) ensureClusterDashboard() {
	fmt.Println("Checking the dashboard...")
	rc := app.NewRestClient(dw.Bag, dw.Logger)
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
	dashName := fmt.Sprintf("%s-%s-%s", dw.Bag.AppName, dw.Bag.TierName, dw.Bag.DashboardSuffix)
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
		dw.createClusterDashboard(dashName)
	}
}

func (dw *DashboardWorker) createClusterDashboard(dashName string) {

	jsonFile, err := os.Open(dw.Bag.DashboardTemplatePath)
	// if we os.Open returns an error then handle it
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Successfully Opened Dashboard template")
	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)

	var dashObject m.Dashboard
	json.Unmarshal([]byte(byteValue), &dashObject)

	dashObject.Name = dashName

	metricsMetadata := m.NewClusterPodMetricsMetadata(dw.Bag, m.ALL, m.ALL).Metadata
	dw.updateDashboard(&dashObject, metricsMetadata)

	generated, errMar := json.Marshal(dashObject)
	if errMar != nil {
		fmt.Println(errMar)
		return
	}
	dir := filepath.Dir(dw.Bag.DashboardTemplatePath)
	fileForUpload := dir + "/GENERATED.json"
	fmt.Printf("About to upload file %s...\n", fileForUpload)
	errSave := ioutil.WriteFile(fileForUpload, generated, 0644)
	if errSave != nil {
		fmt.Printf("Issues when writing generated file. %v\n", errSave)
	}
	rc := app.NewRestClient(dw.Bag, dw.Logger)
	rc.CreateDashboard(fileForUpload)

}

func (dw *DashboardWorker) updateDashboard(dashObject *m.Dashboard, metricsMetadata map[string]m.AppDMetricMetadata) {
	for _, w := range dashObject.WidgetTemplates {
		for _, t := range w.DataSeriesTemplates {
			exp := t.MetricMatchCriteriaTemplate.MetricExpressionTemplate
			dw.updateMetricNode(exp, metricsMetadata, &w)
		}
	}
}

func (dw *DashboardWorker) updateMetricNode(expTemplate map[string]interface{}, metricsMetadata map[string]m.AppDMetricMetadata, parentWidget *m.WidgetTemplate) {
	metricObj := dw.getMatchingMetrics(expTemplate, metricsMetadata)
	if metricObj != nil {
		//metrics path matches. update
		dw.updateMetricPath(expTemplate, metricObj, parentWidget)
	} else {
		dw.updateMetricExpression(expTemplate, metricsMetadata, parentWidget)
	}
}

func (dw *DashboardWorker) updateMetricPath(expTemplate map[string]interface{}, metricObj *m.AppDMetricMetadata, parentWidget *m.WidgetTemplate) {
	expTemplate["metricPath"] = fmt.Sprintf(metricObj.Path, dw.Bag.TierName)
	scopeBlock := expTemplate["scopeEntity"].(map[string]interface{})
	scopeBlock["applicationName"] = dw.Bag.AppName
	scopeBlock["entityName"] = dw.Bag.TierName

	//drill-down
}

func (dw *DashboardWorker) updateMetricExpression(expTemplate map[string]interface{}, metricsMetadata map[string]m.AppDMetricMetadata, parentWidget *m.WidgetTemplate) {
	for i := 0; i < 2; i++ {
		expNodeName := fmt.Sprintf("expression%d", i+1)
		if expNode, ok := expTemplate[expNodeName]; ok {
			metricObj := dw.getMatchingMetrics(expNode.(map[string]interface{}), metricsMetadata)
			if metricObj != nil {
				dw.updateMetricPath(expNode.(map[string]interface{}), metricObj, parentWidget)
			} else {
				dw.updateMetricExpression(expTemplate, metricsMetadata, parentWidget)
			}
		}

	}
}

func (dw *DashboardWorker) getMatchingMetrics(node map[string]interface{}, metricsMetadata map[string]m.AppDMetricMetadata) *m.AppDMetricMetadata {

	if val, ok := node["metricPath"]; ok {
		if metricObj, matches := metricsMetadata[val.(string)]; matches {
			return &metricObj
		}
	}
	return nil
}

//dynamic dashboard
func (dw *DashboardWorker) updateDeploymentDashboard() {

}
