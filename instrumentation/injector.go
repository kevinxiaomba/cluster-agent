package instrumentation

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	app "github.com/sjeltuhin/clusterAgent/appd"
	appsv1 "k8s.io/api/apps/v1"

	m "github.com/sjeltuhin/clusterAgent/models"
	"github.com/sjeltuhin/clusterAgent/utils"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	GET_JAVA_PID_CMD             string = "ps -o pid,comm,args | grep java | awk '{print$1,$2,$3}'"
	ATTACHED_ANNOTATION          string = "appd-attached"
	APPD_ATTACH_PENDING          string = "appd-attach-pending"
	DEPLOY_ANNOTATION            string = "appd-deploy-updated"
	DEPLOY_BIQ_ANNOTATION        string = "appd-deploy-biq-updated"
	APPD_APPID                   string = "appd-appid"
	APPD_TIERID                  string = "appd-tierid"
	APPD_NODEID                  string = "appd-nodeid"
	APPD_NODENAME                string = "appd-nodename"
	MAX_INSTRUMENTATION_ATTEMPTS int    = 1
	ANNOTATION_UPDATE_ERROR      string = "ANNOTATION-UPDATE-FAILURE"
	APPD_SECRET_NAME             string = "appd-secret"
	APPD_SECRET_KEY_NAME         string = "appd-key"
	APPD_ASSOCIATE_ERROR         string = "appd-associate-error"
)

type AgentInjector struct {
	ClientSet      *kubernetes.Clientset
	K8sConfig      *rest.Config
	Bag            *m.AppDBag
	AppdController *app.ControllerClient
}

func NewAgentInjector(client *kubernetes.Clientset, config *rest.Config, bag *m.AppDBag, appdController *app.ControllerClient) AgentInjector {
	return AgentInjector{ClientSet: client, K8sConfig: config, Bag: bag, AppdController: appdController}
}

func AnalyticsAgentExists(podSpec *v1.PodSpec, bag *m.AppDBag) bool {
	exists := false

	for _, c := range podSpec.Containers {
		if c.Name == bag.AnalyticsAgentContainerName {
			exists = true
			break
		}
	}
	return exists
}

func AgentInitExists(podSpec *v1.PodSpec, bag *m.AppDBag) bool {
	exists := false

	for _, c := range podSpec.InitContainers {
		if c.Name == bag.AppDInitContainerName {
			exists = true
			break
		}
	}
	return exists
}

func IsPodInstrumented(podObj *v1.Pod) bool {
	for k, v := range podObj.Annotations {
		if k == ATTACHED_ANNOTATION && v != "" {
			fmt.Printf("Pod %s already instrumented. Skipping...\n", podObj.Name)
			return true
		}
	}
	return false
}

func GetAttachMetadata(appTag string, tierTag string, deploy *appsv1.Deployment, bag *m.AppDBag) (string, string, string) {
	var appName, tierName, biQDeploymentOption string

	if appTag == "" {
		appTag = bag.AppDAppLabel
	}

	if tierTag == "" {
		tierTag = bag.AppDTierLabel
	}

	for k, v := range deploy.Labels {
		fmt.Printf("Labels: %s:%s\n", k, v)
		if k == appTag {
			appName = v
		}

		if k == tierTag {
			tierName = v
		}

		if k == bag.AppDAnalyticsLabel {
			biQDeploymentOption = v
		}
	}

	return appName, tierName, biQDeploymentOption
}

