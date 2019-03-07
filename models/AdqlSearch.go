package models

type AdqlSearch struct {
	ID         int64  `json:"id"`
	SearchName string `json:"searchName"`
	Query      string `json:"-"`
}
