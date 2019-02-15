package models

type DashboardBag struct {
	ClusterName   string
	Namespace     string
	AppName       string
	TierName      string
	ClusterAppID  int
	ClusterTierID int
	AppID         int
	TierID        int
	NodeID        int
	Pods          []PodSchema
}

func NewDashboardBag(ns string, tierName string, pods []PodSchema) DashboardBag {
	return DashboardBag{Namespace: ns, TierName: tierName, Pods: pods}
}
