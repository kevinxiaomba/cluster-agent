package controller

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"

	m "github.com/sjeltuhin/clusterAgent/models"
)

type RestClient struct {
	logger *log.Logger
	Bag    *m.AppDBag
}

const (
	MAX_FIELD_LENGTH int = 4000
)

func NewRestClient(bag *m.AppDBag, logger *log.Logger) *RestClient {
	return &RestClient{logger, bag}
}

func (rc *RestClient) getClient() *http.Client {
	if rc.Bag.ProxyInfo != "" {
		proxyUrl, err := url.Parse(rc.Bag.ProxyInfo)
		if err != nil {
			fmt.Print("Proxy url is invalid")
			return &http.Client{}
		}
		return &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyUrl)}}
	}
	return &http.Client{}
}

func (rc *RestClient) addProxyAuth(req *http.Request) {
	if rc.Bag.ProxyUser != "" && rc.Bag.ProxyPass != "" {
		//adding proxy authentication
		auth := fmt.Sprintf("%s:%s", rc.Bag.ProxyUser, rc.Bag.ProxyPass)
		basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
		req.Header.Add("Proxy-Authorization", basicAuth)
	}
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
		//		fmt.Printf("Sending request. Account: %s   Event Key %s", rc.Bag.GlobalAccount, rc.Bag.EventKey)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("Unable to check event schema %s. %v", schemaName, err)
			return false
		}
		if resp != nil && resp.Body != nil {
			//			fmt.Println("response Status:", resp.Status)

			defer resp.Body.Close()
			body, _ := ioutil.ReadAll(resp.Body)
			//			fmt.Println("response Body:", string(body))
			return resp.StatusCode == 200 && body != nil && len(body) > 0
		} else {
			return false
		}
	}
}

func (rc *RestClient) CreateSchema(schemaName string, data []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/events/schema/%s", rc.Bag.EventServiceUrl, schemaName), bytes.NewBuffer(data))
	if err != nil {
		fmt.Printf("Unable to initiate request. %v", err)
		return nil, err
	} else {
		req.Header.Set("Accept", "application/vnd.appd.events+json;v=2")
		req.Header.Set("Content-Type", "application/vnd.appd.events+json;v=2")
		req.Header.Set("X-Events-API-AccountName", rc.Bag.GlobalAccount)
		req.Header.Set("X-Events-API-Key", rc.Bag.EventKey)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("Unable to create event schema %s. %v", schemaName, err)
			return nil, err
		}
		defer resp.Body.Close()

		fmt.Println("response Status:", resp.Status)
		body, _ := ioutil.ReadAll(resp.Body)
		fmt.Println("response Body:", string(body))

		return body, nil
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

	fmt.Println("PostAppDEvents Status:", resp.Status)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("PostAppDEvents Body:", string(body))
	return body
}

func (rc *RestClient) GetRestAuth() (AppDRestAuth, error) {
	restAuth := NewRestAuth("", "")

	url := rc.getControllerUrl() + "auth?action=login"
	bu := []byte(rc.Bag.RestAPICred)
	creds := base64.StdEncoding.EncodeToString(bu)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Issues building request for obtaining session and cookie. %v", err)
		return restAuth, err
	}
	authHeader := "Basic " + creds
	req.Header.Set("Authorization", authHeader)
	//	fmt.Printf("Header: %s", authHeader)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Issues obtaining session and cookie. %v", err)
		return restAuth, err
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "X-CSRF-TOKEN" {
			restAuth.Token = cookie.Value
		}
		if cookie.Name == "JSESSIONID" {
			restAuth.SessionID = cookie.Value
		}
	}

	return restAuth, nil
}

func (rc *RestClient) getControllerUrl() string {
	controllerHost := rc.Bag.ControllerUrl
	if rc.Bag.SSLEnabled {
		controllerHost = fmt.Sprintf("https://%s/controller/", controllerHost)
	} else {
		controllerHost = fmt.Sprintf("http://%s:%d/controller/", controllerHost, rc.Bag.ControllerPort)
	}
	return controllerHost
}

func (rc *RestClient) CallAppDController(path, method string, data []byte) ([]byte, error) {
	auth, err := rc.GetRestAuth()
	if err != nil {
		return nil, fmt.Errorf("Auth failed. Cannot call AppD controller")
	}

	url := rc.getControllerUrl() + path
	var body io.Reader = nil
	if data != nil {
		body = bytes.NewBuffer(data)
	}
	req, err := http.NewRequest(method, url, body)
	req.Header.Set("Accept", "application/json")
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-CSRF-TOKEN", auth.Token)
	req.Header.Set("Cookie", auth.getAuthCookie())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Unable to post events. %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	//	fmt.Println("response Status:", resp.Status)
	b, _ := ioutil.ReadAll(resp.Body)
	//	fmt.Println("response Body:", string(b))
	return b, nil
}

func (rc *RestClient) CreateDashboard(templatePath string) ([]byte, error) {

	url := rc.getControllerUrl() + "CustomDashboardImportExportServlet"

	fmt.Printf("\nCreating dashboard: %s\n", url)

	bu := []byte(rc.Bag.RestAPICred)
	creds := base64.StdEncoding.EncodeToString(bu)

	fmt.Printf("Credentials: %s\n", creds)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	file, err := os.Open(templatePath)
	if err != nil {
		fmt.Printf("Unable to Open template file. %v\n", err)
		return nil, err
	}
	defer file.Close()

	fmt.Printf("\nGot the file: %s\n", file.Name())

	part, err := writer.CreateFormFile("file", file.Name())
	if err != nil {
		fmt.Printf("Unable to create part for file uploads. %v\n", err)
		return nil, err
	}
	_, errC := io.Copy(part, file)
	if errC != nil {
		fmt.Printf("Unable to copy part for file uploads. %v\n", errC)
		return nil, errC
	}

	err = writer.Close()
	if err != nil {
		fmt.Printf("Unable to create request for dashboard post. %v\n", err)
		return nil, err
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		fmt.Printf("Unable to create request for dashboard post. %v\n", err)
	}
	authHeader := "Basic " + creds
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Unable to create dashboard. %v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	b, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(b))
	return b, nil

}
