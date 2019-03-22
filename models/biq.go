package models

type BiQDeploymentOption string

const (
	NoBiq   BiQDeploymentOption = "none"
	Sidecar BiQDeploymentOption = "sidecar"
	Remote  BiQDeploymentOption = "remote"
)
