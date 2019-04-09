package models

import (
	"fmt"
	"strings"
)

type Utilization struct {
	CpuUse float64
	MemUse float64
}

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
	Events      []string
	Containers  map[string]Utilization
}

func NewHeatNode(podSchema PodSchema) HeatNode {
	return HeatNode{Namespace: podSchema.Namespace, Nodename: podSchema.NodeName, Podname: podSchema.Name,
		State: podSchema.GetState(), APM: podSchema.AppID > 0, PendingTime: int64(podSchema.PendingTime),
		AppID: podSchema.AppID, TierID: podSchema.TierID, NodeID: podSchema.NodeID, Owner: podSchema.Owner, Events: []string{}}
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

func (hn *HeatNode) GetContainerStatsFormatted() string {
	s := ""
	for name, c := range hn.Containers {

		if c.CpuUse >= 0 && c.MemUse >= 0 {
			cpuVal := fmt.Sprintf("Cpu: %.0f%s\n", c.CpuUse, "%")

			if c.CpuUse < 1 {
				cpuVal = "Cpu: <1%\n"
			}
			memVal := fmt.Sprintf("Mem: %.0f%s\n", c.MemUse, "%")
			if c.MemUse < 1 {
				memVal = "Mem: <1%\n"
			}
			s += fmt.Sprintf("%s:\n%s%s", name, cpuVal, memVal)
		}
	}

	return s
}

func (hn *HeatNode) GetEventsFormatted() string {

	return fmt.Sprintf("%s", strings.Join(hn.Events, "\n"))
}

func (hn *HeatNode) GetContainerCount() int {
	return len(hn.Containers)
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
