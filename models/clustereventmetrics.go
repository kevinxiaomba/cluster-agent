package models

import (
	"fmt"
)

type ClusterEventMetrics struct {
	Path            string
	Metadata        map[string]AppDMetricMetadata
	Namespace       string
	EventCount      int64
	EventError      int64
	EventInfo       int64
	ScaleDowns      int64
	CrashLoops      int64
	QuotaViolations int64
	PodIssues       int64
	PodKills        int64
	EvictionThreats int64
	ImagePullErrors int64
	ImagePulls      int64
	StorageIssues   int64
}

func NewClusterEventMetrics(bag *AppDBag, ns string, node string) ClusterEventMetrics {
	p := RootPath
	if ns != "" && ns != ALL {
		p = fmt.Sprintf("%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns, METRIC_SEPARATOR)
	}
	return ClusterEventMetrics{Namespace: ns, Path: p, EventCount: 0, EventError: 0, EventInfo: 0, ScaleDowns: 0, CrashLoops: 0, QuotaViolations: 0, PodIssues: 0,
		PodKills: 0, EvictionThreats: 0, ImagePullErrors: 0, ImagePulls: 0, StorageIssues: 0}
}

func NewClusterEventMetricsMetadata(bag *AppDBag, ns string, node string) ClusterEventMetrics {
	metrics := NewClusterEventMetrics(bag, ns, node)
	metrics.Metadata = buildJobMetadata(bag)
	return metrics
}

func buildEventMetadata(bag *AppDBag) map[string]AppDMetricMetadata {
	pathBase := "Application Infrastructure Performance|%s|Custom Metrics|Cluster Stats|"

	meta := make(map[string]AppDMetricMetadata, 22)
	path := pathBase + "EventCount"
	meta[path] = NewAppDMetricMetadata("PodsCount", bag.PodSchemaName, path, "select * from "+bag.JobSchemaName)

	return meta
}
