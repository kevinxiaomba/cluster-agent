package controller

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"

	log "github.com/sirupsen/logrus"

	m "github.com/appdynamics/cluster-agent/models"
	"github.com/appdynamics/cluster-agent/utils"
)

type RestClient struct {
	logger *log.Logger
	Bag    *m.AppDBag
}

const (
	MAX_FIELD_LENGTH int = 3000
)

func NewRestClient(bag *m.AppDBag, logger *log.Logger) *RestClient {
	return &RestClient{logger, bag}
}

func (rc *RestClient) getClient() *http.Client {
	if rc.Bag.ProxyUrl != "" {
		proxyUrl, err := url.Parse(rc.Bag.ProxyUrl)
		if err != nil {
			rc.logger.Error("Proxy url is invalid")
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

func (rc *RestClient) LoadSchema(schemaName string) (*map[string]interface{}, error) {
	var schemaWrapper map[string]interface{}
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/events/schema/%s", rc.Bag.EventServiceUrl, schemaName), nil)
	if err != nil {
		rc.logger.Errorf("Unable to initiate request. %v", err)
		return nil, err
	} else {
		req.Header.Set("Accept", "application/vnd.appd.events+json;v=2")
		req.Header.Set("Content-Type", "application/vnd.appd.events+json;v=2")
		req.Header.Set("X-Events-API-AccountName", rc.Bag.GlobalAccount)
		req.Header.Set("X-Events-API-Key", rc.Bag.EventKey)
		//		fmt.Printf("Sending request. Account: %s   Event Key %s", rc.Bag.GlobalAccount, rc.Bag.EventKey)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			rc.logger.Errorf("Unable to load event schema %s. %v", schemaName, err)
			return nil, err
		}
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
			body, _ := ioutil.ReadAll(resp.Body)

			if body == nil || len(body) == 0 {
				return nil, fmt.Errorf("Schema %s does not exist\n", schemaName)
			}

			if resp.StatusCode == 404 {
				return nil, fmt.Errorf("Schema %s does not exist\n", schemaName)
			}

			errJson := json.Unmarshal(body, &schemaWrapper)
			if errJson != nil {
				return nil, fmt.Errorf("Unable to deserialize the schemaWrapper. %v", errJson)
			}
			return &schemaWrapper, nil
		} else {
			return nil, fmt.Errorf("Unable to load event schema %s\n", schemaName)
		}
	}
}

func (rc *RestClient) SchemaExists(schemaName string) bool {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/events/schema/%s", rc.Bag.EventServiceUrl, schemaName), nil)
	if err != nil {
		rc.logger.Errorf("Unable to initiate request to check schema %s. %v", schemaName, err)
		return false
	} else {
		req.Header.Set("Accept", "application/vnd.appd.events+json;v=2")
		req.Header.Set("Content-Type", "application/vnd.appd.events+json;v=2")
		req.Header.Set("X-Events-API-AccountName", rc.Bag.GlobalAccount)
		req.Header.Set("X-Events-API-Key", rc.Bag.EventKey)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			rc.logger.Errorf("Unable to check event schema %s. %v", schemaName, err)
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

func (rc *RestClient) EnsureSchema(schemaName string, current m.AppDSchemaInterface) error {
	if rc.Bag.SchemaUpdateCache == nil {
		rc.Bag.SchemaUpdateCache = []string{}
	}

	schemaTypeName := reflect.TypeOf(current).Name()

	//if the schema is marked 'SKIP'
	if strings.Contains(strings.ToLower(schemaName), "skip") {
		ar := strings.Split(schemaName, "^")
		if len(ar) == 2 {
			oldName := ar[0]
			if rc.Bag.SchemaSkipCache == nil {
				rc.Bag.SchemaSkipCache = []string{}
			}

			if !utils.StringInSlice(schemaTypeName, rc.Bag.SchemaSkipCache) {
				rc.Bag.SchemaSkipCache = append(rc.Bag.SchemaSkipCache, schemaTypeName)
				err := rc.DeleteSchema(oldName)
				if err != nil {
					return fmt.Errorf("The schema %s was marked for deletion, but Delete call failed. %v", oldName, err)
				}
				rc.logger.Infof("Schema %s deleted per user request", oldName)
				index := utils.GetIndex(oldName, rc.Bag.SchemaUpdateCache)
				if index >= 0 {
					rc.Bag.SchemaUpdateCache[index] = rc.Bag.SchemaUpdateCache[len(rc.Bag.SchemaUpdateCache)-1]
					rc.Bag.SchemaUpdateCache = rc.Bag.SchemaUpdateCache[:len(rc.Bag.SchemaUpdateCache)-1]
				}
			}
			return nil
		} else {
			return fmt.Errorf("Schema name %s is invalid", schemaName)
		}
	}

	//perform validity check

	if rc.Bag.SchemaUpdateCache != nil && utils.StringInSlice(schemaName, rc.Bag.SchemaUpdateCache) {
		return nil
	} else {
		rc.Bag.SchemaUpdateCache = append(rc.Bag.SchemaUpdateCache, schemaName)
		skipIndex := utils.GetIndex(schemaTypeName, rc.Bag.SchemaSkipCache)
		if skipIndex >= 0 {
			rc.Bag.SchemaSkipCache[skipIndex] = rc.Bag.SchemaSkipCache[len(rc.Bag.SchemaSkipCache)-1]
			rc.Bag.SchemaSkipCache = rc.Bag.SchemaSkipCache[:len(rc.Bag.SchemaSkipCache)-1]
		}

	}
	wrapper, err := rc.LoadSchema(schemaName)
	if err != nil {
		rc.logger.Warnf("Unable load existing schema %s. Attempting to create. %v\n", schemaName, err)
		//try to create
		schemaDef, e := json.Marshal(current)
		if e != nil {
			return fmt.Errorf("Unable to serialize the current schema %s. %v", schemaName, e)
		}
		_, err = rc.CreateSchema(schemaName, schemaDef)
		if err != nil {
			return fmt.Errorf("Unable to recreate the current schema %s. %v", schemaName, err)
		}
		rc.logger.Infof("Schema %s created. \n", schemaName)
		return nil
	}
	equal := true
	currentMap := current.Unwrap()
	currentSchema := (*currentMap)["Schema"]
	currentObj := currentSchema.(map[string]interface{})

	if schema, ok := (*wrapper)["schema"]; ok {
		schemaObj := schema.(map[string]interface{})
		//delete built in fields
		delete(schemaObj, "pickupTimestamp")
		delete(schemaObj, "eventTimestamp")

		if len(currentObj) != len(schemaObj) {
			rc.logger.Infof("Schema %s has changed, %d vs %d fields...\n", schemaName, len(currentObj), len(schemaObj))
			equal = false
		}
		if equal {
			for k, v := range currentObj {
				currentVal, okCur := utils.MapContainsNocase(schemaObj, k)
				if !okCur || currentVal != v {
					rc.logger.Infof("Schema %s has descrepancy in field %s: %v != %v ...\n", schemaName, k, currentVal, v)
					equal = false
					break
				}
			}
		}
	} else {
		equal = false //invalid object
	}

	if !equal {
		rc.logger.Infof("Schema %s has changed, recreating...\n", schemaName)
		err := rc.DeleteSchema(schemaName)
		if err != nil {
			return fmt.Errorf("Unable to delete changed schema. %v", err)
		}
		rc.logger.Infof("Deleted the old schema %s. \n", schemaName)
		schemaDef, e := json.Marshal(current)
		if e != nil {
			return fmt.Errorf("Unable to serialize the current schema %s. %v", schemaName, e)
		}
		_, err = rc.CreateSchema(schemaName, schemaDef)
		if err != nil {
			return fmt.Errorf("Unable to recreate the current schema %s. %v", schemaName, err)
		}
		rc.logger.Infof("Schema %s created. \n", schemaName)

	} else {
		rc.logger.Infof("Schema %s has not changed.\n", schemaName)
	}
	return nil
}

func (rc *RestClient) DeleteSchema(schemaName string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/events/schema/%s", rc.Bag.EventServiceUrl, schemaName), nil)
	if err != nil {
		rc.logger.Errorf("Unable to initiate request. %v", err)
		return err
	} else {
		req.Header.Set("Accept", "application/vnd.appd.events+json;v=2")
		req.Header.Set("Content-Type", "application/vnd.appd.events+json;v=2")
		req.Header.Set("X-Events-API-AccountName", rc.Bag.GlobalAccount)
		req.Header.Set("X-Events-API-Key", rc.Bag.EventKey)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			rc.logger.Errorf("Unable to delete event schema %s. %v", schemaName, err)
			return err
		}
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			} else {
				return fmt.Errorf("Unable to delete schema. %v\n", resp)
			}

		} else {
			return fmt.Errorf("Unable to delete schema. Unknown error\n")
		}
	}
	return nil

}

