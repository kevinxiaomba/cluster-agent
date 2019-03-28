package models

type DashboardType string

const (
	Cluster   DashboardType = "cluster"
	Tier      DashboardType = "tier"
	Node      DashboardType = "node"
	Namespace DashboardType = "namespace"
)

type DashboardBag struct {
	Type          DashboardType //cluster/tier/node/namespace
	ClusterName   string
	Namespace     string
	AppName       string
	TierName      string
	ClusterAppID  int
	ClusterTierID int
	ClusterNodeID int
	AppID         int
	TierID        int
	NodeID        int
	Pods          []PodSchema
	HeatNodes     []HeatNode
}

func NewDashboardBagTier(ns string, tierName string, pods []PodSchema) DashboardBag {
	return DashboardBag{Namespace: ns, TierName: tierName, Pods: pods, Type: Tier}
}

func NewDashboardBagCluster(nodes []HeatNode) DashboardBag {
	return DashboardBag{HeatNodes: nodes, Type: Cluster}
}
