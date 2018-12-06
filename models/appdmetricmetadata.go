package models

type AppDMetricMetadata struct {
	Name         string
	ParentSchema string
	Path         string
	Query        string
}

func NewAppDMetricMetadata(name, schema, path, query string) AppDMetricMetadata {
	return AppDMetricMetadata{Name: name, ParentSchema: schema, Path: path, Query: query}
}
