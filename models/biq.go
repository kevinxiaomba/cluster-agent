package models

type BiQDeploymentOption string

const (
	None    BiQDeploymentOption = "none"
	Sidecar BiQDeploymentOption = "sidecar"
	Remote  BiQDeploymentOption = "remote"
)