func (rc *RestClient) CreateSchema(schemaName string, data []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/events/schema/%s", rc.Bag.EventServiceUrl, schemaName), bytes.NewBuffer(data))
	if err != nil {
		rc.logger.Errorf("Unable to initiate request. %v", err)
		return nil, err
	} else {
		req.Header.Set("Accept", "application/vnd.appd.events+json;v=2")
		req.Header.Set("Content-Type", "application/vnd.appd.events+json;v=2")
		req.Header.Set("X-Events-API-AccountName", rc.Bag.GlobalAccount)
		req.Header.Set("X-Events-API-Key", rc.Bag.EventKey)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			rc.logger.Errorf("Unable to create event schema %s. %v", schemaName, err)
			return nil, err
		}
		defer resp.Body.Close()

		rc.logger.Debugf("Create schema %s response Status: %s", schemaName, resp.Status)
		body, _ := ioutil.ReadAll(resp.Body)
		rc.logger.Debugf("response Body: %s", string(body))

		if resp.StatusCode < 200 || resp.StatusCode > 204 {
			return nil, fmt.Errorf("Controller request failed with status %s. Message: %s", resp.Status, string(body))
		}

		return body, nil
	}
}

func (rc *RestClient) PostAppDEvents(schemaName string, data []byte) []byte {
	rc.logger.Debugf("PostAppDEvents Payload: %s", string(data))
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/events/publish/%s", rc.Bag.EventServiceUrl, schemaName), bytes.NewBuffer(data))
	req.Header.Set("Accept", "application/vnd.appd.events+json;v=2")
	req.Header.Set("Content-Type", "application/vnd.appd.events+json;v=2")
	req.Header.Set("X-Events-API-AccountName", rc.Bag.GlobalAccount)
	req.Header.Set("X-Events-API-Key", rc.Bag.EventKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		rc.logger.Errorf("Unable to post events. %v", err)
	}
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
		rc.logger.Debugf("PostAppDEvents Status: %s", resp.Status)
		body, _ := ioutil.ReadAll(resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode > 204 {
			rc.logger.Debugf("PostAppDEvents Body: %s", string(body))
		}
		return body
	}
	return nil
}

