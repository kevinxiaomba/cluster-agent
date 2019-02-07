package models

import (
	appd "appdynamics"
	"fmt"
)

type AppDMetricInterface interface {
	ShouldExcludeField(fieldName string) bool
	GetPath() string
	Unwrap() *map[string]interface{}
}

const RootPath string = "Server|Component:%s|Custom Metrics|Cluster Stats|"
const ALL string = "all"
const METRIC_SEPARATOR string = "|"
const METRIC_PATH_NODES string = "Nodes"
const METRIC_PATH_NAMESPACES string = "Namespaces"
const METRIC_PATH_APPS string = "Deployments"
const METRIC_PATH_CONT string = "Containers"
const METRIC_PATH_INSTANCES string = "Instances"
const METRIC_PATH_PORTS string = "Ports"
const METRIC_PATH_SERVICES string = "Services"
const METRIC_PATH_EVENTS string = "Events"
const METRIC_PATH_SERVICES_EP string = "Endpoints"
const METRIC_PATH_RQSPEC string = "QuotaSpecs"
const METRIC_PATH_RQUSED string = "QuotaUsed"

type AppDMetric struct {
	MetricName              string
	MetricValue             int64
	MetricPath              string
	MetricAlias             string
	MetricMultiplier        float64
	MetricAggregationType   string
	MetricTimeRollUpType    appd.RollupType
	MetricClusterRollUpType appd.ClusterRollupType
	MetricDelta             bool
}

type AppDMetricList struct {
	Items []AppDMetric
}

func NewAppDMetric(name string, val int64, path string) AppDMetric {
	p := fmt.Sprintf("%s%s", path, name)
	return AppDMetric{MetricName: name, MetricValue: val, MetricPath: p, MetricAggregationType: "OBSERVATION", MetricTimeRollUpType: appd.APPD_TIMEROLLUP_TYPE_CURRENT, MetricClusterRollUpType: appd.APPD_CLUSTERROLLUP_TYPE_INDIVIDUAL}
}

func (am AppDMetric) ToString() string {
	return fmt.Sprintf("Name: %s, Value: %d, Path: %s", am.MetricName, am.MetricValue, am.MetricPath)
}

func NewAppDMetricList() AppDMetricList {
	return AppDMetricList{}
}

func (l AppDMetricList) AddMetrics(obj AppDMetric) []AppDMetric {
	l.Items = append(l.Items, obj)
	return l.Items
}
