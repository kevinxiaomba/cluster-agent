package models

import (
	"fmt"

	"github.com/fatih/structs"
)

type ClusterPodMetrics struct {
	Path                string
	Metadata            map[string]AppDMetricMetadata
	Namespace           string
	Nodename            string
	PodCount            int64
	NamespaceCount      int64
	NamespaceNoQuotas   int64
	ServiceCount        int64
	EndpointCount       int64
	EPReadyCount        int64
	EPNotReadyCount     int64
	OrphanEndpoint      int64
	ExtServiceCount     int64
	Evictions           int64
	PodRestarts         int64
	PodRunning          int64
	PodFailed           int64
	PodPending          int64
	PendingTime         int64
	UpTime              int64
	ContainerCount      int64
	InitContainerCount  int64
	NoLimits            int64
	NoReadinessProbe    int64
	NoLivenessProbe     int64
	Privileged          int64
	PodStorageRequest   int64
	PodStorageLimit     int64
	StorageRequest      int64
	StorageCapacity     int64
	RequestCpu          int64
	RequestMemory       int64
	LimitCpu            int64
	LimitMemory         int64
	UseCpu              int64
	UseMemory           int64
	ConsumptionCpu      int64
	ConsumptionMem      int64
	MissingDependencies int64
	NoConnectivity      int64
	QuotasSpec          RQFields
	QuotasUsed          RQFields
}

func (cpm ClusterPodMetrics) GetPath() string {

	return cpm.Path
}

func (cpm ClusterPodMetrics) ShouldExcludeField(fieldName string) bool {
	if fieldName == "Nodename" || fieldName == "Namespace" || fieldName == "Path" || fieldName == "Metadata" || fieldName == "QuotasSpec" || fieldName == "QuotasUsed" {
		return true
	}
	return false
}

func (cpm ClusterPodMetrics) Unwrap() *map[string]interface{} {
	objMap := structs.Map(cpm)

	return &objMap
}

func (cpm ClusterPodMetrics) GetQuotaSpecMetrics() RQFields {

	path := fmt.Sprintf("%s%s%s", cpm.GetPath(), METRIC_PATH_RQSPEC, METRIC_SEPARATOR)
	cpm.QuotasSpec.Path = path
	return cpm.QuotasSpec
}

func (cpm ClusterPodMetrics) GetQuotaUsedMetrics() RQFields {

	path := fmt.Sprintf("%s%s%s", cpm.GetPath(), METRIC_PATH_RQUSED, METRIC_SEPARATOR)
	cpm.QuotasUsed.Path = path
	return cpm.QuotasUsed
}

func NewClusterPodMetrics(bag *AppDBag, ns string, node string) ClusterPodMetrics {
	p := RootPath
	if node != "" && node != ALL {
		p = fmt.Sprintf("%s%s%s%s%s", p, METRIC_PATH_NODES, METRIC_SEPARATOR, node, METRIC_SEPARATOR)
	} else if ns != "" && ns != ALL {
		p = fmt.Sprintf("%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns, METRIC_SEPARATOR)
	}
	return ClusterPodMetrics{Namespace: ns, Nodename: node, PodCount: 0, Evictions: 0,
		PodRestarts: 0, PodRunning: 0, PodFailed: 0, PodPending: 0, PendingTime: 0, UpTime: 0, ContainerCount: 0, InitContainerCount: 0,
		NoLimits: 0, NoReadinessProbe: 0, NoLivenessProbe: 0, Privileged: 0, RequestCpu: 0, RequestMemory: 0, LimitCpu: 0, LimitMemory: 0,
		UseCpu: 0, UseMemory: 0, PodStorageRequest: 0, PodStorageLimit: 0, StorageRequest: 0, StorageCapacity: 0, NamespaceCount: 0,
		NamespaceNoQuotas: 0, ServiceCount: 0, EndpointCount: 0, OrphanEndpoint: 0, EPReadyCount: 0, EPNotReadyCount: 0,
		ExtServiceCount: 0, MissingDependencies: 0, NoConnectivity: 0, ConsumptionCpu: 0, ConsumptionMem: 0, QuotasSpec: NewRQFields(), QuotasUsed: NewRQFields(), Path: p}
}

func NewClusterPodMetricsMetadata(bag *AppDBag, ns string, node string) ClusterPodMetrics {
	metrics := NewClusterPodMetrics(bag, ns, node)
	//	metrics.Metadata = buildAppMetadata(bag)
	return metrics
}
