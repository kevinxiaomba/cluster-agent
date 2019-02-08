package workers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	app "github.com/sjeltuhin/clusterAgent/appd"
	m "github.com/sjeltuhin/clusterAgent/models"
)

const (
	WIDGET_GAP   int    = 10
	FULL_CLUSTER string = "/k8s_dashboard_template.json"
	DEPLOY_BASE  string = "/base_deploy_template.json"
	TIER_SUMMARY string = "/tier_widget.json"
	BACKGROUND   string = "/background.json"
)

type DashboardWorker struct {
	Bag    *m.AppDBag
	Logger *log.Logger
}

func NewDashboardWorker(bag *m.AppDBag, l *log.Logger) DashboardWorker {
	return DashboardWorker{bag, l}
}

func (dw *DashboardWorker) ensureClusterDashboard() error {
	dashName := fmt.Sprintf("%s-%s-%s", dw.Bag.AppName, dw.Bag.TierName, dw.Bag.DashboardSuffix)
	dashboard, err := dw.loadDashboard(dashName)
	if err != nil {
		return err
	}
	if dashboard == nil {
		fmt.Printf("Dashaboard %s does not exist. Creating...\n", dashName)
		dw.createClusterDashboard(dashName)
	}
	return nil
}

func (dw *DashboardWorker) saveDashboard(dashboard *m.Dashboard) error {

	data, err := json.Marshal(dashboard)
	if err != nil {
		return fmt.Errorf("Unable to save dashboard %s. %v", dashboard.Name, err)
	}
	rc := app.NewRestClient(dw.Bag, dw.Logger)
	data, errSave := rc.CallAppDController("restui/dashboards/updateDashboard", "POST", data)
	if errSave != nil {

		return fmt.Errorf("Unable to save dashaboard %s. %v\n", dashboard.Name, errSave)
	}

	return nil
}

func (dw *DashboardWorker) loadDashboard(dashName string) (*m.Dashboard, error) {
	var theDash *m.Dashboard = nil

	fmt.Println("Checking the dashboard...")
	rc := app.NewRestClient(dw.Bag, dw.Logger)
	data, err := rc.CallAppDController("restui/dashboards/getAllDashboardsByType/false", "GET", nil)
	if err != nil {
		fmt.Printf("Unable to get the list of dashaboard. %v\n", err)
		return theDash, err
	}
	var list []m.Dashboard
	e := json.Unmarshal(data, &list)
	if e != nil {
		fmt.Printf("Unable to deserialize the list of dashaboards. %v\n", e)
		return theDash, e
	}

	for _, d := range list {
		if d.Name == dashName {
			fmt.Printf("Dashaboard %s exists. No action required\n", dashName)
			theDash = &d
			break
		}
	}
	return theDash, nil
}

func (dw *DashboardWorker) readJsonTemplate(templateFile string) ([]byte, error, bool) {
	if templateFile == "" {
		templateFile = dw.Bag.DashboardTemplatePath
	}

	dir := filepath.Dir(dw.Bag.DashboardTemplatePath)
	templateFile = dir + templateFile

	exists := true

	jsonFile, err := os.Open(templateFile)

	if err != nil {
		exists = !os.IsNotExist(err)
		return nil, fmt.Errorf("Unable to open template file %s. %v", templateFile, err), exists
	}
	fmt.Printf("Successfully opened template %s\n", templateFile)
	defer jsonFile.Close()

	byteValue, errRead := ioutil.ReadAll(jsonFile)
	if errRead != nil {
		return nil, fmt.Errorf("Unable to read json file %s. %v", templateFile, errRead), exists
	}

	return byteValue, nil, exists
}

func (dw *DashboardWorker) loadDashboardTemplate(templateFile string) (*m.Dashboard, error, bool) {
	var dashObject *m.Dashboard = nil
	byteValue, err, exists := dw.readJsonTemplate(templateFile)
	if err != nil || !exists {
		return nil, err, exists
	}

	serErr := json.Unmarshal([]byte(byteValue), &dashObject)
	if serErr != nil {
		return dashObject, fmt.Errorf("Unable to get dashboard template %s. %v", templateFile, serErr), exists
	}

	return dashObject, nil, exists
}

func (dw *DashboardWorker) loadWidgetTemplate(templateFile string) (*[]m.WidgetTemplate, error, bool) {
	var widget *[]m.WidgetTemplate = nil

	byteValue, err, exists := dw.readJsonTemplate(templateFile)
	if err != nil || !exists {
		return nil, err, exists
	}

	serErr := json.Unmarshal([]byte(byteValue), &widget)
	if serErr != nil {
		return widget, fmt.Errorf("Unable to get widget template %s. %v", templateFile, serErr), exists
	}

	return widget, nil, exists
}

