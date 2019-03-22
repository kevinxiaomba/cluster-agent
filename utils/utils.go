package utils

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	m "github.com/sjeltuhin/clusterAgent/models"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
)

func IsPodRunnnig(podObj *v1.Pod) bool {
	return podObj != nil && podObj.Status.Phase == "Running" && podObj.DeletionTimestamp == nil
}

func SplitPodKey(podKey string) (string, string) {
	ar := strings.Split(podKey, "/")
	if len(ar) > 1 {
		return ar[0], ar[1]
	}

	return "", ""
}

func GetDeployKey(deployObj *appsv1.Deployment) string {
	return fmt.Sprintf("%s/%s", deployObj.Namespace, deployObj.Name)
}

func GetPodKey(podObj *v1.Pod) string {
	return GetKey(podObj.Namespace, podObj.Name)
}

func GetKey(namespace, podName string) string {
	return fmt.Sprintf("%s/%s", namespace, podName)
}

func GetPodSchemaKey(podObj *m.PodSchema) string {
	return GetKey(podObj.Namespace, podObj.Name)
}

func GetK8sServiceKey(svc *v1.Service) string {
	return fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
}

func GetServiceKey(svc *m.ServiceSchema) string {
	return fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
}

func GetEndpointKey(ep *v1.Endpoints) string {
	return fmt.Sprintf("%s/%s", ep.Namespace, ep.Name)
}

func StringInSlice(s string, list []string) bool {
	return GetIndex(s, list) > -1
}

func GetIndex(s string, list []string) int {
	index := -1
	for i, b := range list {
		if b == s {
			index = i
			break
		}
	}
	return index
}

func RemoveFromSlice(s string, list []string) []string {
	i := GetIndex(s, list)
	if i < 0 {
		return list
	}
	list[i] = list[len(list)-1]

	return list[:len(list)-1]
}

func CloneMap(source map[string]interface{}) (map[string]interface{}, error) {
	jsonStr, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}
	var dest map[string]interface{}
	err = json.Unmarshal(jsonStr, &dest)
	if err != nil {
		return nil, err
	}
	return dest, nil
}

func MapContainsNocase(m map[string]interface{}, key string) (interface{}, bool) {
	for k, v := range m {
		if strings.ToLower(key) == strings.ToLower(k) {
			return v, true
		}
	}
	return nil, false
}

func NSQualifiesForMonitoring(ns string, bag *m.AppDBag) bool {
	return (len(bag.NsToMonitor) == 0 ||
		StringInSlice(ns, bag.NsToMonitor)) &&
		!StringInSlice(ns, bag.NsToMonitorExclude)
}

func NSQualifiesForInstrumentation(ns string, bag *m.AppDBag) bool {
	return (len(bag.NsToInstrument) == 0 ||
		StringInSlice(ns, bag.NsToInstrument)) &&
		!StringInSlice(ns, bag.NsToInstrumentExclude)
}

func GetAgentRequestsForDeployment(deploy *appsv1.Deployment, bag *m.AppDBag) *m.AgentRequestList {
	var list *m.AgentRequestList = nil
	if rules, ok := bag.NSInstrumentRule[deploy.Namespace]; ok {
		list = m.NewAgentRequestListFromArray(rules)
	}

	return list
}

func DeployQualifiesForInstrumentation(deploy *appsv1.Deployment, bag *m.AppDBag) bool {
	if rules, ok := bag.NSInstrumentRule[deploy.Namespace]; ok {
		for _, r := range rules {
			reg, _ := regexp.Compile(r.MatchString)
			if reg.MatchString(deploy.Name) {
				return true
			}
			for _, v := range deploy.Labels {
				if reg.MatchString(v) {
					return true
				}
			}
		}
	}
	if bag.InstrumentMatchString != "" {
		globReg, _ := regexp.Compile(bag.InstrumentMatchString)
		if globReg.MatchString(deploy.Name) {
			return true
		}

		for _, v := range deploy.Labels {
			if globReg.MatchString(v) {
				return true
			}
		}
	}

	return NSQualifiesForInstrumentation(deploy.Namespace, bag)
}

func NodeQualifiesForMonitoring(name string, bag *m.AppDBag) bool {
	return (len(bag.NodesToMonitor) == 0 ||
		StringInSlice(name, bag.NodesToMonitor)) &&
		!StringInSlice(name, bag.NodesToMonitorExclude)
}