func GetAgentRequestsForDeployment(deploy *appsv1.Deployment, bag *m.AppDBag) *m.AgentRequestList {
	//check for exclusions
	if bag.InstrumentationMethod == m.None || utils.StringInSlice(deploy.Namespace, bag.NsToInstrumentExclude) {
		fmt.Printf("Instrumentation is not configured for namespace %s\n", deploy.Namespace)
		return nil
	}

	var list *m.AgentRequestList = nil
	//check deployment labels for instrumentation requests
	var appAgent string
	appName, tierName, biQDeploymentOption := GetAttachMetadata(bag.AppDAppLabel, bag.AppDTierLabel, deploy, bag)

	fmt.Printf("biQDeploymentOption: %s\n", biQDeploymentOption)
	for k, v := range deploy.Labels {
		if k == bag.AgentLabel {
			appAgent = v
		}
	}

	if appAgent != "" || appName != "" {
		al := m.NewAgentRequestList(appAgent, appName, tierName, biQDeploymentOption, deploy.Spec.Template.Spec.Containers, bag)
		fmt.Printf("Using deployment metadata for agent request. AppName: %s AppAgent: %s\n", appName, appAgent)
		list = &al
	} else {
		//try global configuration
		//first rules
		var namespaceRule *m.AgentRequest = nil
		arr := []m.AgentRequest{}

		for _, r := range bag.NSInstrumentRule {
			applies := false
			for _, ns := range r.Namespaces {
				if ns == deploy.Namespace {
					applies = true
					break
				}
			}
			if applies {
				if len(r.MatchString) == 0 {
					namespaceRule = &r
				}
				for _, ms := range r.MatchString {
					reg, _ := regexp.Compile(ms)
					if reg.MatchString(deploy.Name) {
						r.AppName, r.TierName, _ = GetAttachMetadata(r.AppDAppLabel, r.AppDTierLabel, deploy, bag)
						arr = append(arr, r)
					}
					if len(arr) == 0 {
						for _, v := range deploy.Labels {
							if reg.MatchString(v) {
								r.AppName, r.TierName, _ = GetAttachMetadata(r.AppDAppLabel, r.AppDTierLabel, deploy, bag)
								arr = append(arr, r)
							}
						}
					}
				}
			}
		}

		//in case a namespace-wide rule exists
		if len(arr) == 0 && namespaceRule != nil {
			namespaceRule.AppName, namespaceRule.TierName, namespaceRule.BiQ = GetAttachMetadata(namespaceRule.AppDAppLabel, namespaceRule.AppDTierLabel, deploy, bag)
			arr = append(arr, (*namespaceRule))
		}
		if len(arr) > 0 {
			fmt.Printf("Applying %d custom rules for agent request\n", len(arr))
			list = m.NewAgentRequestListFromArray(arr, bag, deploy.Spec.Template.Spec.Containers)
		}

		//if no rules exist for deployment/namespace, check namespace settings
		if list == nil {
			if utils.StringInSlice(deploy.Namespace, bag.NsToInstrument) {
				global := false
				if len(bag.InstrumentMatchString) == 0 {
					//everything in the namespace needs to be instrumented
					global = true

				} else {
					for _, ms := range bag.InstrumentMatchString {
						globReg, _ := regexp.Compile(ms)
						if globReg.MatchString(deploy.Name) {
							global = true
						}
						if list == nil {
							for _, v := range deploy.Labels {
								if globReg.MatchString(v) {
									global = true
									break
								}
							}
						}
					}
				}
				if global {
					fmt.Printf("Applying global rule for agent request\n")
					if appName == "" {
						appName = deploy.Name
					}
					al := m.NewAgentRequestList("", appName, tierName, biQDeploymentOption, deploy.Spec.Template.Spec.Containers, bag)
					list = &al
				}
			}
		}
	}
	if list != nil {
		fmt.Printf(list.String())
	} else {
		fmt.Printf("No Agent requests found\n")
	}
	return list
}

