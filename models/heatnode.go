package models

type HeatNode struct {
	Namespace string
	Nodename  string
	Podname   string
	State     string
	APM       bool
}

func NewHeatNode(ns string, node string, podName string, state string, apm bool) HeatNode {
	return HeatNode{Namespace: ns, Nodename: node, Podname: podName, State: state, APM: apm}
}