func (rc *RestClient) GetRestAuth() (AppDRestAuth, error) {
	restAuth := NewRestAuth("", "")

	url := rc.getControllerUrl() + "auth?action=login"
	bu := []byte(rc.Bag.RestAPICred)
	creds := base64.StdEncoding.EncodeToString(bu)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		rc.logger.Errorf("Issues building request for obtaining session and cookie. %v", err)
		return restAuth, err
	}
	authHeader := "Basic " + creds
	req.Header.Set("Authorization", authHeader)
	rc.logger.Debugf("Header: %s", authHeader)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		rc.logger.Errorf("Issues obtaining session and cookie. %v", err)
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
	return rc.Bag.RestAPIUrl
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
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-CSRF-TOKEN", auth.Token)
	req.Header.Set("Cookie", auth.getAuthCookie())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		rc.logger.Errorf("Failed to call AppD controller. %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	rc.logger.Debugf("CallAppDController method %s. Response Status: %s", method, resp.Status)
	b, _ := ioutil.ReadAll(resp.Body)
	//	fmt.Println("response Body:", string(b))
	if resp.StatusCode < 200 || resp.StatusCode > 202 {
		return b, fmt.Errorf("Controller request failed with status %s.", resp.Status)
	}
	return b, nil
}

func (rc *RestClient) CreateDashboard(templatePath string) ([]byte, error) {

	url := rc.getControllerUrl() + "CustomDashboardImportExportServlet"

	rc.logger.Debugf("\nCreating dashboard: %s\n", url)

	bu := []byte(rc.Bag.RestAPICred)
	creds := base64.StdEncoding.EncodeToString(bu)

	rc.logger.Debugf("Credentials: %s\n", creds)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	file, err := os.Open(templatePath)
	if err != nil {
		rc.logger.Errorf("Unable to Open template file. %v\n", err)
		return nil, err
	}
	defer file.Close()

	rc.logger.Debugf("\nGot the file: %s\n", file.Name())

	part, err := writer.CreateFormFile("file", file.Name())
	if err != nil {
		fmt.Printf("Unable to create part for file uploads. %v\n", err)
		return nil, err
	}
	_, errC := io.Copy(part, file)
	if errC != nil {
		rc.logger.Errorf("Unable to copy part for file uploads. %v\n", errC)
		return nil, errC
	}

	err = writer.Close()
	if err != nil {
		rc.logger.Errorf("Unable to create request for dashboard post. %v\n", err)
		return nil, err
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		rc.logger.Errorf("Unable to create request for dashboard post. %v\n", err)
	}
	authHeader := "Basic " + creds
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		rc.logger.Errorf("Unable to create dashboard. %v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	rc.logger.Debugf("CreateDashboard response Status:", resp.Status)
	b, _ := ioutil.ReadAll(resp.Body)
	rc.logger.Debugf("CreateDashboard response Body:", string(b))
	return b, nil

}

func (rc *RestClient) MarkNodeHistorical(nodeId int) error {
	url := fmt.Sprintf("%srest/mark-nodes-historical?application-component-node-ids=%d", rc.getControllerUrl(), nodeId)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("Unable to create request for mark node historical. %v\n", err)
	}
	ar := strings.Split(rc.Bag.RestAPICred, ":")
	if len(ar) != 2 {
		return fmt.Errorf("Rest API credentials are formatted incorrectly. Must be <username>@<account>:<password>")
	}

	req.SetBasicAuth(ar[0], ar[1])
	client := &http.Client{}
	_, errReq := client.Do(req)
	if errReq != nil {
		return fmt.Errorf("Unable to mark node as historical. %v\n", errReq)
	}

	return nil
}

