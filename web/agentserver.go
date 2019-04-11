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

	"github.com/gorilla/mux"

	m "github.com/sjeltuhin/clusterAgent/models"
)

type AgentWebServer struct {
	Bag *m.AppDBag
}

func NewAgentWebServer(bag *m.AppDBag) *AgentWebServer {
	aws := AgentWebServer{Bag: bag}
	return &aws
}

func (ws *AgentWebServer) RunServer() {
	r := mux.NewRouter()
	r.HandleFunc("/version", ws.getVersion)
	r.HandleFunc("/config", ws.updateConfig)
	addr := fmt.Sprintf(":%d", ws.Bag.AgentServerPort)
	server := &http.Server{Addr: addr, Handler: r}

	go func() {
		log.Printf("Starting internal web server on port %d\n", ws.Bag.AgentServerPort)
		if err := server.ListenAndServe(); err != nil {
			log.Println(err)
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
	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	log.Println("shutting down")
	os.Exit(0)

}

func (ws *AgentWebServer) getVersion(w http.ResponseWriter, req *http.Request) {
	io.WriteString(w, "1.0")
}

func (ws *AgentWebServer) updateConfig(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		decoder := json.NewDecoder(req.Body)
		var bag map[string]interface{}
		err := decoder.Decode(&bag)
		if err != nil {
			http.Error(w, "Bag update failed", 500)
			return
		}
		//		for k, v : range bag{

		//		}

		w.Header().Set("Content-Type", "application/json")
		result, _ := json.Marshal(ws.Bag)
		io.WriteString(w, string(result))
	} else {
		http.Error(w, "Only POST is supported", 404)
	}
}