func ShouldInstrumentDeployment(deployObj *appsv1.Deployment, bag *m.AppDBag, pendingCache *[]string, failedCache *map[string]m.AttachStatus) (bool, bool, *m.AgentRequestList) {
	//check if already updated
	updated := false
	biqUpdated := false

	for k, v := range deployObj.Annotations {
		if k == DEPLOY_ANNOTATION && v != "" {
			updated = true
		}

		if k == DEPLOY_BIQ_ANNOTATION && v != "" {
			biqUpdated = true
		}
	}

	fmt.Printf("Update status: %t. BiQ updated: %t\n", updated, biqUpdated)

	if updated || biqUpdated {
		(*pendingCache) = utils.RemoveFromSlice(utils.GetDeployKey(deployObj), *pendingCache)
		fmt.Printf("Deployment %s already updated for AppD. Skipping...\n", deployObj.Name)
		return false, false, nil
	}

	if utils.StringInSlice(utils.GetDeployKey(deployObj), *pendingCache) {
		fmt.Printf("Deployment %s is in process of update. Waiting...\n", deployObj.Name)
		return false, false, nil
	}

	//	check Failed cache not to exceed failure limit
	status, ok := (*failedCache)[utils.GetDeployKey(deployObj)]
	if ok && status.Count >= MAX_INSTRUMENTATION_ATTEMPTS {
		fmt.Printf("Deployment %s exceeded the max number of failed instrumentation attempts. Skipping...\n", deployObj.Name)
		return false, false, nil
	}

	agentRequests := GetAgentRequestsForDeployment(deployObj, bag)
	if agentRequests == nil {
		fmt.Printf("Deployment %s does not need to be instrumented. Ignoring...\n", deployObj.Name)
		return false, false, nil
	}

	var biqRequested = agentRequests.BiQRequested()
	initRequested := !AgentInitExists(&deployObj.Spec.Template.Spec, bag) && agentRequests.InitContainerRequired()

	if !biqRequested && !initRequested {
		fmt.Printf("Instrumentation not requested. Skipping %s...\n", deployObj.Name)
	}

	biq := agentRequests.GetBiQOption() == string(m.Sidecar) && !AnalyticsAgentExists(&deployObj.Spec.Template.Spec, bag)

	return initRequested, biq, agentRequests
}

func (ai *AgentInjector) findContainer(agentRequest *m.AgentRequest, podObj *v1.Pod) *v1.Container {

	for _, c := range podObj.Spec.Containers {
		if c.Name == agentRequest.ContainerName {
			return &c
		}
	}

	return nil
}

func (ai AgentInjector) EnsureInstrumentation(statusChanel chan m.AttachStatus, podObj *v1.Pod, podSchema *m.PodSchema) error {
	//check if already instrumented
	if IsPodInstrumented(podObj) {
		statusChanel <- ai.buildAttachStatus(podObj, nil, nil, true)
		return nil
	}

	//get pending attach annotation
	var agentRequests *m.AgentRequestList = nil
	for ak, av := range podObj.Annotations {
		if ak == APPD_ATTACH_PENDING {
			agentRequests = m.FromAnnotation(av)
			break
		}
	}
	if agentRequests == nil {
		fmt.Println("Instrumentation not requested. Aborting...")
		statusChanel <- ai.buildAttachStatus(podObj, nil, nil, true)
	} else {
		fmt.Printf("Agent request from pod annotation: %s\n", agentRequests.String())

		for _, r := range agentRequests.Items {
			c := ai.findContainer(&r, podObj)
			if r.Method == m.MountAttach {
				fmt.Printf("Container %s requested. Instrumenting...\n", c.Name)
				err := ai.instrumentContainer(r.AppName, r.TierName, c, podObj, m.BiQDeploymentOption(r.BiQ), &r)
				statusChanel <- ai.buildAttachStatus(podObj, &r, err, false)
			} else {
				ai.finilizeAttach(statusChanel, podObj, &r)
			}
		}

	}

	return nil

}

func (ai AgentInjector) finilizeAttach(statusChanel chan m.AttachStatus, podObj *v1.Pod, agentRequest *m.AgentRequest) {
	if agentRequest.Tech == m.DotNet || agentRequest.Method == m.MountEnv {
		fmt.Printf("Finilizing instrumentation for container %s...\n", agentRequest.ContainerName)
		exec := NewExecutor(ai.ClientSet, ai.K8sConfig)
		updateErr := ai.Associate(podObj, &exec, agentRequest)
		if updateErr != nil {
			statusChanel <- ai.buildAttachStatus(podObj, agentRequest, fmt.Errorf("%s, Error: %v\n", ANNOTATION_UPDATE_ERROR, updateErr), false)
		} else {
			statusChanel <- ai.buildAttachStatus(podObj, nil, nil, false)
		}
	}
}

func (ai AgentInjector) buildAttachStatus(podObj *v1.Pod, agentRequest *m.AgentRequest, err error, skip bool) m.AttachStatus {
	if err == nil {
		return m.AttachStatus{Key: utils.GetPodKey(podObj), Success: !skip}
	}
	message := err.Error()
	st := m.AttachStatus{Key: utils.GetPodKey(podObj), Count: 1, LastAttempt: time.Now(), LastMessage: message}
	st.Request = agentRequest
	if strings.Contains(message, ANNOTATION_UPDATE_ERROR) || strings.Contains(message, APPD_ASSOCIATE_ERROR) {
		st.Success = true
		st.RetryAssociation = true
	}
	return st
}

