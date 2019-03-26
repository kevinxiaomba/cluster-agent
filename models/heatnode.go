package models

import (
	"fmt"
	"time"
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
	val := hn.PendingTime * 1000000
	d := time.Duration(val)
	var output = ""
	if d.Minutes() > 59 {
		output = fmt.Sprintf("%v:%v:%v", d.Hours(), d.Minutes(), d.Seconds())
	} else {
		output = fmt.Sprintf("%v:%v", d.Minutes(), d.Seconds())
	}
	fmt.Printf("Pending time: %s\n", output)
	return output
}
