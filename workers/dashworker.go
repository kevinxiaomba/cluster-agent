package workers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	app "github.com/sjeltuhin/clusterAgent/appd"
	m "github.com/sjeltuhin/clusterAgent/models"
)

const (
	WIDGET_GAP   float64 = 10
	FULL_CLUSTER string  = "/k8s_dashboard_template.json"
	DEPLOY_BASE  string  = "/base_deploy_template.json"
	TIER_SUMMARY string  = "/tier_stats_widget.json"
	BACKGROUND   string  = "/background.json"
)

type DashboardWorker struct {
	Bag            *m.AppDBag
	Logger         *log.Logger
	AppdController *app.ControllerClient
}

func NewDashboardWorker(bag *m.AppDBag, l *log.Logger, appController *app.ControllerClient) DashboardWorker {
	return DashboardWorker{bag, l, appController}
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

func (dw *DashboardWorker) createDashboard(dashboard *m.Dashboard) (*m.Dashboard, error) {
	var dashObject *m.Dashboard = nil
	data, err := json.Marshal(dashboard)
	if err != nil {
		return dashObject, fmt.Errorf("Unable to create dashboard %s. %v", dashboard.Name, err)
	}
	rc := app.NewRestClient(dw.Bag, dw.Logger)
	saved, errSave := rc.CallAppDController("restui/dashboards/createDashboard", "POST", data)
	if errSave != nil {

		return dashObject, fmt.Errorf("Unable to create dashaboard %s. %v\n", dashboard.Name, errSave)
	}

	e := json.Unmarshal(saved, &dashObject)
	if e != nil {
		fmt.Printf("Unable to deserialize the new dashaboards %v\n", e)
		return dashObject, e
	}

	return dashObject, nil
}

func (dw *DashboardWorker) saveDashboard(dashboard *m.Dashboard) error {

	data, err := json.Marshal(dashboard)
	if err != nil {
		return fmt.Errorf("Unable to save dashboard %s. %v", dashboard.Name, err)
	}
	rc := app.NewRestClient(dw.Bag, dw.Logger)
	_, errSave := rc.CallAppDController("restui/dashboards/updateDashboard", "POST", data)
	if errSave != nil {

		return fmt.Errorf("Unable to save dashaboard %s. %v\n", dashboard.Name, errSave)
	}

	return nil
}

func (dw *DashboardWorker) updateWidget(widget *map[string]interface{}) error {
	data, err := json.Marshal(widget)
	if err != nil {
		return fmt.Errorf("Unable to save widget %s. %v", (*widget)["type"], err)
	}
	rc := app.NewRestClient(dw.Bag, dw.Logger)
	_, errSave := rc.CallAppDController("restui/dashboards/updateWidget", "POST", data)
	if errSave != nil {
		return fmt.Errorf("Unable to update widget %s. %v\n", (*widget)["type"], errSave)
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

func (dw *DashboardWorker) loadWidgetTemplate(templateFile string) (*[]map[string]interface{}, error, bool) {
	var widget *[]map[string]interface{} = nil

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

	_, errCreate := dw.createDashboardFromFile(dashObject, "/GENERATED.json")

	return errCreate
}

func (dw *DashboardWorker) createDashboardFromFile(dashboard *m.Dashboard, genPath string) (*m.Dashboard, error) {

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
	bytes, errM := json.Marshal(raw)
	if errM != nil {
		return nil, fmt.Errorf("Unable to extract dashaboard node. %v\n", errM)
	}
	var dashObj m.Dashboard
	eD := json.Unmarshal(bytes, &dashObj)
	if eD != nil {
		return nil, fmt.Errorf("Unable to create dashaboard. Unable to deserialize the new dashaboards. %v\n", eD)
	}

	fmt.Printf("Dashboard saved %.0f %s\n", dashObj.ID, dashObj.Name)
	return dashboard, nil
}

func (dw *DashboardWorker) saveTemplate(dashboard *m.Dashboard, genPath string) (*string, error) {
	generated, errMar := json.Marshal(dashboard)
	if errMar != nil {
		return nil, fmt.Errorf("Unable to save template. %v", errMar)
	}
	dir := filepath.Dir(dw.Bag.DashboardTemplatePath)
	fileForUpload := dir + genPath
	fmt.Printf("About to save template file %s...\n", fileForUpload)
	errSave := ioutil.WriteFile(fileForUpload, generated, 0644)
	if errSave != nil {
		return nil, fmt.Errorf("Issues when writing generated template. %v", errMar)
	}
	fmt.Printf("File %s... saved\n", fileForUpload)
	return &fileForUpload, nil
}

func (dw *DashboardWorker) updateDashboard(dashObject *m.Dashboard, metricsMetadata map[string]m.AppDMetricMetadata) {
	for _, w := range dashObject.Widgets {
		widget := w
		dsTemplates, ok := widget["dataSeriesTemplates"]
		if ok {
			for _, t := range dsTemplates.(map[string]interface{}) {
				mmTemplate, ok := t.(map[string]interface{})["metricMatchCriteriaTemplate"]
				if ok {
					exp, ok := mmTemplate.(map[string]interface{})["metricExpressionTemplate"]
					if ok {
						dw.updateMetricNode(exp.(map[string]interface{}), metricsMetadata, &widget)
					}
				}
			}
		}
	}
}

func (dw *DashboardWorker) updateMetricNode(expTemplate map[string]interface{}, metricsMetadata map[string]m.AppDMetricMetadata, parentWidget *map[string]interface{}) {
	metricObj := dw.getMatchingMetrics(expTemplate, metricsMetadata)
	if metricObj != nil {
		//metrics path matches. update
		dw.updateMetricPath(expTemplate, metricObj, parentWidget)
	} else {
		dw.updateMetricExpression(expTemplate, metricsMetadata, parentWidget)
	}
}

func (dw *DashboardWorker) updateMetricPath(expTemplate map[string]interface{}, metricObj *m.AppDMetricMetadata, parentWidget *map[string]interface{}) {
	expTemplate["metricPath"] = fmt.Sprintf(metricObj.Path, dw.Bag.TierName)
	scopeBlock := expTemplate["scopeEntity"].(map[string]interface{})
	scopeBlock["applicationName"] = dw.Bag.AppName
	scopeBlock["entityName"] = dw.Bag.TierName

}

func (dw *DashboardWorker) updateMetricExpression(expTemplate map[string]interface{}, metricsMetadata map[string]m.AppDMetricMetadata, parentWidget *map[string]interface{}) {
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

func (dw *DashboardWorker) validateTierDashboardBag(bag *m.DashboardBag) error {
	msg := ""
	if bag.ClusterAppID == 0 {
		msg = "Missing Agent App ID."
	}
	if bag.AppID == 0 {
		msg += " APM App ID is missing."
	}
	if bag.TierID == 0 {
		msg += " APM Tier ID is missing."
	}
	if bag.NodeID == 0 {
		msg += " APM Node ID is missing."
	}
	if msg != "" {
		return fmt.Errorf("Dashboard validation failed. %s", msg)
	}
	return nil
}

//dynamic dashboard
func (dw *DashboardWorker) updateTierDashboard(bag *m.DashboardBag) error {
	valErr := dw.validateTierDashboardBag(bag)
	if valErr != nil {
		dw.Logger.Printf("Dashboard parameters are invalid. %v", valErr)
		return valErr
	}

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
			dashboard, err = dw.createDashboard(dashboard)
			fmt.Printf("Create dashboard result. Error: %v, \n", err)
			if err != nil {
				return err
			}
		}
	}
	if dashboard == nil {
		return fmt.Errorf("Unable to build tier Dashboard. Weird things do happen...")
	}
	//by this time we should have dashboard object with id
	//save base template with id, strip widgets
	dashboard.Widgets = []map[string]interface{}{}
	_, errSave := dw.saveTemplate(dashboard, fileName)
	if errSave != nil {
		fmt.Printf("Issues when saving template for dashboard %s/%s\n", bag.Namespace, bag.TierName)
	}
	updated, errGen := dw.generateDeploymentDashboard(dashboard, bag)
	fmt.Printf("generateDeploymentDashboard. %v\n", errGen)
	if errGen != nil {
		return fmt.Errorf("Issues when generating template for deployment dashboard %s, %s. %v\n", bag.Namespace, bag.TierName, err)
	}
	dw.saveTemplate(updated, fileName)
	fmt.Printf("Saving dashboard %.0f, %s\n", updated.ID, updated.Name)
	errSaveDash := dw.saveDashboard(updated)
	fmt.Printf("saveDashboard. %v\n", errSaveDash)

	return errSaveDash
}

func (dw *DashboardWorker) generateDeploymentDashboard(dashboard *m.Dashboard, bag *m.DashboardBag) (*m.Dashboard, error) {
	//	dashboard.WidgetTemplates = []m.WidgetTemplate{}
	fmt.Printf("generateDeploymentDashboard %s/%s\n", bag.Namespace, bag.TierName)
	d, err := dw.addBackground(dashboard, bag)
	if err != nil {
		fmt.Printf("Error when adding background to dash %s. %v\n", dashboard.Name, err)
		return nil, err
	}
	d, err = dw.addDeploymentSummaryWidget(dashboard, bag)
	if err != nil {
		fmt.Printf("Error when adding tier summary to dash %s. %v\n", dashboard.Name, err)
		return nil, err
	}
	fmt.Printf("Added %d widgets to dash %.0f, %s\n", len(d.Widgets), d.ID, d.Name)
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
	backgroundWidet["height"] = dashboard.Height - WIDGET_GAP
	backgroundWidet["dashboardId"] = dashboard.ID

	dashboard.Widgets = append(dashboard.Widgets, backgroundWidet)
	fmt.Printf("Added background %.0f x %.0f to dash %s\n", backgroundWidet["width"], backgroundWidet["weight"], dashboard.Name)
	return dashboard, nil
}

func (dw *DashboardWorker) addDeploymentSummaryWidget(dashboard *m.Dashboard, bag *m.DashboardBag) (*m.Dashboard, error) {
	widgetList, err, exists := dw.loadWidgetTemplate(TIER_SUMMARY)
	fmt.Printf("Loaded tier summary  %v. %t. Num widgets: %d\n", err, exists, len(*widgetList))
	if err != nil && exists {
		return nil, fmt.Errorf("Tier summary template exists, but cannot be loaded. %v\n", err)
	}
	if !exists {
		return nil, fmt.Errorf("Tier summary template does not exist, skipping dashboard for deployment %s/%s. %v\n", bag.Namespace, bag.TierName, err)
	}
	fmt.Printf("Adding tier summary to dash %s\n", dashboard.Name)
	var maxY float64 = 0
	for _, widget := range *widgetList {
		widget["id"] = 0
		appName, tierName := dw.GetEntityInfo(&widget, bag)
		fmt.Printf("Widget info: %s %s\n", appName, tierName)
		y := widget["y"].(float64)
		height := widget["height"].(float64)

		if y+height > maxY {
			maxY = y + height
		}

		widgetTitle, ok := widget["title"]
		if ok && widgetTitle != nil && strings.Contains(widgetTitle.(string), "%APP_TIER_NAME%") {
			widget["title"] = strings.Replace(widgetTitle.(string), "%APP_TIER_NAME%", bag.TierName, 1)
		}
		appID, okApp := widget["applicationId"]
		if okApp && appID != nil && strings.Contains(appID.(string), "%APP_ID%") {
			widget["applicationId"] = bag.AppID
		}

		entityIDs, hasEntityIDs := widget["entityIds"]
		if hasEntityIDs && entityIDs != nil && strings.Contains(entityIDs.(string), "%APP_TIER_ID%") {
			widget["entityIds"] = []int{bag.TierID}
		}

		dsTemplates, ok := widget["widgetsMetricMatchCriterias"]
		if ok && dsTemplates != nil {
			for _, t := range dsTemplates.([]interface{}) {
				if t != nil {
					mm, ok := t.(map[string]interface{})["metricMatchCriteria"]
					if ok && mm != nil {
						mmBlock := mm.(map[string]interface{})
						mmBlock["applicationId"] = bag.ClusterAppID
						exp, ok := mmBlock["metricExpression"]
						if ok && exp != nil {
							dw.updateMetricDefinition(exp.(map[string]interface{}), bag, &widget)
						}
					}
				}
			}
		}
	}

	dashboard.Height = maxY + WIDGET_GAP
	dashboard.Widgets = *widgetList
	fmt.Printf("Adjusted dashboard dimensions %d x %d\n", dashboard.Width, dashboard.Height)
	//adjust the background height
	dashboard.Widgets[0]["height"] = dashboard.Height
	fmt.Printf("Adjusted background dimensions %.0f x %.0f\n", dashboard.Widgets[0]["width"], dashboard.Widgets[0]["height"])
	fmt.Printf("Added tier summary to to dash %s\n", dashboard.Name)
	return dashboard, nil
}

func (dw *DashboardWorker) updateMetricDefinition(expTemplate map[string]interface{}, bag *m.DashboardBag, parentWidget *map[string]interface{}) {
	ok, definition := dw.nodeHasDefinition(expTemplate)
	if ok {
		dw.updateMetricName(definition, bag, parentWidget)
	} else {
		dw.updateMetricsExpression(expTemplate, bag, parentWidget)
	}
}

func (dw *DashboardWorker) nodeHasDefinition(node map[string]interface{}) (bool, map[string]interface{}) {

	definition, ok := node["metricDefinition"]
	if ok && definition != nil {
		definitionObj := definition.(map[string]interface{})
		_, ok = definitionObj["logicalMetricName"]
		return ok, definitionObj
	}
	return ok && definition != nil, nil
}

func (dw *DashboardWorker) updateMetricName(expTemplate map[string]interface{}, dashBag *m.DashboardBag, parentWidget *map[string]interface{}) {
	if expTemplate["logicalMetricName"] == nil {
		return
	}
	metricPath := fmt.Sprintf(expTemplate["logicalMetricName"].(string), dw.Bag.TierName, dashBag.Namespace, dashBag.TierName)
	metricID, err := dw.AppdController.GetMetricID(metricPath)
	if err != nil {
		dw.Logger.Printf("Cannot get metric ID for %s. %v\n", metricPath, err)
	} else {
		expTemplate["metricId"] = metricID
	}
	expTemplate["logicalMetricName"] = metricPath
	scopeBlock := expTemplate["scope"].(map[string]interface{})
	scopeBlock["entityId"] = dashBag.ClusterTierID
}

func (dw *DashboardWorker) updateMetricsExpression(expTemplate map[string]interface{}, dashBag *m.DashboardBag, parentWidget *map[string]interface{}) {
	for i := 0; i < 2; i++ {
		expNodeName := fmt.Sprintf("expression%d", i+1)
		if expNode, ok := expTemplate[expNodeName]; ok {
			hasDefinition, definition := dw.nodeHasDefinition(expNode.(map[string]interface{}))
			if hasDefinition {
				dw.updateMetricName(definition, dashBag, parentWidget)
			} else {
				dw.updateMetricsExpression(expNode.(map[string]interface{}), dashBag, parentWidget)
			}
		}
	}
}

//templates

func (dw *DashboardWorker) updateDashNode(expTemplate map[string]interface{}, bag *m.DashboardBag, parentWidget *map[string]interface{}) {
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

func (dw *DashboardWorker) updateNodePath(expTemplate map[string]interface{}, dashBag *m.DashboardBag, parentWidget *map[string]interface{}) {
	expTemplate["metricPath"] = fmt.Sprintf(expTemplate["metricPath"].(string), dw.Bag.TierName, dashBag.Namespace, dashBag.TierName)
	scopeBlock := expTemplate["scopeEntity"].(map[string]interface{})
	appName, tierName := dw.GetEntityInfo(parentWidget, dashBag)
	scopeBlock["applicationName"] = appName
	scopeBlock["entityName"] = tierName

	//drill-down
}

func (dw *DashboardWorker) updateNodeExpression(expTemplate map[string]interface{}, dashBag *m.DashboardBag, parentWidget *map[string]interface{}) {
	for i := 0; i < 2; i++ {
		expNodeName := fmt.Sprintf("expression%d", i+1)
		if expNode, ok := expTemplate[expNodeName]; ok {
			if dw.nodeHasPath(expNode.(map[string]interface{})) {
				dw.updateNodePath(expNode.(map[string]interface{}), dashBag, parentWidget)
			} else {
				dw.updateNodeExpression(expNode.(map[string]interface{}), dashBag, parentWidget)
			}
		}
	}
}

func (dw *DashboardWorker) GetEntityInfo(widget *map[string]interface{}, dashBag *m.DashboardBag) (string, string) {
	label, ok := (*widget)["label"]
	isAPMWidget := ok && (label == "APM")
	appName := dw.Bag.AppName
	tierName := dw.Bag.TierName
	if isAPMWidget {
		appName = dashBag.AppName
		tierName = dashBag.TierName
	}
	return appName, tierName
}
