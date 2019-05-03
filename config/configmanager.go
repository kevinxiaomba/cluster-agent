package config

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/appdynamics/cluster-agent/utils"

	m "github.com/appdynamics/cluster-agent/models"
	log "github.com/sirupsen/logrus"
)

const CONFIG_FILE = "/opt/appdynamics/config/cluster-agent-config.json"

/*
 Simple interface that allows us to switch out both implementations of the Manager
*/
type ConfigManager interface {
	Set(*m.AppDBag)
	Get() *m.AppDBag
	Close()
}

/*
 This struct manages the configuration instance by
 preforming locking around access to the Config struct.
*/
type MutexConfigManager struct {
	Conf   *m.AppDBag
	Mutex  *sync.Mutex
	Watch  *ConfigWatcher
	Logger *log.Logger
}

func NewMutexConfigManager(env *m.AppDBag, l *log.Logger) *MutexConfigManager {
	conf, e := loadConfig(CONFIG_FILE, l)
	if e != nil {
		l.Warn("Config file not found. Using env vars and passed-in parameters")
		return &MutexConfigManager{Conf: env, Mutex: &sync.Mutex{}}
	} else {
		l.WithField("configFile", CONFIG_FILE).Info("Using config file\n")
	}
	cm := MutexConfigManager{Conf: conf, Mutex: &sync.Mutex{}, Logger: l}
	cm.setDefaults(env)
	watcher, err := WatchFile(CONFIG_FILE, time.Second, cm.onConfigUpdate)
	if err != nil {
		l.WithField("error", err).Error("Enable to start config watcher")
	}
	cm.Watch = watcher
	return &cm
}

func (self *MutexConfigManager) setDefaults(env *m.AppDBag) {
	//set all secrets passed via env vars
	self.Conf.RestAPICred = env.RestAPICred
	self.Conf.AccessKey = env.AccessKey
	self.Conf.EventKey = env.EventKey
	self.Conf.AgentNamespace = env.AgentNamespace

	if self.Conf.DashboardTemplatePath == "" {
		self.Conf.DashboardTemplatePath = env.DashboardTemplatePath
	}
	self.Logger.WithField("path", self.Conf.DashboardTemplatePath).Info("Dashboard template path")
}

func (self *MutexConfigManager) onConfigUpdate() {
	self.Logger.Info("Config file updated")
	conf, e := loadConfig(CONFIG_FILE, self.Logger)
	if e != nil {
		self.Logger.WithField("error", e).Error("Unable to read the config file")
		return
	}
	self.reconcile(conf)
}

func (self *MutexConfigManager) Set(conf *m.AppDBag) {
	self.Mutex.Lock()
	if self.Conf != nil && self.Conf.SchemaUpdateCache != nil {
		conf.SchemaUpdateCache = self.Conf.SchemaUpdateCache
	}

	if self.Conf != nil && self.Conf.SchemaSkipCache != nil {
		conf.SchemaSkipCache = self.Conf.SchemaSkipCache
	}

	self.Conf = conf

	self.validate()

	self.Mutex.Unlock()
	self.Logger.WithField("bag", self.Conf).Debug("Config bag:")
}

func (self *MutexConfigManager) validate() {
	if self.Conf.NSInstrumentRule == nil {
		self.Conf.NSInstrumentRule = []m.AgentRequest{}
	}
	if self.Conf.InstrumentMatchString == nil {
		self.Conf.InstrumentMatchString = []string{}
	}
	if self.Conf.SchemaUpdateCache == nil {
		self.Conf.SchemaUpdateCache = []string{}
	}
	if self.Conf.SchemaSkipCache == nil {
		self.Conf.SchemaSkipCache = []string{}
	}
	//update log level
	l, errLevel := log.ParseLevel(self.Conf.LogLevel)
	if errLevel != nil {
		self.Logger.SetLevel(log.InfoLevel)
		self.Logger.WithField("level", self.Conf.LogLevel).Error("Invalid logging level configured. Setting to Info...")
	} else {
		self.Logger.SetLevel(l)
		self.Logger.WithField("level", l).Info("New logging level configured")
	}

	if self.Conf.ProxyUrl != "" {
		arr := strings.Split(self.Conf.ProxyUrl, ":")
		if len(arr) != 3 {
			self.Logger.Error("ProxyUrl Url is invalid. Use this format: protocol://url:port")
		}
		self.Conf.ProxyHost = strings.TrimLeft(arr[1], "//")
		self.Conf.ProxyPort = arr[2]
	}
	if self.Conf.AnalyticsAgentUrl != "" {
		protocol, host, port, err := utils.SplitUrl(self.Conf.AnalyticsAgentUrl)
		if err != nil {
			self.Logger.Error("Analytics agent Url is invalid. Use this format: protocol://url:port")
		}
		self.Conf.RemoteBiqProtocol = protocol
		self.Conf.RemoteBiqHost = host
		self.Conf.RemoteBiqPort = port
	}
	self.Conf.EnsureDefaults()
}

func (self *MutexConfigManager) reconcile(updated *m.AppDBag) {
	self.Mutex.Lock()
	updatedVal := reflect.ValueOf(*updated)
	currentVal := reflect.ValueOf(self.Conf)
	for i := 0; i < updatedVal.Type().NumField(); i++ {
		field := updatedVal.Type().Field(i)
		if m.IsUpdatable(field.Name) {
			val := updatedVal.FieldByName(field.Name)
			current := currentVal.Elem().FieldByName(field.Name)
			m.UpdateField(field.Name, &current, &val)
			self.Logger.WithFields(log.Fields{"name": field.Name, "value": val}).Debug("Updating configMap field")
		}
	}
	self.validate()
	self.Mutex.Unlock()
}

func (self *MutexConfigManager) Get() *m.AppDBag {
	self.Mutex.Lock()
	temp := self.Conf
	self.Mutex.Unlock()
	return temp
}

func (self *MutexConfigManager) Close() {
	if self.Watch != nil {
		self.Watch.Close()
	}
}

func loadConfig(configFile string, l *log.Logger) (*m.AppDBag, error) {
	if _, e := os.Stat(configFile); e != nil {
		return nil, e
	}
	conf := &m.AppDBag{}
	configData, err := ioutil.ReadFile(configFile)
	if err != nil {
		l.WithField("error", err).Error("Cannot read the config file")
		return conf, err
	}

	err = json.Unmarshal(configData, conf)
	if err != nil {
		l.WithField("error", err).Error("Cannot deserialize the config file")
		return conf, err
	}
	return conf, nil
}
