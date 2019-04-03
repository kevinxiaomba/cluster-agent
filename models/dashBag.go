package models

import (
	"sort"
)

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
	NSMap         map[string]map[string][]HeatNode
}

func NewDashboardBagTier(ns string, tierName string, pods []PodSchema) DashboardBag {
	return DashboardBag{Namespace: ns, TierName: tierName, Pods: pods, Type: Tier, NSMap: make(map[string]map[string][]HeatNode)}
}

func NewDashboardBagCluster() *DashboardBag {
	return &(DashboardBag{Type: Cluster, NSMap: make(map[string]map[string][]HeatNode)})
}

func (db *DashboardBag) AddNode(hn *HeatNode) {
	if _, ok := db.NSMap[hn.Namespace]; !ok {
		db.NSMap[hn.Namespace] = make(map[string][]HeatNode)
	}
	if _, okD := db.NSMap[hn.Namespace][hn.Owner]; !okD {
		db.NSMap[hn.Namespace][hn.Owner] = []HeatNode{}
	}
	db.NSMap[hn.Namespace][hn.Owner] = append(db.NSMap[hn.Namespace][hn.Owner], *hn)

}

func (db *DashboardBag) GetNodes() ([]HeatNode, int, int) {
	nodes := []HeatNode{}

	numNS := 0
	numDeploy := 0

	//sort namespaces
	var keys []string
	for k := range db.NSMap {
		keys = append(keys, k)
		numNS++
	}
	sort.Strings(keys)
	for _, nsKey := range keys {
		dv := db.NSMap[nsKey]
		var keysDeploy []string
		for kD := range dv {
			keysDeploy = append(keysDeploy, kD)
			numDeploy++
		}
		sort.Strings(keysDeploy)
		for _, narrKey := range keysDeploy {
			nodeArray := dv[narrKey]
			sort.Slice(nodeArray[:], func(i, j int) bool {
				return nodeArray[i].Podname < nodeArray[j].Podname
			})
			for _, n := range nodeArray {
				nodes = append(nodes, n)
			}
		}
	}
	return nodes, numNS, numDeploy
}