func (ai AgentInjector) instrumentContainer(appName string, tierName string, container *v1.Container, podObj *v1.Pod, biQDeploymentOption m.BiQDeploymentOption, agentRequest *m.AgentRequest) error {
	var procName, args string
	var procID int = 0
	exec := NewExecutor(ai.ClientSet, ai.K8sConfig)

	if ai.Bag.InstrumentationMethod == m.CopyAttach {
		//copy files
		err := ai.copyArtifactsSync(&exec, podObj, container.Name)
		if err != nil {
			return fmt.Errorf("Unable to instrument. Failed to copy necessary artifacts into the pod. %v", err)
		}
		fmt.Println("Artifacts copied. Starting instrumentation...")
	} else {
		fmt.Println("Artifacts mounted. Starting instrumentation...")
	}

	//run attach
	code, output, err := exec.RunCommandInPod(podObj.Name, podObj.Namespace, container.Name, "", GET_JAVA_PID_CMD)
	if code == 0 {
		legit := true
		//split by line
		rows := strings.Split(output, "\n")
		for _, line := range rows {
			fields := strings.SplitN(line, " ", 3)

			if len(fields) > 1 {
				procName = fields[1]
			}
			if len(fields) > 2 {
				args = fields[2]
				if strings.Contains(args, "-DAppdynamics") {
					legit = false
					break
				}
			}
			pid, err := strconv.Atoi(fields[0])
			if err == nil {
				fmt.Printf("PID: %d, Name: %s, Args: %s\n", pid, procName, args)
				if strings.Contains(procName, "java") {
					procID = pid
				}
			}
		}
		if legit && procID > 0 {
			fmt.Printf("Instrumenting process %d\n", procID)
			return ai.instrument(podObj, procID, appName, tierName, container.Name, &exec, biQDeploymentOption, agentRequest)
		} else {
			return fmt.Errorf("Enable to determine process to instrument")
		}
	} else {
		return fmt.Errorf("Exec error code = %d. Output: %s, Error = %v\n", code, output, err)
	}
	return nil
}

func (ai AgentInjector) copyArtifactsSync(exec *Executor, podObj *v1.Pod, containerName string) error {
	bth := ai.AppdController.StartBT("CopyArtifacts")
	err := ai.copyFileSync(exec, "assets/tools.jar", ai.Bag.AgentMountPath, podObj, containerName)
	if err == nil {
		err = ai.copyFileSync(exec, "assets/AppServerAgent/", ai.Bag.AgentMountPath, podObj, containerName)
	}
	ai.AppdController.StopBT(bth)
	return err
}

func (ai AgentInjector) copyArtifacts(exec *Executor, podObj *v1.Pod, containerName string) bool {
	okJDKChan := make(chan bool)
	okAppDChan := make(chan bool)
	var wg sync.WaitGroup
	wg.Add(1)
	//JDK tools
	go ai.copyFile(okJDKChan, &wg, exec, "assets/tools.jar", ai.Bag.AgentMountPath, podObj, containerName)

	wg.Add(1)
	//AppD Agent
	go ai.copyFile(okAppDChan, &wg, exec, "assets/AppServerAgent/", ai.Bag.AgentMountPath, podObj, containerName)

	okJDK := <-okJDKChan
	okAppD := <-okAppDChan

	wg.Wait()

	if !okJDK || !okAppD {
		return false
	}
	return true
}

func (ai AgentInjector) copyFileSync(exec *Executor, fileName, dir string, podObj *v1.Pod, containerName string) error {
	filePath, errFile := filepath.Abs(fileName)
	if errFile != nil {
		fmt.Printf("Cannot find %s path. %v\n", filePath, errFile)
		return errFile
	}
	fmt.Printf("Copying file %s\n", filePath)
	_, _, copyErr := exec.CopyFilesToPod(podObj, containerName, filePath, dir)
	if copyErr != nil {
		fmt.Printf("Copy failed. Aborting instrumentation. %v\n", copyErr)
		return copyErr
	}
	return nil
}

