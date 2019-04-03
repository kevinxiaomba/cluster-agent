package models

import (
	"fmt"
)

type HeatNode struct {
	Namespace   string
	Nodename    string
	Podname     string
	Owner       string
	State       string
	PendingTime int64
	APM         bool
	AppID       int
	TierID      int
	NodeID      int
}

func NewHeatNode(podSchema PodSchema) HeatNode {
	return HeatNode{Namespace: podSchema.Namespace, Nodename: podSchema.NodeName, Podname: podSchema.Name,
		State: podSchema.GetState(), APM: podSchema.AppID > 0, PendingTime: int64(podSchema.PendingTime),
		AppID: podSchema.AppID, TierID: podSchema.TierID, NodeID: podSchema.NodeID, Owner: podSchema.Owner}
}

func (hn *HeatNode) FormatPendingTime() string {
	val := hn.PendingTime
	seconds := (val / 1000) % 60
	minutes := ((val / (1000 * 60)) % 60)
	hours := (int)((val / (1000 * 60 * 60)) % 24)

	var output = ""
	if minutes > 59 {
		output = fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	} else {
		if minutes == 0 {
			output = fmt.Sprintf("00:00:%02d", seconds)
		} else {
			output = fmt.Sprintf("00:%02d:%02d", minutes, seconds)
		}
	}
	fmt.Printf("Pending time: %s\n", output)
	return output
}

func (hn *HeatNode) GetAPMID() (int, bool) {
	if hn.NodeID > 0 {
		return hn.NodeID, true
	}
	if hn.TierID > 0 {
		return hn.TierID, false

	}
	return 0, false
}
