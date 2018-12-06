package workers

import (
	//	"encoding/json"
	"fmt"

	m "github.com/sjeltuhin/clusterAgent/models"
	appsv1 "k8s.io/api/apps/v1"

	app "github.com/sjeltuhin/clusterAgent/appd"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type DeployWorker struct {
	informer       cache.SharedIndexInformer
	Client         *kubernetes.Clientset
	Bag            *m.AppDBag
	SummaryMap     map[string]m.ClusterPodMetrics
	WQ             workqueue.RateLimitingInterface
	AppdController *app.ControllerClient
}

func NewDeployWorker(client *kubernetes.Clientset, bag *m.AppDBag, controller *app.ControllerClient) DeployWorker {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	pw := DeployWorker{Client: client, Bag: bag, SummaryMap: make(map[string]m.ClusterPodMetrics), WQ: queue, AppdController: controller}
	pw.initPodInformer(client)
	return pw
}

func (nw *DeployWorker) initPodInformer(client *kubernetes.Clientset) cache.SharedIndexInformer {
	i := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.AppsV1().Deployments(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.AppsV1().Deployments(metav1.NamespaceAll).Watch(options)
			},
		},
		&v1.Node{},
		0,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	i.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    nw.onNewDeployment,
		DeleteFunc: nw.onDeleteDeployment,
		UpdateFunc: nw.onUpdateDeployment,
	})
	nw.informer = i

	return i
}

func (nw *DeployWorker) onNewDeployment(obj interface{}) {
	deployObj := obj.(*appsv1.Deployment)
	fmt.Printf("Added Deployment: %s\n", deployObj.Name)

	labels := deployObj.Labels
	if labels != nil {
		if val, ok := labels[nw.Bag.AgentLabel]; ok {
			//add volume mount to the container name in the label
			var container *v1.Container = nil
			for _, c := range deployObj.Spec.Template.Spec.Containers {
				if c.Name == val {
					container = &c
					break
				}
			}

			if container != nil {
				mountExists := false
				for _, vm := range container.VolumeMounts {
					if vm.Name == nw.Bag.AgentMountName {
						mountExists = true
						break
					}
				}
				if !mountExists {
					//container volume mount
					volumeMount := v1.VolumeMount{Name: nw.Bag.AgentMountName, MountPath: "/opt/appd", ReadOnly: true}
					if container.VolumeMounts == nil || len(container.VolumeMounts) == 0 {
						container.VolumeMounts = []v1.VolumeMount{volumeMount}
					} else {
						container.VolumeMounts = append(container.VolumeMounts, volumeMount)
					}
				}

				volExists := false
				for _, vol := range deployObj.Spec.Template.Spec.Volumes {
					if vol.Name == nw.Bag.AgentMountName {
						volExists = true
						break
					}
				}
				if !volExists {
					//pod volume
					pathType := v1.HostPathDirectory
					source := v1.HostPathVolumeSource{Path: "/opt/appd", Type: &pathType}
					volSource := v1.VolumeSource{HostPath: &source}
					vol := v1.Volume{Name: "appd-repo", VolumeSource: volSource}
					deployObj.Spec.Template.Spec.Volumes = []v1.Volume{vol}
				}

				//update deployment
				deploymentsClient := nw.Client.AppsV1().Deployments(deployObj.Namespace)
				deploymentsClient.Update(deployObj)
			}
		}
	}

}

func (nw *DeployWorker) onDeleteDeployment(obj interface{}) {
	deployObj := obj.(*appsv1.Deployment)
	fmt.Printf("Deleted Deployment: %s\n", deployObj.Name)

}

func (nw *DeployWorker) onUpdateDeployment(objOld interface{}, objNew interface{}) {
	deployObj := objOld.(*appsv1.Deployment)
	fmt.Printf("Updated Deployment: %s\n", deployObj.Name)

}
