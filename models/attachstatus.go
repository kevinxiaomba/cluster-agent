package models

import (
	"time"
)

type AttachStatus struct {
	Key         string
	Count       int
	LastAttempt time.Time
	LastMessage string
	Success     bool
}
