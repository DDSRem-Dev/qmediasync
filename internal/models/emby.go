package models

type EmbyItem struct {
	BaseModel
	Name   string `json:"name"`
	ItemId int64  `json:"item_id"`
}
