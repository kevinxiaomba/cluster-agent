package models

import (
	"fmt"

	res "k8s.io/apimachinery/pkg/api/resource"
)

type NodeMetricsObjList struct {
	Kind  string
	Items []struct {
		Metadata struct {
			Name              string
			SelfLink          string
			creationTimestamp string
		}
		Timestamp string
		Window    string
		Usage     struct {
			Cpu    string
			Memory string
		}
	}
}

type PodMetricsObjList struct {
	Kind  string
	Items []struct {
		Metadata struct {
			Name              string
			SelfLink          string
			creationTimestamp string
		}
		Timestamp  string
		Window     string
		Containers []struct {
			Name  string
			Usage struct {
				Cpu    string
				Memory string
			}
		}
	}
}

type PodMetricsObj struct {
	Kind     string
	Metadata struct {
		Name              string
		SelfLink          string
		creationTimestamp string
	}
	Timestamp  string
	Window     string
	Containers []struct {
		Name  string
		Usage struct {
			Cpu    string
			Memory string
		}
	}
}

type NodeMetricsObj struct {
	Kind     string
	Metadata struct {
		Name              string
		SelfLink          string
		creationTimestamp string
	}
	Timestamp string
	Window    string
	Usage     struct {
		Cpu    string
		Memory string
	}
}

type UsageStats struct {
	Name   string
	CPU    int64
	Memory int64
}

func (m NodeMetricsObjList) PrintNodeList() map[string]UsageStats {
	objMap := make(map[string]UsageStats)
	for _, item := range m.Items {
		stats := UsageStats{}
		stats.Name = item.Metadata.Name
		cpuQ, err := res.ParseQuantity(item.Usage.Cpu)
		if err != nil {
			fmt.Printf("Cannot parse cpu %s to quantity\n", item.Usage.Cpu)
		}

		stats.CPU = cpuQ.MilliValue()
		fmt.Printf("Node CPU = %d\n", stats.CPU)

		memQ, err := res.ParseQuantity(item.Usage.Memory)
		if err != nil {
			fmt.Printf("Cannot parse mem %s to quantity\n", item.Usage.Memory)
		}
		stats.Memory = memQ.MilliValue() / 1000
		fmt.Printf("Node Memory = %d\n", stats.Memory)

		fmt.Printf("Name: %s\n CPU: %s\n, Memory: %s\n", item.Metadata.Name, item.Usage.Cpu, item.Usage.Memory)
		objMap[stats.Name] = stats
	}
	return objMap
}

func (m PodMetricsObjList) PrintPodList() map[string]UsageStats {
	objMap := make(map[string]UsageStats)
	for _, item := range m.Items {
		stats := UsageStats{}
		stats.Name = item.Metadata.Name
		stats.CPU = 0
		stats.Memory = 0
		for _, c := range item.Containers {
			cpuQ, err := res.ParseQuantity(c.Usage.Cpu)
			if err != nil {
				fmt.Printf("Cannot parse cpu %s to quantity\n", c.Usage.Cpu)
			}

			stats.CPU += cpuQ.MilliValue()
			fmt.Printf("Pod CPU = %d\n", stats.CPU)

			memQ, err := res.ParseQuantity(c.Usage.Memory)
			if err != nil {
				fmt.Printf("Cannot parse mem %s to quantity\n", c.Usage.Memory)
			}
			stats.Memory += memQ.MilliValue() / 1000
			fmt.Printf("Pod Memory = %d\n", stats.Memory)
			fmt.Printf("Pod Name: %s CPU: %s\n, Memory: %s\n", item.Metadata.Name, c.Usage.Cpu, c.Usage.Memory)
		}
		objMap[stats.Name] = stats
	}
	return objMap
}

func (mobj PodMetricsObj) GetPodUsage() UsageStats {
	stats := UsageStats{}
	stats.Name = mobj.Metadata.Name
	stats.CPU = 0
	stats.Memory = 0
	for _, c := range mobj.Containers {
		cpuQ, err := res.ParseQuantity(c.Usage.Cpu)
		if err != nil {
			fmt.Printf("Cannot parse cpu %s to quantity\n", c.Usage.Cpu)
		}

		stats.CPU += cpuQ.MilliValue()
		//		fmt.Printf("Pod CPU = %d\n", stats.CPU)

		memQ, err := res.ParseQuantity(c.Usage.Memory)
		if err != nil {
			fmt.Printf("Cannot parse mem %s to quantity\n", c.Usage.Memory)
		}
		stats.Memory += memQ.MilliValue() / 1000
		//		fmt.Printf("Pod Memory = %d\n", stats.Memory)
		//		fmt.Printf("Pod Name: %s CPU: %s\n, Memory: %s\n", mobj.Metadata.Name, c.Usage.Cpu, c.Usage.Memory)
	}
	return stats
}
