package models

import (
	"time"

	"k8s.io/api/core/v1"
)

type AttachStatus struct {
	Key              string
	Count            int
	LastAttempt      time.Time
	LastMessage      string
	Success          bool
	RetryAssociation bool
	Request          *AgentRequest
}

type AgentRetryRequest struct {
	Request *AgentRequest
	Pod     *v1.Pod
}
