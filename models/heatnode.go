package models

import (
	"fmt"
	"strings"
)

type Utilization struct {
	CpuUse       float64
	MemUse       float64
	CpuUseString string
	MemUseString string
	Overconsume  bool
	CpuGoal      float64
	MemGoal      float64
	Restarts     int32
}

func (u *Utilization) CheckStatus(threshold float64, cpuRequest float64, memRequest float64) {
	if u.CpuUse >= threshold {
		u.Overconsume = true
		//		if cpuRequest > 0 {
		//			diff := u.CpuUse - threshold
		//			u.CpuGoal = cpuRequest + (diff+20)*cpuRequest
		//		}
	}

	if u.MemUse >= threshold {
		u.Overconsume = true
		//		if memRequest > 0 {
		//			diff := u.MemUse - threshold
		//			u.MemGoal = memRequest + (diff+20)*memRequest
		//		}
	}
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
	Restarts    int32
	Events      []string
	Containers  map[string]Utilization
}

func NewHeatNode(podSchema PodSchema) HeatNode {
	return HeatNode{Namespace: podSchema.Namespace, Nodename: podSchema.NodeName, Podname: podSchema.Name,
		State: podSchema.GetState(), APM: podSchema.AppID > 0, PendingTime: int64(podSchema.PendingTime),
		AppID: podSchema.AppID, TierID: podSchema.TierID, NodeID: podSchema.NodeID, Owner: podSchema.Owner,
		Events: []string{}, Restarts: podSchema.PodRestarts}
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
	return output
}

func (hn *HeatNode) GetContainerStatsFormatted() string {
	if len(hn.Containers) == 0 {
		return fmt.Sprintf("Restarts: %d\n", hn.Restarts)
	}
	s := ""
	for name, c := range hn.Containers {
		cpuVal := ""
		memVal := ""
		restartVal := ""
		if c.CpuUse >= 0 && c.MemUse >= 0 {
			if c.CpuUse < 1 {
				cpuVal = fmt.Sprintf("Cpu: %s (<1%s)\n", c.CpuUseString, "%")
			} else {
				cpuVal = fmt.Sprintf("Cpu: %s (%.0f%s)\n", c.CpuUseString, c.CpuUse, "%")
				//				if c.CpuGoal > 0 {
				//					cpuVal = fmt.Sprintf("Cpu: %.0f%s (%.0f)\n", c.CpuUse, "%", c.CpuGoal)
				//				} else {
				//					cpuVal = fmt.Sprintf("Cpu: %.0f%s\n", c.CpuUse, "%")
				//				}
			}

			if c.MemUse < 1 {
				memVal = fmt.Sprintf("Mem: %s (<1%s)\n", c.MemUseString, "%")
			} else {
				memVal = fmt.Sprintf("Mem: %s (%.0f%s)\n", c.MemUseString, c.MemUse, "%")
				//				if c.MemGoal > 0 {
				//					memVal = fmt.Sprintf("Mem: %.0f%s (%.0f)\n", c.MemUse, "%", c.MemGoal)
				//				} else {
				//					memVal = fmt.Sprintf("Mem: %.0f%s\n", c.MemUse, "%")
				//				}
			}
			if c.Restarts > 0 {
				restartVal = fmt.Sprintf("Restarts: %d\n", c.Restarts)
			}

			s += fmt.Sprintf("%s:\n%s%s%s", name, restartVal, cpuVal, memVal)
		}
	}

	return s
}

func (hn *HeatNode) IsOverconsuming() bool {
	for _, c := range hn.Containers {
		if c.Overconsume == true {
			return true
		}
	}
	return false
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
