package models

import (
	"fmt"

	"github.com/fatih/structs"
)

type ClusterAppMetrics struct {
	Path                string
	Metadata            map[string]AppDMetricMetadata
	Namespace           string
	TierName            string
	PodCount            int64
	Privileged          int64
	Evictions           int64
	PodRestarts         int64
	PodRunning          int64
	PodFailed           int64
	PodPending          int64
	PendingTime         int64
	UpTime              int64
	ContainerCount      int64
	InitContainerCount  int64
	RequestCpu          int64
	RequestMemory       int64
	LimitCpu            int64
	LimitMemory         int64
	UseCpu              int64
	UseMemory           int64
	ConsumptionCpu      int64
	ConsumptionMem      int64
	NoLimits            int64
	NoReadinessProbe    int64
	NoLivenessProbe     int64
	MissingDependencies int64
	NoConnectivity      int64
	Services            []ClusterServiceMetrics
	QuotasSpec          RQFields
	QuotasUsed          RQFields
}

type ClusterServiceMetrics struct {
	Path           string
	Metadata       map[string]AppDMetricMetadata
	Namespace      string
	TierName       string
	ServiceName    string
	Accessible     int64 //0 or 1
	EndpointsTotal int64
	Endpoints      map[string]ClusterServiceEPMetrics
}

type ClusterServiceEPMetrics struct {
	Path              string
	Metadata          map[string]AppDMetricMetadata
	Namespace         string
	TierName          string
	ServiceName       string
	EndpointName      string
	EndpointsReady    int64
	EndpointsNotReady int64
}

func (cpm ClusterAppMetrics) GetPath() string {

	return cpm.Path
}

func (cpm ClusterServiceMetrics) GetPath() string {

	return cpm.Path
}

func (cpm ClusterServiceEPMetrics) GetPath() string {

	return cpm.Path
}

func (cpm ClusterAppMetrics) Unwrap() *map[string]interface{} {
	objMap := structs.Map(cpm)
	return &objMap
}

func (cpm ClusterAppMetrics) GetQuotaSpecMetrics() RQFields {

	path := fmt.Sprintf("%s%s%s", cpm.GetPath(), METRIC_PATH_RQSPEC, METRIC_SEPARATOR)
	cpm.QuotasSpec.Path = path
	return cpm.QuotasSpec
}

func (cpm ClusterAppMetrics) GetQuotaUsedMetrics() RQFields {

	path := fmt.Sprintf("%s%s%s", cpm.GetPath(), METRIC_PATH_RQUSED, METRIC_SEPARATOR)
	cpm.QuotasUsed.Path = path
	return cpm.QuotasUsed
}

func (cpm ClusterServiceMetrics) Unwrap() *map[string]interface{} {
	objMap := structs.Map(cpm)

	return &objMap
}

func (cpm ClusterServiceEPMetrics) Unwrap() *map[string]interface{} {
	objMap := structs.Map(cpm)

	return &objMap
}

func (cpm ClusterAppMetrics) ShouldExcludeField(fieldName string) bool {
	if fieldName == "TierName" || fieldName == "Namespace" || fieldName == "Path" || fieldName == "Metadata" || fieldName == "Services" || fieldName == "QuotasSpec" || fieldName == "QuotasUsed" {
		return true
	}
	return false
}

func (cpm ClusterServiceMetrics) ShouldExcludeField(fieldName string) bool {
	if fieldName == "TierName" || fieldName == "Namespace" || fieldName == "Path" || fieldName == "Metadata" || fieldName == "ServiceName" || fieldName == "Endpoints" {
		return true
	}
	return false
}

func (cpm ClusterServiceEPMetrics) ShouldExcludeField(fieldName string) bool {
	if fieldName == "TierName" || fieldName == "Namespace" || fieldName == "Path" || fieldName == "Metadata" || fieldName == "ServiceName" || fieldName == "EndpointName" {
		return true
	}
	return false
}

