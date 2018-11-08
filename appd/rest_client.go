package controller

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	m "github.com/sjeltuhin/clusterAgent/models"
)

type RestClient struct {
	logger *log.Logger
	Bag    *m.AppDBag
}

func NewRestClient(bag *m.AppDBag, logger *log.Logger) *RestClient {
	return &RestClient{logger, bag}
}

func (rc *RestClient) SchemaExists(schemaName string) bool {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/events/schema/%s", rc.Bag.EventServiceUrl, schemaName), nil)
	req.Header.Set("Accept", "application/vnd.appd.events+json;v=2")
	req.Header.Set("Content-Type", "application/vnd.appd.events+json;v=2")
	req.Header.Set("X-Events-API-AccountName", rc.Bag.Account)
	req.Header.Set("X-Events-API-Key", rc.Bag.EventKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Unable to check event schema %s. %v", schemaName, err)
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))
	return body != nil && len(body) > 0
}

func (rc *RestClient) CreateSchema(schemaName string, data []byte) []byte {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/events/schema/%s", rc.Bag.EventServiceUrl, schemaName), bytes.NewBuffer(data))
	req.Header.Set("Accept", "application/vnd.appd.events+json;v=2")
	req.Header.Set("Content-Type", "application/vnd.appd.events+json;v=2")
	req.Header.Set("X-Events-API-AccountName", rc.Bag.Account)
	req.Header.Set("X-Events-API-Key", rc.Bag.EventKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Unable to create event schema %s. %v", schemaName, err)
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))

	return body
}

func (rc *RestClient) PostAppDEvents(schemaName string, data []byte) []byte {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/events/publish/%s", rc.Bag.EventServiceUrl, schemaName), bytes.NewBuffer(data))
	req.Header.Set("Accept", "application/vnd.appd.events+json;v=2")
	req.Header.Set("Content-Type", "application/vnd.appd.events+json;v=2")
	req.Header.Set("X-Events-API-AccountName", rc.Bag.Account)
	req.Header.Set("X-Events-API-Key", rc.Bag.EventKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Unable to post events. %v", err)
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))
	return body
}
