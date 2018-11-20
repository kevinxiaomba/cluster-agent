package controller

import (
	"bytes"
	"encoding/base64"
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
	if err != nil {
		fmt.Printf("Unable to initiate request. %v", err)
		return false
	} else {
		req.Header.Set("Accept", "application/vnd.appd.events+json;v=2")
		req.Header.Set("Content-Type", "application/vnd.appd.events+json;v=2")
		req.Header.Set("X-Events-API-AccountName", rc.Bag.GlobalAccount)
		req.Header.Set("X-Events-API-Key", rc.Bag.EventKey)
		fmt.Printf("Sending request. Account: %s   Event Key", rc.Bag.GlobalAccount, rc.Bag.EventKey)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("Unable to check event schema %s. %v", schemaName, err)
			return false
		}
		if resp != nil && resp.Body != nil {
			fmt.Println("response Status:", resp.Status)
			fmt.Println("response Headers:", resp.Header)

			defer resp.Body.Close()
			body, _ := ioutil.ReadAll(resp.Body)
			fmt.Println("response Body:", string(body))
			return resp.StatusCode == 200 && body != nil && len(body) > 0
		} else {
			return false
		}
	}
}

func (rc *RestClient) CreateSchema(schemaName string, data []byte) []byte {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/events/schema/%s", rc.Bag.EventServiceUrl, schemaName), bytes.NewBuffer(data))
	if err != nil {
		fmt.Printf("Unable to initiate request. %v", err)
		return nil
	} else {
		req.Header.Set("Accept", "application/vnd.appd.events+json;v=2")
		req.Header.Set("Content-Type", "application/vnd.appd.events+json;v=2")
		req.Header.Set("X-Events-API-AccountName", rc.Bag.GlobalAccount)
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
}

func (rc *RestClient) PostAppDEvents(schemaName string, data []byte) []byte {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/events/publish/%s", rc.Bag.EventServiceUrl, schemaName), bytes.NewBuffer(data))
	req.Header.Set("Accept", "application/vnd.appd.events+json;v=2")
	req.Header.Set("Content-Type", "application/vnd.appd.events+json;v=2")
	req.Header.Set("X-Events-API-AccountName", rc.Bag.GlobalAccount)
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

func (rc *RestClient) GetRestAuth() AppDRestAuth {
	restAuth := NewRestAuth("", "")
	controllerHost := rc.Bag.ControllerUrl
	if rc.Bag.SSLEnabled {
		controllerHost = fmt.Sprintf("https://%s/controller/", controllerHost)
	} else {
		controllerHost = fmt.Sprintf("http://%s:%d/controller/", controllerHost, rc.Bag.ControllerPort)
	}
	url := controllerHost + "auth?action=login"
	fmt.Printf("Auth url: %s", url)
	bu := []byte(rc.Bag.RestAPICred)
	creds := base64.StdEncoding.EncodeToString(bu)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Issues building request for obtaining session and cookie. %v", err)
	}
	authHeader := "Basic " + creds
	req.Header.Set("Authorization", authHeader)
	fmt.Printf("Header: %s", authHeader)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Issues obtaining session and cookie. %v", err)
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "X-CSRF-TOKEN" {
			restAuth.Token = cookie.Value
		}
		if cookie.Name == "JSESSIONID" {
			restAuth.SessionID = cookie.Value
		}
	}

	return restAuth
}
