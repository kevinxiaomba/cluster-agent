package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/appdynamics/cluster-agent/config"
	m "github.com/appdynamics/cluster-agent/models"
	"github.com/appdynamics/cluster-agent/version"
	"github.com/gorilla/mux"
)

type AgentWebServer struct {
	ConfigManager *config.MutexConfigManager
	Logger        *log.Logger
}

func NewAgentWebServer(c *config.MutexConfigManager, l *log.Logger) *AgentWebServer {
	aws := AgentWebServer{ConfigManager: c, Logger: l}
	return &aws
}

func (ws *AgentWebServer) RunServer() {
	bag := ws.ConfigManager.Conf
	r := mux.NewRouter()
	r.HandleFunc("/version", ws.getVersion)
	r.HandleFunc("/status", ws.getStatus)
	addr := fmt.Sprintf(":%d", bag.AgentServerPort)
	server := &http.Server{Addr: addr, Handler: r}

	go func() {
		ws.Logger.Infof("Starting internal web server on port %d\n", bag.AgentServerPort)
		if err := server.ListenAndServe(); err != nil {
			ws.Logger.Errorln(err)
		}
	}()

	c := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)

	// Block until we receive our signal.
	<-c
	var wait time.Duration = 30
	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	server.Shutdown(ctx)

	ws.Logger.Info("Internal Web Server. Shutting down...")
	os.Exit(0)

}

func (ws *AgentWebServer) getVersion(w http.ResponseWriter, req *http.Request) {
	io.WriteString(w, version.Version)
}

func (ws *AgentWebServer) getStatus(w http.ResponseWriter, req *http.Request) {
	bag := ws.ConfigManager.Conf
	if req.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		statusObj := m.AgentStatus{}
		statusObj.Version = version.Version
		statusObj.NsToMonitor = bag.NsToMonitor
		statusObj.NodesToMonitor = bag.NodesToMonitor

		statusObj.InstrumentationMethod = bag.InstrumentationMethod
		statusObj.DefaultInstrumentationTech = bag.DefaultInstrumentationTech
		statusObj.NsToInstrument = bag.NsToInstrument
		statusObj.NSInstrumentRule = bag.NSInstrumentRule
		statusObj.AnalyticsAgentImage = bag.AnalyticsAgentImage
		statusObj.AppDJavaAttachImage = bag.AppDJavaAttachImage
		statusObj.AppDDotNetAttachImage = bag.AppDDotNetAttachImage
		statusObj.AnalyticsAgentImage = bag.AnalyticsAgentImage
		statusObj.BiqService = bag.BiqService
		statusObj.InstrumentMatchString = bag.InstrumentMatchString

		statusObj.LogLevel = bag.LogLevel
		statusObj.LogLines = bag.LogLines
		statusObj.MetricsSyncInterval = bag.MetricsSyncInterval
		statusObj.SnapshotSyncInterval = bag.SnapshotSyncInterval

		result, _ := json.Marshal(statusObj)
		io.WriteString(w, string(result))
	} else {
		http.Error(w, "Only GET is supported", 404)
	}
}
