package models

type AppDBag struct {
	AppName         string
	TierName        string
	NodeName        string
	Account         string
	AccessKey       string
	ControllerUrl   string
	ControllerPort  uint16
	SSLEnabled      bool
	SystemSSLCert   string
	AgentSSLCert    string
	EventKey        string
	EventServiceUrl string
	RestAPICred     string
}
