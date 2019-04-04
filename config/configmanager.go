package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"time"

	m "github.com/sjeltuhin/clusterAgent/models"
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
	Conf  *m.AppDBag
	Mutex *sync.Mutex
	Watch *ConfigWatcher
}

func NewMutexConfigManager(env *m.AppDBag) *MutexConfigManager {
	conf, e := loadConfig(CONFIG_FILE)
	if e != nil {
		fmt.Printf("Config file not found. Using env vars and passed-in parameters\n")
		return &MutexConfigManager{Conf: env, Mutex: &sync.Mutex{}}
	} else {
		fmt.Printf("Using config file %s\n", CONFIG_FILE)
	}
	cm := MutexConfigManager{Conf: conf, Mutex: &sync.Mutex{}}
	watcher, err := WatchFile(CONFIG_FILE, time.Second, cm.onConfigUpdate)
	if err != nil {
		fmt.Printf("Enable to start config watcher. %v", err)
	}
	cm.Watch = watcher
	return &cm
}

func (self *MutexConfigManager) onConfigUpdate() {
	fmt.Printf("Config file Updated\n")
	conf, e := loadConfig(CONFIG_FILE)
	if e != nil {
		fmt.Printf("Unable to read the config file. %v", e)
		return
	}
	self.Set(conf)
}

func (self *MutexConfigManager) Set(conf *m.AppDBag) {
	self.Mutex.Lock()
	if self.Conf != nil && self.Conf.SchemaUpdateCache != nil {
		conf.SchemaUpdateCache = self.Conf.SchemaUpdateCache
	}
	self.Conf = conf
	if self.Conf.NSInstrumentRule == nil {
		self.Conf.NSInstrumentRule = []m.AgentRequest{}
	}
	if self.Conf.InstrumentMatchString == nil {
		self.Conf.InstrumentMatchString = []string{}
	}
	if self.Conf.SchemaUpdateCache == nil {
		self.Conf.SchemaUpdateCache = []string{}
	}
	self.Mutex.Unlock()
	fmt.Printf("Config bag: %v\n", self.Conf)
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

func loadConfig(configFile string) (*m.AppDBag, error) {
	if _, e := os.Stat(configFile); e != nil {
		return nil, e
	}
	conf := &m.AppDBag{}
	configData, err := ioutil.ReadFile(configFile)
	if err != nil {
		fmt.Printf("Cannot read the config file. %v", err)
		return conf, err
	}

	err = json.Unmarshal(configData, conf)
	if err != nil {
		fmt.Printf("Cannot deserialize the config file. %v", err)
		return conf, err
	}
	return conf, nil
}
