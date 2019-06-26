package instrumentation

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	app "github.com/appdynamics/cluster-agent/appd"
	appsv1 "k8s.io/api/apps/v1"

	m "github.com/appdynamics/cluster-agent/models"
	"github.com/appdynamics/cluster-agent/utils"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	GET_JAVA_PID_CMD             string = "ps -o pid,comm,args | grep java | awk '{print$1,$2,$3}'"
	ATTACHED_ANNOTATION          string = "appd-attached"
	APPD_ATTACH_PENDING          string = "appd-attach-pending"
	APPD_ATTACH_FAILED           string = "Failed. Image unavailable"
	APPD_ATTACH_DEPLOYMENT       string = "appd-attach-deploy"
	DEPLOY_ANNOTATION            string = "appd-deploy-updated"
	DEPLOY_BIQ_ANNOTATION        string = "appd-deploy-biq-updated"
	APPD_APPID                   string = "appd-appid"
	APPD_TIERID                  string = "appd-tierid"
	APPD_NODEID                  string = "appd-nodeid"
	APPD_AGENT_ID                string = "appd-agent-id"
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
	Logger         *log.Logger
}

func NewAgentInjector(client *kubernetes.Clientset, config *rest.Config, bag *m.AppDBag, appdController *app.ControllerClient, l *log.Logger) AgentInjector {
	return AgentInjector{ClientSet: client, K8sConfig: config, Bag: bag, AppdController: appdController, Logger: l}
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

func IsPodInstrumented(podObj *v1.Pod, l *log.Logger) bool {
	for k, v := range podObj.Annotations {
		if k == ATTACHED_ANNOTATION && v != "" {
			l.Infof("Pod %s already instrumented. Skipping...\n", podObj.Name)
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

func GetAgentRequestsForDeployment(deploy *appsv1.Deployment, bag *m.AppDBag, l *log.Logger) *m.AgentRequestList {
	//check for exclusions
	if bag.InstrumentationMethod == m.None || utils.StringInSlice(deploy.Namespace, bag.NsToInstrumentExclude) {
		l.Infof("Instrumentation is not configured for namespace %s\n", deploy.Namespace)
		return nil
	}

	var list *m.AgentRequestList = nil
	//check deployment labels for instrumentation requests
	var appAgent string
	appName, tierName, biQDeploymentOption := GetAttachMetadata(bag.AppDAppLabel, bag.AppDTierLabel, deploy, bag)

	l.Infof("biQDeploymentOption: %s\n", biQDeploymentOption)
	for k, v := range deploy.Labels {
		if k == bag.AgentLabel {
			appAgent = v
		}
	}

	if appAgent != "" || appName != "" {
		al := m.NewAgentRequestList(appAgent, appName, tierName, biQDeploymentOption, deploy.Spec.Template.Spec.Containers, bag)
		l.Infof("Using deployment metadata for agent request. AppName: %s AppAgent: %s\n", appName, appAgent)
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
					reg, re := regexp.Compile(ms)
					if re != nil {
						l.Errorf("Instrumentation match string %s represents an invalid regex expression. Instrumentation will not be executed. %v\n", ms, re)
					} else {
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
		}

		//in case a namespace-wide rule exists
		if len(arr) == 0 && namespaceRule != nil {
			namespaceRule.AppName, namespaceRule.TierName, _ = GetAttachMetadata(namespaceRule.AppDAppLabel, namespaceRule.AppDTierLabel, deploy, bag)
			arr = append(arr, (*namespaceRule))
		}
		if len(arr) > 0 {
			l.Infof("Applying %d custom rules for agent request\n", len(arr))
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
						globReg, re := regexp.Compile(ms)
						if re != nil {
							l.Errorf("Instrumentation match string %s represents an invalid regex expression. Instrumentation will not be executed. %v\n", ms, re)
						} else {
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
				}
				if global {
					l.Info("Applying global rule for agent request")
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
		l.WithField("list", list.String()).Info("Agent requests")
	} else {
		l.Info("No Agent requests found\n")
	}
	return list
}

func ShouldInstrumentDeployment(deployObj *appsv1.Deployment, bag *m.AppDBag, pendingCache *[]string, failedCache *map[string]m.AttachStatus, l *log.Logger) (bool, bool, *m.AgentRequestList) {
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

	l.Debugf("Update status: %t. BiQ updated: %t\n", updated, biqUpdated)

	if updated || biqUpdated {
		(*pendingCache) = utils.RemoveFromSlice(utils.GetDeployKey(deployObj), *pendingCache)
		l.Infof("Deployment %s already updated for AppD. Skipping...\n", deployObj.Name)
		return false, false, nil
	}

	if utils.StringInSlice(utils.GetDeployKey(deployObj), *pendingCache) {
		l.Infof("Deployment %s is in process of update. Waiting...\n", deployObj.Name)
		return false, false, nil
	}

	//	check Failed cache not to exceed failure limit
	status, ok := (*failedCache)[utils.GetDeployKey(deployObj)]
	if ok && status.Count >= MAX_INSTRUMENTATION_ATTEMPTS {
		l.Errorf("Deployment %s exceeded the max number of failed instrumentation attempts. Skipping...\n", deployObj.Name)
		return false, false, nil
	}

	agentRequests := GetAgentRequestsForDeployment(deployObj, bag, l)
	if agentRequests == nil {
		l.Infof("Deployment %s does not need to be instrumented. Ignoring...", deployObj.Name)
		return false, false, nil
	}

	var biqRequested = agentRequests.BiQRequested()
	initRequested := !AgentInitExists(&deployObj.Spec.Template.Spec, bag) && agentRequests.InitContainerRequired()

	if !biqRequested && !initRequested {
		l.Infof("Instrumentation not requested. Skipping %s...", deployObj.Name)
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
	if IsPodInstrumented(podObj, ai.Logger) {
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
		ai.Logger.Info("Instrumentation not requested. Aborting...")
		statusChanel <- ai.buildAttachStatus(podObj, nil, nil, true)
	} else {
		for _, r := range agentRequests.Items {
			if r.Valid() {
				ai.Logger.Infof("Applying Agent Request from pod annotation: %s\n", r.String())
				c := ai.findContainer(&r, podObj)
				if r.Method == m.MountAttach {
					ai.Logger.Infof("Container %s requested. Instrumenting...", c.Name)
					err := ai.instrumentContainer(r.AppName, r.TierName, c, podObj, m.BiQDeploymentOption(r.BiQ), &r)
					statusChanel <- ai.buildAttachStatus(podObj, &r, err, false)
				} else {
					ai.finilizeAttach(statusChanel, podObj, &r)
				}
			}
		}

	}

	return nil

}

func (ai AgentInjector) finilizeAttach(statusChanel chan m.AttachStatus, podObj *v1.Pod, agentRequest *m.AgentRequest) {
	if agentRequest.Tech == m.DotNet || agentRequest.Method == m.MountEnv {
		ai.Logger.Infof("Finilizing instrumentation for container %s...", agentRequest.ContainerName)
		exec := NewExecutor(ai.ClientSet, ai.K8sConfig, ai.Logger)
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
	exec := NewExecutor(ai.ClientSet, ai.K8sConfig, ai.Logger)

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
				ai.Logger.Debugf("PID: %d, Name: %s, Args: %s\n", pid, procName, args)
				if strings.Contains(procName, "java") {
					procID = pid
				}
			}
		}
		if legit && procID > 0 {
			ai.Logger.Infof("Instrumenting process %d", procID)
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
		ai.Logger.Errorf("Cannot find %s path. %v", filePath, errFile)
		return errFile
	}
	ai.Logger.Infof("Copying file %s", filePath)
	_, _, copyErr := exec.CopyFilesToPod(podObj, containerName, filePath, dir)
	if copyErr != nil {
		ai.Logger.Errorf("Copy failed. Aborting instrumentation. %v\n", copyErr)
		return copyErr
	}
	return nil
}

func (ai AgentInjector) copyFile(ok chan bool, wg *sync.WaitGroup, exec *Executor, fileName, dir string, podObj *v1.Pod, containerName string) {
	defer wg.Done()
	filePath, errFile := filepath.Abs(fileName)
	if errFile != nil {
		ai.Logger.Errorf("Cannot find %s path. %v\n", filePath, errFile)
		ok <- false
	}
	ai.Logger.Infof("Copying file %s", filePath)
	_, _, copyErr := exec.CopyFilesToPod(podObj, containerName, filePath, dir)
	if copyErr != nil {
		ai.Logger.Errorf("Copy failed. Aborting instrumentation. %v", copyErr)
		ok <- false
	}
	ok <- true
}

func (ai AgentInjector) instrument(podObj *v1.Pod, pid int, appName string, tierName string, containerName string, exec *Executor, biQDeploymentOption m.BiQDeploymentOption, agentRequest *m.AgentRequest) error {
	jarPath := GetVolumePath(ai.Bag, agentRequest)

	nodePrefix := ai.Bag.NodeNamePrefix
	if nodePrefix == "" {
		nodePrefix = tierName
	}
	bth := ai.AppdController.StartBT("InstrumentJavaAttach")
	cmd := fmt.Sprintf("java -Xbootclasspath/a:%s/tools.jar -jar %s/javaagent.jar %d appdynamics.controller.hostName=%s,appdynamics.controller.port=%d,appdynamics.controller.ssl.enabled=%t,appdynamics.agent.accountName=%s,appdynamics.agent.accountAccessKey=%s,appdynamics.agent.applicationName=%s,appdynamics.agent.tierName=%s,appdynamics.agent.reuse.nodeName=true,appdynamics.agent.reuse.nodeName.prefix=%s",
		jarPath, jarPath, pid, ai.Bag.ControllerUrl, ai.Bag.ControllerPort, ai.Bag.SSLEnabled, ai.Bag.Account, ai.Bag.AccessKey, appName, tierName, nodePrefix)

	if ai.Bag.AgentLogOverride != "" {
		cmd = fmt.Sprintf("%s,appdynamics.agent.logs.dir=%s", cmd, ai.Bag.AgentLogOverride)
	}

	if ai.Bag.NetVizPort > 0 {
		cmd = fmt.Sprintf("%s,-Dappdynamics.socket.collection.bci.enable=true", cmd)
	}

	//BIQ instrumentation. If Analytics agent is remote, provide the url when attaching
	ai.Logger.Infof("BiQ deployment option is %s.", biQDeploymentOption)
	if agentRequest.IsBiQRemote() {
		ai.Logger.Debugf("Will add remote url %s\n", ai.Bag.AnalyticsAgentUrl)
		if ai.Bag.AnalyticsAgentUrl != "" {
			cmd = fmt.Sprintf("%s,appdynamics.analytics.agent.url=%s/v2/sinks/bt", cmd, ai.Bag.AnalyticsAgentUrl)
		}
	}
	code, output, err := exec.RunCommandInPod(podObj.Name, podObj.Namespace, containerName, "", cmd)
	if code == 0 {
		ai.Logger.Info("AppDynamics Java agent attached.")
		ai.Logger.Debugf("AppDynamics Java agent attached. Output: %s. Error: %v\n", output, err)
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
	exec := NewExecutor(ai.ClientSet, ai.K8sConfig, ai.Logger)
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
		ai.Logger.Warnf("Association error. No appID for app %s", agentRequest.AppName)
		associateError = APPD_ASSOCIATE_ERROR
	}

	if agentRequest.Tech == m.Java {
		//determine the AppD node name
		findVerCmd := fmt.Sprintf("find %s/ -maxdepth 1 -type d -name '*ver*' -printf %%f -quit", jarPath)
		cVer, verFolderName, errVer := exec.RunCommandInPod(podObj.Name, podObj.Namespace, agentRequest.ContainerName, "", findVerCmd)
		ai.Logger.Debugf("AppD agent version probe. Output: %s. Error: %v\n", verFolderName, errVer)
		if cVer == 0 && verFolderName != "" {
			nodePrefix := ai.Bag.NodeNamePrefix
			if nodePrefix == "" {
				nodePrefix = agentRequest.TierName
			}
			findCmd := fmt.Sprintf("find %s/%s/logs/ -maxdepth 1 -type d -name '*%s*' -printf %%f -quit", jarPath, verFolderName, nodePrefix)
			c, folderName, errFolder := exec.RunCommandInPod(podObj.Name, podObj.Namespace, agentRequest.ContainerName, "", findCmd)
			ai.Logger.Debugf("AppD agent version probe. Output: %s. Error: %v\n", folderName, errFolder)

			if c == 0 {
				nodeName := strings.TrimSpace(folderName)
				if nodeName != "" {
					podObj.Annotations[APPD_NODENAME] = nodeName
					app, tier, nodeID, errAppd := ai.AppdController.DetermineNodeID(agentRequest.AppName, agentRequest.TierName, nodeName)
					if errAppd == nil {
						appID = app
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
					ai.Logger.Warnf("Association error. Empty node name: %s (%s)\n", nodeName, folderName)
					associateError = APPD_ASSOCIATE_ERROR
				}
			}
		}
	}
	if appID > 0 && agentRequest.BiQRequested() {
		analyticsErr := ai.AppdController.EnableAppAnalytics(appID, agentRequest.AppName)
		if analyticsErr != nil {
			ai.Logger.Errorf("Unable to turn analytics on for application %s. %v\n", agentRequest.AppName, analyticsErr)
		}
	}
	_, updateErr := ai.ClientSet.CoreV1().Pods(podObj.Namespace).Update(podObj)
	if updateErr != nil {
		ai.Logger.Errorf("%s, Pod update failed: %v\n", ANNOTATION_UPDATE_ERROR, updateErr)
		return fmt.Errorf("%s, Error: %v\n", ANNOTATION_UPDATE_ERROR, updateErr)
	}
	if associateError != "" {
		ai.Logger.Warn("Associate error. Will make another attempt later\n")
		return fmt.Errorf("%s", associateError)
	}
	return nil
}

func GetVolumePath(bag *m.AppDBag, r *m.AgentRequest) string {
	return fmt.Sprintf("%s-%s", bag.AgentMountPath, string(r.Tech))
}