func NewClusterAppMetrics(bag *AppDBag, podObject *PodSchema) ClusterAppMetrics {
	p := RootPath
	p = fmt.Sprintf("%s%s%s%s%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, podObject.Namespace, METRIC_SEPARATOR, METRIC_PATH_APPS, METRIC_SEPARATOR, podObject.Owner, METRIC_SEPARATOR)

	appMetrics := ClusterAppMetrics{Namespace: podObject.Namespace, TierName: podObject.Owner, Privileged: 0, PodCount: 0, Evictions: 0,
		PodRestarts: 0, PodRunning: 0, PodFailed: 0, PodPending: 0, PendingTime: 0, UpTime: 0, ContainerCount: 0, InitContainerCount: 0,
		RequestCpu: 0, RequestMemory: 0, LimitCpu: 0, LimitMemory: 0, UseCpu: 0, UseMemory: 0,
		ConsumptionCpu: 0, ConsumptionMem: 0, NoLimits: 0, NoReadinessProbe: 0, NoLivenessProbe: 0,
		MissingDependencies: 0, NoConnectivity: 0, QuotasSpec: NewRQFields(), QuotasUsed: NewRQFields(), Path: p}

	for _, svc := range podObject.Services {
		svcMetrics := NewClusterServiceMetrics(bag, podObject.Namespace, podObject.Owner, &svc)
		appMetrics.Services = append(appMetrics.Services, svcMetrics)
	}

	return appMetrics
}

func NewClusterServiceMetrics(bag *AppDBag, ns string, tierName string, svcSchema *ServiceSchema) ClusterServiceMetrics {
	p := RootPath
	p = fmt.Sprintf("%s%s%s%s%s%s%s%s%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns, METRIC_SEPARATOR, METRIC_PATH_APPS, METRIC_SEPARATOR, tierName, METRIC_SEPARATOR, METRIC_PATH_SERVICES, METRIC_SEPARATOR, svcSchema.Name, METRIC_SEPARATOR)

	metrics := ClusterServiceMetrics{Namespace: ns, TierName: tierName, ServiceName: svcSchema.Name, Path: p, Accessible: 0, EndpointsTotal: 0, Endpoints: make(map[string]ClusterServiceEPMetrics)}
	if svcSchema.IsAccessible {
		metrics.Accessible = 1
	}
	total, ready, notready := svcSchema.GetEndPointsStats()
	metrics.EndpointsTotal = int64(total)

	for epName, val := range ready {
		epm := NewClusterServiceEPMetrics(bag, ns, tierName, svcSchema, epName)
		epm.EndpointsReady = int64(val)
		metrics.Endpoints[epName] = epm
	}
	for epName, val := range notready {
		epm, ok := metrics.Endpoints[epName]
		if !ok {
			epm = NewClusterServiceEPMetrics(bag, ns, tierName, svcSchema, epName)
		}
		epm.EndpointsNotReady = int64(val)
		metrics.Endpoints[epName] = epm
	}

	return metrics
}

func NewClusterServiceEPMetrics(bag *AppDBag, ns string, tierName string, svcSchema *ServiceSchema, endpointName string) ClusterServiceEPMetrics {
	p := RootPath
	p = fmt.Sprintf("%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns, METRIC_SEPARATOR, METRIC_PATH_APPS, METRIC_SEPARATOR, tierName, METRIC_SEPARATOR, METRIC_PATH_SERVICES, METRIC_SEPARATOR, svcSchema.Name, METRIC_SEPARATOR, METRIC_PATH_SERVICES_EP, METRIC_SEPARATOR, endpointName, METRIC_SEPARATOR)

	metrics := ClusterServiceEPMetrics{Namespace: ns, TierName: tierName, ServiceName: svcSchema.Name, EndpointName: endpointName, Path: p, EndpointsReady: 0, EndpointsNotReady: 0}

	return metrics
}

func NewClusterAppMetricsMetadata(bag *AppDBag, podObject *PodSchema) ClusterAppMetrics {
	metrics := NewClusterAppMetrics(bag, podObject)
	metrics.Metadata = buildMetadata(bag)
	return metrics
}

func buildMetadata(bag *AppDBag) map[string]AppDMetricMetadata {
	pathBase := "Application Infrastructure Performance|%s|Custom Metrics|Cluster Stats|"

	meta := make(map[string]AppDMetricMetadata, 22)
	path := pathBase + "PodCount"
	meta[path] = NewAppDMetricMetadata("PodCount", bag.PodSchemaName, path, "select * from "+bag.PodSchemaName)

	return meta
}
