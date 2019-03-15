package models

import (
	"fmt"

	"github.com/fatih/structs"
)

type ClusterRsMetrics struct {
	Path                  string
	Namespace             string
	RsCount               int64
	RsReplicas            int64
	RsReplicasAvailable   int64
	RsReplicasUnAvailable int64
}

func (cpm ClusterRsMetrics) GetPath() string {

	return cpm.Path
}

func (cpm ClusterRsMetrics) ShouldExcludeField(fieldName string) bool {
	if fieldName == "Namespace" || fieldName == "Path" {
		return true
	}
	return false
}

func (cpm ClusterRsMetrics) Unwrap() *map[string]interface{} {
	objMap := structs.Map(cpm)

	return &objMap
}

func NewClusterRsMetrics(bag *AppDBag, ns string) ClusterRsMetrics {
	p := RootPath
	if ns != "" && ns != ALL {
		p = fmt.Sprintf("%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns, METRIC_SEPARATOR)
	}
	return ClusterRsMetrics{Namespace: ns, RsCount: 0, RsReplicas: 0, RsReplicasAvailable: 0, RsReplicasUnAvailable: 0,
		Path: p}
}
