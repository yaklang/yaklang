package schema

import "github.com/jinzhu/gorm"

type AIForge struct {
	gorm.Model
	ForgeName     string `gorm:"unique_index"`
	ForgeContent  string
	ForgeType     string // "yak" or "json"
	Params        string // cli params
	DefaultParams string // for user preferences
	Description   string // forge description

	InitPrompt       string
	PersistentPrompt string
	PlanPrompt       string
	ResultPrompt     string
}

var FORGE_TYPE_YAK = "yak"
var FORGE_TYPE_JSON = "json"

func (s *AIForge) AfterCreate(tx *gorm.DB) (err error) {
	broadcastData.Call("aiforge", "create")
	return nil
}

func (s *AIForge) AfterUpdate(tx *gorm.DB) (err error) {
	broadcastData.Call("aiforge", "update")
	return nil
}

func (s *AIForge) AfterDelete(tx *gorm.DB) (err error) {
	broadcastData.Call("aiforge", "delete")
	return nil
}

//todo  schema2grpc model
