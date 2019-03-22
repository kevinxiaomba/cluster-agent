package models

import (
	"strconv"
)

type HeatNode struct {
	Namespace   string
	Nodename    string
	Podname     string
	State       string
	PendingTime int64
	APM         bool
}

func NewHeatNode(podSchema PodSchema) HeatNode {
	return HeatNode{Namespace: podSchema.Namespace, Nodename: podSchema.NodeName, Podname: podSchema.Name,
		State: podSchema.GetState(), APM: podSchema.AppID > 0, PendingTime: int64(podSchema.PendingTime)}
}

func (hn HeatNode) FormatPendingTime() string {
	val := hn.PendingTime / 1000
	return strconv.FormatInt(val, 10)
}
