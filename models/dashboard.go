package models

type ScopeEntity struct {
	Subtype           *string `json: "subtype"`
	EntityType        string  `json: "entityType"`
	EntityName        string  `json: "entityName"`
	ScopingEntityType *string `json: "scopingEntityType"`
	ScopingEntityName *string `json: "scopingEntityName"`
	ApplicationName   string  `json: "applicationName"`
}

type MetricExpressionTemplate struct {
	InputMetricPath      *string           `json: "inputMetricPath, omitempty"`
	DisplayName          string            `json: "displayName, omitempty"`
	FunctionType         string            `json: "functionType, omitempty"`
	InputMetricText      bool              `json: "inputMetricText, omitempty"`
	MetricPath           string            `json: "metricPath, omitempty"`
	ScopeEntity          ScopeEntity       `json: "scopeEntity, omitempty"`
	MetricExpressionType string            `json: "metricExpressionType, omitempty"`
	Operator             Operator          `json: "operator, omitempty"`
	Expression1          MetricExpression  `json: "expression1, omitempty"`
	Expression2          MetricExpression2 `json: "expression2, omitempty"`
}

type MetricExpression struct {
	InputMetricPath      *string           `json: "inputMetricPath"`
	DisplayName          string            `json: "displayName"`
	FunctionType         string            `json: "functionType"`
	InputMetricText      bool              `json: "inputMetricText"`
	MetricPath           string            `json: "metricPath"`
	ScopeEntity          ScopeEntity       `json: "scopeEntity"`
	MetricExpressionType string            `json: "metricExpressionType"`
	LiteralValue         float64           `json: "literalValue"`
	Operator             Operator          `json: "operator, omitempty"`
	Expression2          MetricExpression2 `json: "expression2, omitempty"`
}

type MetricExpression2 struct {
	InputMetricPath      *string     `json: "inputMetricPath"`
	DisplayName          string      `json: "displayName"`
	FunctionType         string      `json: "functionType"`
	InputMetricText      bool        `json: "inputMetricText"`
	MetricPath           string      `json: "metricPath"`
	ScopeEntity          ScopeEntity `json: "scopeEntity"`
	MetricExpressionType string      `json: "metricExpressionType"`
	LiteralValue         float64     `json: "literalValue"`
	Operator             Operator    `json: "operator, omitempty"`
}

type Operator struct {
	Type string `json: "type"`
}

type MetricMatchCriteriaTemplate struct {
	EvaluationScopeType           *string                `json:"evaluationScopeType"`
	RollupMetricData              bool                   `json:"rollupMetricData"`
	MaxResults                    int                    `json:"maxResults"`
	EntityMatchCriteria           *string                `json:"entityMatchCriteria"`
	UseActiveBaseline             bool                   `json:"useActiveBaseline"`
	SortResultsAscending          bool                   `json:"sortResultsAscending"`
	MetricExpressionTemplate      map[string]interface{} `json:"metricExpressionTemplate" `
	MetricDisplayNameCustomFormat string                 `json:"metricDisplayNameCustomFormat"`
	ExpressionString              string                 `json:"expressionString"`
	BaselineName                  *string                `json:"baselineName"`
	ApplicationName               string                 `json:"applicationName"`
	MetricDisplayNameStyle        string                 `json:"metricDisplayNameStyle"`
}

type ColorPalette struct {
	Colors []int `json: "colors"`
}

type DataSeriesTemplate struct {
	MetricType                  string                      `json:"metricType"`
	SeriesType                  string                      `json:"seriesType"`
	AxisPosition                *string                     `json:"axisPosition"`
	Name                        string                      `json:"name"`
	ShowRawMetricName           bool                        `json:"showRawMetricName"`
	ColorPalette                ColorPalette                `json:"colorPalette"`
	MetricMatchCriteriaTemplate MetricMatchCriteriaTemplate `json:"metricMatchCriteriaTemplate"`
}

