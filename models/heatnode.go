package models

import (
	"fmt"
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
	val := hn.PendingTime
	seconds := (val / 1000) % 60
	minutes := ((val / (1000 * 60)) % 60)
	hours := (int)((val / (1000 * 60 * 60)) % 24)

	var output = ""
	if minutes > 59 {
		output = fmt.Sprintf("%d:%d:%d", hours, minutes, seconds)
	} else {
		output = fmt.Sprintf("%d:%d", minutes, seconds)
	}
	fmt.Printf("Pending time: %s\n", output)
	return output
}
