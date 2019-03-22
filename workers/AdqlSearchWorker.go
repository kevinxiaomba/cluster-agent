package workers

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	app "github.com/sjeltuhin/clusterAgent/appd"
	m "github.com/sjeltuhin/clusterAgent/models"

	"github.com/google/uuid"
)

const (
	DRILL_DOWN_URL_TEMPLATE string = "%s#/location=ANALYTICS_ADQL_SEARCH&searchId=%d"
	BASE_PATH               string = "Application Infrastructure Performance|%s|Custom Metrics|Cluster Stats|"
)

type AdqlSearchWorker struct {
	Bag         *m.AppDBag
	SearchCache map[string]m.AdqlSearch
	Logger      *log.Logger
}

func NewAdqlSearchWorker(bag *m.AppDBag, l *log.Logger) AdqlSearchWorker {
	return AdqlSearchWorker{Bag: bag, SearchCache: make(map[string]m.AdqlSearch), Logger: l}
}

func (aw *AdqlSearchWorker) GetSearch(metricPath string) string {
	search := ""
	if searchObj, ok := aw.SearchCache[metricPath]; ok {
		search = fmt.Sprintf(DRILL_DOWN_URL_TEMPLATE, aw.Bag.RestAPIUrl, searchObj.ID)
	} else {
		if sObj, ok := aw.getQueryMap()[metricPath]; ok {
			obj, err := aw.CreateSearch(&sObj)
			if err != nil {
				aw.Logger.Printf("Unable to save search object. %V", err)
			} else {
				aw.SearchCache[metricPath] = *obj
				search = fmt.Sprintf(DRILL_DOWN_URL_TEMPLATE, aw.Bag.RestAPIUrl, obj.ID)
			}
		}
		//if not in the queryMap, no search is necessary
	}
	return search
}

func (aw *AdqlSearchWorker) CacheSearches() error {
	rc := app.NewRestClient(aw.Bag, aw.Logger)
	data, err := rc.CallAppDController("restui/analyticsSavedSearches/getAllAnalyticsSavedSearches", "GET", nil)

	if err != nil {
		fmt.Printf("Unable to get the list of saved searches. %v\n", err)
		return err
	}
	var list []m.AdqlSearch
	e := json.Unmarshal(data, &list)
	if e != nil {
		fmt.Printf("Unable to deserialize the list of saved searches. %v\n", e)
		return e
	}

	for _, searchObj := range list {
		name := searchObj.SearchName
		if strings.Contains(name, aw.Bag.AppName) {
			aw.SearchCache[name] = searchObj
		}
	}

	return nil
}

func (aw *AdqlSearchWorker) CreateSearch(searchObj *m.AdqlSearch) (*m.AdqlSearch, error) {
	name := uuid.New().String()
	schemaWrapper := searchObj.SchemaDef.Unwrap()
	schemaDef := (*schemaWrapper)["schema"].(map[string]interface{})
	cols := []string{}
	for col, _ := range schemaDef {
		cols = append(cols, col)
	}
	jsonStr := fmt.Sprintf(`{"name":"%s","adqlQueries":["%s"],"searchType":"SINGLE","searchMode":"ADVANCED","viewMode":"DATA","visualization":"TABLE","selectedFields":[%s],"widgets":[],"searchName":"%s"}`, name, searchObj.Query, strings.Join(cols, ","), searchObj.SearchName)

	body, err := json.Marshal(jsonStr)
	if err != nil {
		return nil, fmt.Errorf("Unable to serialize create search request %s. %v", name, err)
	}
	rc := app.NewRestClient(aw.Bag, aw.Logger)
	data, errSave := rc.CallAppDController("restui/analyticsSavedSearches/createAnalyticsSavedSearch", "POST", body)
	if errSave != nil {
		return nil, fmt.Errorf("Unable to create search %s. %v\n", name, errSave)
	}

	var obj m.AdqlSearch
	e := json.Unmarshal(data, &obj)
	if e != nil {
		fmt.Printf("Unable to deserialize new saved searches. %v\n", e)
		return nil, e
	}

	return &obj, nil
}

func (aw *AdqlSearchWorker) getQueryMap() map[string]m.AdqlSearch {
	var queryMap = map[string]m.AdqlSearch{
		BASE_PATH + "EventsCount": m.AdqlSearch{SchemaDef: m.EventSchemaDefWrapper{}, SearchName: "EventsCount", SchemaName: aw.Bag.EventSchemaName,
			Query: fmt.Sprintf(`select * from %s where clusterName = "%s" ORDER BY creationTimestamp DESC`, aw.Bag.EventSchemaName, aw.Bag.AppName)},
	}

	return queryMap
}