type WidgetTemplate struct {
	UseMetricBrowserAsDrillDown bool                 `json:"useMetricBrowserAsDrillDown"`
	BorderColor                 int                  `json:"borderColor"`
	ShowValues                  bool                 `json:"showValues"`
	Color                       int                  `json:"color"`
	CompactMode                 bool                 `json:"compactMode"`
	ShowTimeRange               bool                 `json:"showTimeRange"`
	LegendColumnCount           *int                 `json:"legendColumnCount"`
	ShowLegend                  bool                 `json:"showLegend"`
	Description                 *string              `json:"description"`
	BackgroundAlpha             int                  `json:"backgroundAlpha"`
	FormatNumber                bool                 `json:"formatNumber"`
	Title                       *string              `json:"title"`
	DataSeriesTemplates         []DataSeriesTemplate `json:"dataSeriesTemplates"`
	MinHeight                   int                  `json:"minHeight"`
	BorderThickness             int                  `json:"borderThickness"`
	UseAutomaticFontSize        bool                 `json:"useAutomaticFontSize"`
	RemoveZeros                 bool                 `json:"removeZeros"`
	DrillDownUrl                *string              `json:"drillDownUrl"`
	StartTime                   *int                 `json:"startTime"`
	Text                        string               `json:"text"`
	DrillDownActionType         *string              `json:"drillDownActionType"`
	Height                      int                  `json:"height"`
	NumDecimals                 int                  `json:"numDecimals"`
	BackgroundColor             int                  `json:"backgroundColor"`
	Margin                      int                  `json:"margin"`
	TextAlign                   string               `json:"textAlign"`
	MinWidth                    int                  `json:"minWidth"`
	PropertiesMap               *string              `json:"propertiesMap"`
	Label                       *string              `json:"label"`
	RenderIn3D                  bool                 `json:"renderIn3D"`
	MinutesBeforeAnchorTime     int                  `json:"minutesBeforeAnchorTime"`
	WidgetType                  string               `json:"widgetType"`
	BackgroundColorsStr         string               `json:"backgroundColorsStr"`
	BorderEnabled               bool                 `json:"borderEnabled"`
	Width                       int                  `json:"width"`
	X                           int                  `json:"x"`
	BackgroundColors            *string              `json:"backgroundColors"`
	IsGlobal                    bool                 `json:"isGlobal"`
	Y                           int                  `json:"y"`
	FontSize                    int                  `json:"fontSize"`
	LegendPosition              *string              `json:"legendPosition"`
	EndTime                     *int                 `json:"endTime"`
	CustomVerticalAxisMin       *int                 `json:"customVerticalAxisMin"`
	CustomVerticalAxisMax       *int                 `json:"customVerticalAxisMax"`
	MultipleYAxis               *string              `json:"multipleYAxis"`
	StaticThresholdList         []StaticThreshold    `json:"staticThresholdList"`
	ShowAllTooltips             bool                 `json:"showAllTooltips"`
	AxisType                    string               `json:"axisType"`
	HorizontalAxisLabel         *string              `json:"horizontalAxisLabel"`
	VerticalAxisLabel           *string              `json:"verticalAxisLabel"`
	ShowEvents                  *string              `json:"showEvents"`
	HideHorizontalAxis          *string              `json:"hideHorizontalAxis"`
	StackMode                   bool                 `json:"stackMode"`
	InterpolateDataGaps         bool                 `json:"interpolateDataGaps"`
	EventFilterTemplate         *string              `json:"eventFilterTemplate"`
}

type StaticThreshold struct {
}

type Dashboard struct {
	Name                      string           `json:"name"`
	Template                  bool             `json:"template"`
	TemplateEntityType        string           `json:"templateEntityType"`
	BackgroundColor           int              `json:"backgroundColor"`
	SchemaVersion             *string          `json:"schemaVersion"`
	Color                     int              `json:"color"`
	EndDate                   *int             `json:"endDate"`
	RefreshInterval           int              `json:"refreshInterval"`
	WidgetTemplates           []WidgetTemplate `json:"widgetTemplates"`
	Description               *string          `json:"description"`
	CanvasType                string           `json:"canvasType"`
	MinutesBeforeAnchorTime   int              `json:"minutesBeforeAnchorTime"`
	DashboardFormatVersion    string           `json:"dashboardFormatVersion"`
	Width                     int              `json:"width"`
	AssociatedEntityTemplates *string          `json:"associatedEntityTemplates"`
	LayoutType                string           `json:"layoutType"`
	WarRoom                   bool             `json:"warRoom"`
	Properties                *string          `json:"properties"`
	StartDate                 *int             `json:"startDate"`
	Height                    int              `json:"height"`
}
