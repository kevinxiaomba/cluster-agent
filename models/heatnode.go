package models

type HeatNode struct {
	Namespace string
	Nodename  string
	Podname   string
	State     string
	APM       bool
}

func NewHeatNode(podSchema PodSchema) HeatNode {
	return HeatNode{Namespace: podSchema.Namespace, Nodename: podSchema.NodeName, Podname: podSchema.Name, State: podSchema.GetState(), APM: podSchema.AppID > 0}
}