func (dw *DashboardWorker) createClusterDashboard(dashName string) error {

	dashObject, err, _ := dw.loadDashboardTemplate(FULL_CLUSTER)
	if err != nil {
		return err
	}

	dashObject.Name = dashName

	metricsMetadata := m.NewClusterPodMetricsMetadata(dw.Bag, m.ALL, m.ALL).Metadata
	dw.updateDashboard(dashObject, metricsMetadata)

	_, errCreate := dw.createDashboard(dashObject, "/GENERATED.json")

	return errCreate
}

func (dw *DashboardWorker) createDashboard(dashboard *m.Dashboard, genPath string) (*m.Dashboard, error) {

	fileForUpload, err := dw.saveTemplate(dashboard, genPath)
	if err != nil {
		return nil, fmt.Errorf("Unable to create dashboard. %v\n", err)
	}

	rc := app.NewRestClient(dw.Bag, dw.Logger)
	data, errCreate := rc.CreateDashboard(*fileForUpload)
	if errCreate != nil {
		return nil, errCreate
	}
	var raw map[string]interface{}
	e := json.Unmarshal(data, &raw)
	if e != nil {
		return nil, fmt.Errorf("Unable to create dashaboard. Unable to deserialize the new dashaboards. %v\n", err)
	}
	dashObj, ok := raw["dashboard"]
	obj := dashObj.(map[string]interface{})
	if ok {
		dashboard.ID = obj["id"].(float64)
	}
	fmt.Printf("Dashboard saved %.0f %s\n", dashboard.ID, dashboard.Name)
	return dashboard, nil
}