func (ai AgentInjector) copyFile(ok chan bool, wg *sync.WaitGroup, exec *Executor, fileName, dir string, podObj *v1.Pod, containerName string) {
	defer wg.Done()
	filePath, errFile := filepath.Abs(fileName)
	if errFile != nil {
		fmt.Printf("Cannot find %s path. %v\n", filePath, errFile)
		ok <- false
	}
	fmt.Printf("Copying file %s\n", filePath)
	_, _, copyErr := exec.CopyFilesToPod(podObj, containerName, filePath, dir)
	if copyErr != nil {
		fmt.Printf("Copy failed. Aborting instrumentation. %v\n", copyErr)
		ok <- false
	}
	ok <- true
}

func (ai AgentInjector) instrument(podObj *v1.Pod, pid int, appName string, tierName string, containerName string, exec *Executor, biQDeploymentOption m.BiQDeploymentOption, agentRequest *m.AgentRequest) error {
	jdkPath := ai.Bag.AgentMountPath
	jarPath := ai.Bag.AgentMountPath

	bth := ai.AppdController.StartBT("Instrument")
	cmd := fmt.Sprintf("java -Xbootclasspath/a:%s/tools.jar -jar %s/javaagent.jar %d appdynamics.controller.hostName=%s,appdynamics.controller.port=%d,appdynamics.controller.ssl.enabled=%t,appdynamics.agent.accountName=%s,appdynamics.agent.accountAccessKey=%s,appdynamics.agent.applicationName=%s,appdynamics.agent.tierName=%s,appdynamics.agent.reuse.nodeName=true,appdynamics.agent.reuse.nodeName.prefix=%s",
		jdkPath, jarPath, pid, ai.Bag.ControllerUrl, ai.Bag.ControllerPort, ai.Bag.SSLEnabled, ai.Bag.Account, ai.Bag.AccessKey, appName, tierName, tierName)

	//BIQ instrumentation. If Analytics agent is remote, provide the url when attaching
	fmt.Printf("BiQ deployment option is %s.\n", biQDeploymentOption)
	if biQDeploymentOption != m.Sidecar {
		fmt.Printf("Will add remote url %s\n", ai.Bag.AnalyticsAgentUrl)
		if ai.Bag.AnalyticsAgentUrl != "" {
			cmd = fmt.Sprintf("%s,appdynamics.analytics.agent.url=%s/v2/sinks/bt", cmd, ai.Bag.AnalyticsAgentUrl)
		}
	}
	code, output, err := exec.RunCommandInPod(podObj.Name, podObj.Namespace, containerName, "", cmd)
	if code == 0 {
		fmt.Printf("AppD agent attached. Output: %s. Error: %v\n", output, err)
		errA := ai.Associate(podObj, exec, agentRequest)
		if errA != nil {
			return fmt.Errorf("Unable to annotate and associate the attached pod %v\n", errA)
		}

	} else {
		return fmt.Errorf("Unable to attach Java agent. Error code = %d. Output: %s, Error: %v\n", code, output, err)
	}
	ai.AppdController.StopBT(bth)
	return nil
}

func (ai AgentInjector) RetryAssociate(podObj *v1.Pod, agentRequest *m.AgentRequest) error {
	exec := NewExecutor(ai.ClientSet, ai.K8sConfig)
	return ai.Associate(podObj, &exec, agentRequest)
}

