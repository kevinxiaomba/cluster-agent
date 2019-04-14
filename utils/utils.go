package utils

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	m "github.com/appdynamics/cluster-agent/models"
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

func GetConfigMapKey(cm *v1.ConfigMap) string {
	return fmt.Sprintf("%s/%s", cm.Namespace, cm.Name)
}

func GetSecretKey(secret *v1.Secret) string {
	return fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)
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

func IsSystemNamespace(namespace string) bool {
	if strings.Contains(namespace, "kube") || strings.Contains(namespace, "openshift") {
		return true
	}
	return false
}

func NodeQualifiesForMonitoring(name string, bag *m.AppDBag) bool {
	return (len(bag.NodesToMonitor) == 0 ||
		StringInSlice(name, bag.NodesToMonitor)) &&
		!StringInSlice(name, bag.NodesToMonitorExclude)
}

func FormatDuration(sinceNanosec int64) string {
	val := sinceNanosec / 1000000
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

func SplitUrl(url string) (string, string, int, error) {
	if strings.Contains(url, "http") {
		arr := strings.Split(url, ":")
		if len(arr) != 3 {
			return "", "", 0, fmt.Errorf("Url is invalid. Use this format: protocol://url:port")
		}
		protocol := arr[0]
		host := strings.TrimLeft(arr[1], "//")
		port, errPort := strconv.Atoi(arr[2])
		if errPort != nil {
			return "", "", 0, fmt.Errorf("Port is invalid. %v", errPort)
		}
		return protocol, host, port, nil
	} else {
		return "", "", 0, fmt.Errorf("Url is invalid. Use this format: protocol://url:port")
	}

}