func (dw *DashboardWorker) saveTemplate(dashboard *m.Dashboard, genPath string) (*string, error) {
	generated, errMar := json.Marshal(dashboard)
	if errMar != nil {
		return nil, fmt.Errorf("Unable to save template. %v", errMar)
	}
	dir := filepath.Dir(dw.Bag.DashboardTemplatePath)
	fileForUpload := dir + genPath
	fmt.Printf("About to upload file %s...\n", fileForUpload)
	errSave := ioutil.WriteFile(fileForUpload, generated, 0644)
	if errSave != nil {
		return nil, fmt.Errorf("Issues when writing generated template. %v", errMar)
	}
	return &fileForUpload, nil
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
func (dw *DashboardWorker) updateTierDashboard(bag *m.DashboardBag) error {
	fileName := fmt.Sprintf("/deploy/%s_%s.json", bag.Namespace, bag.TierName)
	dashboard, err, exists := dw.loadDashboardTemplate(fileName)
	if err != nil && exists {
		return fmt.Errorf("Deployment template for %s/%s exists, but cannot be loaded. %v", bag.Namespace, bag.TierName, err)
	}
	dashName := fmt.Sprintf("%s-%s-%s-%s", dw.Bag.AppName, bag.Namespace, bag.TierName, dw.Bag.DashboardSuffix)
	if !exists {
		fmt.Printf("Checking dashboard for deployment %s/%s on the server\n", bag.Namespace, bag.TierName)
		dashboard, err = dw.loadDashboard(dashName)
		if dashboard == nil {
			fmt.Printf("Not found. Creating dashboard for deployment %s/%s from scratch\n", bag.Namespace, bag.TierName)
			dashboard, err, exists = dw.loadDashboardTemplate(DEPLOY_BASE)
			fmt.Printf("Load template %s. Error: %v, Exists: %t\n", DEPLOY_BASE, err, true)
			if !exists {
				return fmt.Errorf("Deployment template not found. Aborting dashboard generation")
			}
			if err != nil {
				return fmt.Errorf("Deployment template exists, but cannot be loaded. %v", err)
			}
			dashboard.Name = dashName
			dashboard, err = dw.createDashboard(dashboard, fmt.Sprintf("/scratch/GENERATED_%s_%s.json", bag.Namespace, bag.TierName))
			fmt.Printf("Create dashboard result. Error: %v, \n", err)
			if err != nil {
				return err
			}
		}
	}
	if dashboard == nil {
		return fmt.Errorf("Unable to build tier Dashboard. Weird things do happen")
	}
	//by this time we should have dashboard object with id
	//save base template with id, strip widgets
	//	dashboard.WidgetTemplates = []m.WidgetTemplate{}
	_, errSave := dw.saveTemplate(dashboard, fileName)
	if errSave != nil {
		fmt.Printf("Issues when saving template for dashboard %s/%s\n", bag.Namespace, bag.TierName)
	}
	updated, errGen := dw.generateDeploymentDashboard(dashboard, bag)
	fmt.Printf("generateDeploymentDashboard. %v\n", errGen)
	if errGen != nil {
		return fmt.Errorf("Issues when generating template for deployment dashboard %s, %s. %v\n", bag.Namespace, bag.TierName, err)
	}

	errSaveDash := dw.saveDashboard(updated)
	fmt.Printf("saveDashboard. %v\n", errSaveDash)

	return errSaveDash
}

func (dw *DashboardWorker) generateDeploymentDashboard(dashboard *m.Dashboard, bag *m.DashboardBag) (*m.Dashboard, error) {
	//	dashboard.WidgetTemplates = []m.WidgetTemplate{}
	fmt.Printf("generateDeploymentDashboard %s/%s\n", bag.Namespace, bag.TierName)
	d, err := dw.addBackground(dashboard, bag)
	if err != nil {
		return nil, err
	}
	d, err = dw.addDeploymentSummaryWidget(dashboard, bag)
	fmt.Printf("Added %d widgets to dash %s\n", len(dashboard.WidgetTemplates), dashboard.Name)
	return d, err
}

func (dw *DashboardWorker) addBackground(dashboard *m.Dashboard, bag *m.DashboardBag) (*m.Dashboard, error) {
	widgetList, err, exists := dw.loadWidgetTemplate(BACKGROUND)
	if err != nil && exists {
		return nil, fmt.Errorf("Background template exists, but cannot be loaded. %v\n", err)
	}
	if !exists {
		return nil, fmt.Errorf("Background template does not exist, skipping dashboard for deployment %s/%s. %v\n", bag.Namespace, bag.TierName, err)
	}
	backgroundWidet := (*widgetList)[0]
	backgroundWidet.Height = dashboard.Height - WIDGET_GAP
	dashboard.WidgetTemplates = append(dashboard.WidgetTemplates, backgroundWidet)
	return dashboard, nil
}

func (dw *DashboardWorker) addDeploymentSummaryWidget(dashboard *m.Dashboard, bag *m.DashboardBag) (*m.Dashboard, error) {
	widgetList, err, exists := dw.loadWidgetTemplate(TIER_SUMMARY)
	if err != nil && exists {
		return nil, fmt.Errorf("Tier summary template exists, but cannot be loaded. %v\n", err)
	}
	if !exists {
		return nil, fmt.Errorf("Tier summary template does not exist, skipping dashboard for deployment %s/%s. %v\n", bag.Namespace, bag.TierName, err)
	}

	maxY := 0
	for _, widget := range *widgetList {
		if widget.Y+widget.Height > maxY {
			maxY = widget.Y + widget.Height
		}
		if widget.ApplicationReference != nil {
			widget.ApplicationReference.ApplicationName = bag.AppName
			widget.ApplicationReference.EntityName = bag.AppName
		}
		if widget.EntityReferences != nil {
			refs := widget.EntityReferences
			for _, er := range *refs {
				er.ApplicationName = bag.AppName
				er.EntityName = bag.TierName
				break
			}
		}

		for _, t := range widget.DataSeriesTemplates {
			exp := t.MetricMatchCriteriaTemplate.MetricExpressionTemplate
			dw.updateDashNode(exp, bag, &widget)
		}
	}

	dashboard.Height = maxY + WIDGET_GAP
	dashboard.WidgetTemplates = *widgetList
	//adjust the background height
	dashboard.WidgetTemplates[0].Height = dashboard.Height

	return dashboard, nil
}

func (dw *DashboardWorker) updateDashNode(expTemplate map[string]interface{}, bag *m.DashboardBag, parentWidget *m.WidgetTemplate) {
	if dw.nodeHasPath(expTemplate) {
		dw.updateNodePath(expTemplate, bag, parentWidget)
	} else {
		dw.updateNodeExpression(expTemplate, bag, parentWidget)
	}
}

func (dw *DashboardWorker) nodeHasPath(node map[string]interface{}) bool {

	_, ok := node["metricPath"]

	return ok
}

func (dw *DashboardWorker) updateNodePath(expTemplate map[string]interface{}, dashBag *m.DashboardBag, parentWidget *m.WidgetTemplate) {
	expTemplate["metricPath"] = fmt.Sprintf(expTemplate["metricPath"].(string), dw.Bag.TierName, dashBag.Namespace, dashBag.TierName)
	scopeBlock := expTemplate["scopeEntity"].(map[string]interface{})
	scopeBlock["applicationName"] = dashBag.AppName
	scopeBlock["entityName"] = dashBag.TierName

	//drill-down
}

func (dw *DashboardWorker) updateNodeExpression(expTemplate map[string]interface{}, dashBag *m.DashboardBag, parentWidget *m.WidgetTemplate) {
	for i := 0; i < 2; i++ {
		expNodeName := fmt.Sprintf("expression%d", i+1)
		if expNode, ok := expTemplate[expNodeName]; ok {
			if dw.nodeHasPath(expNode.(map[string]interface{})) {
				dw.updateNodePath(expNode.(map[string]interface{}), dashBag, parentWidget)
			} else {
				dw.updateNodeExpression(expTemplate, dashBag, parentWidget)
			}
		}
	}
}