func (ai AgentInjector) Associate(podObj *v1.Pod, exec *Executor, agentRequest *m.AgentRequest) error {
	jarPath := GetVolumePath(ai.Bag, agentRequest)
	if ai.Bag.InstrumentationMethod == m.CopyAttach {
		jarPath = fmt.Sprintf("%s/%s", jarPath, "AppServerAgent")
	}
	//annotate pod
	if podObj.Annotations == nil {
		podObj.Annotations = make(map[string]string)
	}
	//annotate pod
	podObj.Annotations[ATTACHED_ANNOTATION] = time.Now().String()

	associateError := ""

	//associate with the tier in case node is not accessible
	containerAppID := fmt.Sprintf("%s_%s", agentRequest.ContainerName, APPD_APPID)
	containerTierID := fmt.Sprintf("%s_%s", agentRequest.ContainerName, APPD_TIERID)
	containerNodeID := fmt.Sprintf("%s_%s", agentRequest.ContainerName, APPD_NODEID)
	appID, tierID, _, errAppd := ai.AppdController.DetermineNodeID(agentRequest.AppName, agentRequest.TierName, "")
	if errAppd == nil {
		if podObj.Annotations[APPD_APPID] == "" {
			podObj.Annotations[APPD_APPID] = strconv.Itoa(appID)
			podObj.Annotations[APPD_TIERID] = strconv.Itoa(tierID)
		}
		podObj.Annotations[containerAppID] = strconv.Itoa(appID)
		podObj.Annotations[containerTierID] = strconv.Itoa(tierID)
	}
	if appID == 0 {
		//mark to retry to give the agent time to initialize
		fmt.Printf("Association error. No appID\n")
		associateError = APPD_ASSOCIATE_ERROR
	}

	if agentRequest.Tech == m.Java {
		//determine the AppD node name
		findVerCmd := fmt.Sprintf("find %s/ -maxdepth 1 -type d -name '*ver*' -printf %%f -quit", jarPath)
		cVer, verFolderName, errVer := exec.RunCommandInPod(podObj.Name, podObj.Namespace, agentRequest.ContainerName, "", findVerCmd)
		fmt.Printf("AppD agent version probe. Output: %s. Error: %v\n", verFolderName, errVer)
		if cVer == 0 && verFolderName != "" {
			findCmd := fmt.Sprintf("find %s/%s/logs/ -maxdepth 1 -type d -name '*%s*' -printf %%f -quit", jarPath, verFolderName, agentRequest.TierName)
			c, folderName, errFolder := exec.RunCommandInPod(podObj.Name, podObj.Namespace, agentRequest.ContainerName, "", findCmd)
			fmt.Printf("AppD agent version probe. Output: %s. Error: %v\n", folderName, errFolder)

			if c == 0 {
				nodeName := strings.TrimSpace(folderName)
				if nodeName != "" {
					podObj.Annotations[APPD_NODENAME] = nodeName
					app, tier, nodeID, errAppd := ai.AppdController.DetermineNodeID(agentRequest.AppName, agentRequest.TierName, nodeName)
					if errAppd == nil {
						if podObj.Annotations[APPD_NODEID] == "" {
							podObj.Annotations[APPD_NODEID] = strconv.Itoa(nodeID)
						}
						if podObj.Annotations[APPD_APPID] == "" {
							podObj.Annotations[APPD_APPID] = strconv.Itoa(app)
						}
						if podObj.Annotations[APPD_TIERID] == "" {
							podObj.Annotations[APPD_TIERID] = strconv.Itoa(tier)
						}
						associateError = ""
						podObj.Annotations[containerAppID] = strconv.Itoa(app)
						podObj.Annotations[containerTierID] = strconv.Itoa(tier)
						podObj.Annotations[containerNodeID] = strconv.Itoa(nodeID)
						//TODO: if analytics was intstrumented, queue up to enable the app for analytics
					}
				} else {
					fmt.Printf("Association error. Empty node name: %s (%s)\n", nodeName, folderName)
					associateError = APPD_ASSOCIATE_ERROR
				}
			}
		}
	}
	if appID > 0 && agentRequest.BiQRequested() {
		analyticsErr := ai.AppdController.EnableAppAnalytics(appID, agentRequest.AppName)
		if analyticsErr != nil {
			fmt.Printf("Unable to enable analytics for application %s\n", agentRequest.AppName)
		}
	}
	_, updateErr := ai.ClientSet.Core().Pods(podObj.Namespace).Update(podObj)
	if updateErr != nil {
		fmt.Printf("%s, Error: %v\n", ANNOTATION_UPDATE_ERROR, updateErr)
		return fmt.Errorf("%s, Error: %v\n", ANNOTATION_UPDATE_ERROR, updateErr)
	}
	if associateError != "" {
		fmt.Printf("Associate error is not empty %s\n", associateError)
		return fmt.Errorf("%s", associateError)
	}
	return nil
}

func GetVolumePath(bag *m.AppDBag, r *m.AgentRequest) string {
	return fmt.Sprintf("%s-%s", bag.AgentMountPath, string(r.Tech))
}
