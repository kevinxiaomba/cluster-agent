package models

import (
	"fmt"

	"github.com/fatih/structs"
)

type ClusterDeployMetrics struct {
	Path                      string
	Namespace                 string
	DeployCount               int64
	DeployReplicas            int64
	DeployReplicasUnAvailable int64
	DeployCollisionCount      int64
}

func (cpm ClusterDeployMetrics) GetPath() string {

	return cpm.Path
}

func (cpm ClusterDeployMetrics) ShouldExcludeField(fieldName string) bool {
	if fieldName == "Namespace" || fieldName == "Path" {
		return true
	}
	return false
}

func (cpm ClusterDeployMetrics) Unwrap() *map[string]interface{} {
	objMap := structs.Map(cpm)

	return &objMap
}

func NewClusterDeployMetrics(bag *AppDBag, ns string) ClusterDeployMetrics {
	p := RootPath
	if ns != "" && ns != ALL {
		p = fmt.Sprintf("%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns, METRIC_SEPARATOR)
	}
	return ClusterDeployMetrics{Namespace: ns, DeployCount: 0, DeployReplicas: 0, DeployReplicasUnAvailable: 0, DeployCollisionCount: 0,
		Path: p}
}