func (rc *RestClient) GetControllerVersion() ([]byte, error) {
	url := fmt.Sprintf("%srest/serverstatus", rc.getControllerUrl())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Unable to create request to obtain controller version. %v\n", err)
	}
	ar := strings.Split(rc.Bag.RestAPICred, ":")
	if len(ar) != 2 {
		return nil, fmt.Errorf("Rest API credentials are formatted incorrectly. Must be <username>@<account>:<password>")
	}

	req.SetBasicAuth(ar[0], ar[1])
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		rc.logger.Errorf("Failed to get controller version. %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode > 202 {
		return b, fmt.Errorf("Controller request failed with status %s", resp.Status)
	}

	return b, nil
}

func (rc *RestClient) validateLicense(accountID int) (bool, error) {
	licenseExists := false
	url := fmt.Sprintf("%sapi/accounts/%d/licensemodules", rc.getControllerUrl(), accountID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("Unable to get license information for account %d. %v\n", accountID, err)
	}
	ar := strings.Split(rc.Bag.RestAPICred, ":")
	if len(ar) != 2 {
		return false, fmt.Errorf("Rest API credentials are formatted incorrectly. Must be <username>@<account>:<password>")
	}

	req.SetBasicAuth(ar[0], ar[1])
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		rc.logger.Errorf("Failed to get license information for account %d. %v", accountID, err)
		return false, err
	}
	defer resp.Body.Close()

	rc.logger.Debugf("Call validateLicense. Response Status: %s", resp.Status)
	data, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode > 202 {
		return false, fmt.Errorf("Controller request failed with status %s.", resp.Status)
	}

	var licObj map[string]interface{}
	errJson := json.Unmarshal(data, &licObj)
	if errJson != nil {
		return false, fmt.Errorf("Unable to deserialize licensing data. %v", errJson)
	}
	for k, v := range licObj {
		if k == "modules" {
			mList := v.([]interface{})
			for _, mod := range mList {
				modObj := mod.(map[string]interface{})
				rc.logger.Debugf("Module name: %s\n", modObj["name"])
				if modObj["name"] == "golang-sdk" {
					licenseExists = true
					break
				}
			}
			break
		}
	}

	return licenseExists, nil
}
