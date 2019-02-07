package utils

import (
	"fmt"
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
