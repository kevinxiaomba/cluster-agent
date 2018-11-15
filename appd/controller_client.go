package controller

import (
	"fmt"
	"io"
	"log"
	"os"

	m "github.com/sjeltuhin/clusterAgent/models"

	appd "appdynamics"
)

type ControllerClient struct {
	logger *log.Logger
	Bag    *m.AppDBag
}

func NewControllerClient(bag *m.AppDBag, logger *log.Logger) *ControllerClient {
	cfg := appd.Config{}

	cfg.AppName = bag.AppName
	cfg.TierName = bag.TierName
	cfg.NodeName = bag.NodeName
	cfg.Controller.Host = bag.ControllerUrl
	cfg.Controller.Port = bag.ControllerPort
	cfg.Controller.UseSSL = bag.SSLEnabled
	if bag.SSLEnabled {
		err := writeSSLFromEnv(bag, logger)
		if err != nil {
			logger.Printf("Unable to set SSL certificates. Using system certs %s", bag.SystemSSLCert)
			cfg.Controller.CertificateFile = bag.SystemSSLCert

		} else {
			logger.Printf("Setting agent certs to %s", bag.AgentSSLCert)
			cfg.Controller.CertificateFile = bag.AgentSSLCert
		}
	}
	cfg.Controller.Account = bag.Account
	cfg.Controller.AccessKey = bag.AccessKey
	cfg.UseConfigFromEnv = true
	cfg.InitTimeoutMs = 1000
	cfg.Logging.BaseDir = "__console__"
	cfg.Logging.MinimumLevel = appd.APPD_LOG_LEVEL_DEBUG
	if err := appd.InitSDK(&cfg); err != nil {
		logger.Printf("Error initializing the AppDynamics SDK. %v\n", err)
	} else {
		logger.Printf("Initialized AppDynamics SDK successfully\n")
	}
	logger.Println(&cfg.Controller)

	return &ControllerClient{Bag: bag, logger: logger}
}

func (c *ControllerClient) PostMetrics(metrics m.AppDMetricList) error {
	c.logger.Println("Pushing Metrics through the agent:")
	bt := appd.StartBT("PostMetrics", "")
	for _, metric := range metrics.Items {
		metric.MetricPath = fmt.Sprintf(metric.MetricPath, c.Bag.TierName)
		c.logger.Println(metric)
		appd.AddCustomMetric("", metric.MetricPath,
			metric.MetricTimeRollUpType,
			metric.MetricClusterRollUpType,
			appd.APPD_HOLEHANDLING_TYPE_REGULAR_COUNTER)
		appd.ReportCustomMetric(c.Bag.AppName, metric.MetricPath, metric.MetricValue)
	}
	appd.EndBT(bt)
	c.logger.Println("Done pushing Metrics through the agent")

	return nil
}

func writeSSLFromEnv(bag *m.AppDBag, logger *log.Logger) error {
	from, err := os.Open(bag.SystemSSLCert)
	if err != nil {
		logger.Println(err)
		return err
	}
	defer from.Close()

	logger.Printf("Copying system certificates from %s to %s \n", bag.SystemSSLCert, bag.AgentSSLCert)
	to, err := os.OpenFile(bag.AgentSSLCert, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		logger.Println(err)
		return err
	}
	defer to.Close()

	_, err = io.Copy(to, from)
	if err != nil {
		logger.Println(err)
		return err
	}

	logger.Println("Writing Trusted certificates to %s", bag.AgentSSLCert)
	trustedCerts := os.Getenv("APPD_TRUSTED_CERTS")
	if trustedCerts != "NOTSET" {
		if _, err = to.Write([]byte(trustedCerts)); err != nil {
			logger.Println(err)
			return err
		}
	} else {
		logger.Println("Trusted certificates not found, skipping")
	}

	return nil
}
