package yakit

import (
	"encoding/json"
	"github.com/jinzhu/gorm"
	"strconv"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yak/yaklib/codec"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

type MenuItem struct {
	gorm.Model

	Group         string `json:"group" `
	Verbose       string `json:"verbose"`
	YakScriptName string `json:"yak_script_name"`
	Hash          string `json:"-" gorm:"unique_index"`

	// quoted json
	BatchPluginFilterJson string `json:"batch_plugin_filter_json"`
	Mode                  string `json:"mode"`
	MenuSort              int64  `json:"menu_sort"`
	GroupSort             int64  `json:"group_sort"`
}

/*
{"group":"12312","name":"aaa","query":{"type":"mitm,port-scan,nuclei","tags":"","include":["ElasticSearch 未授权访问","[wptouch-open-redirect]: WPTouch Switch Desktop 3.x Open Redirection","[wpmudev-pub-keys]: Wpmudev Dashboard Pub Key","[wpdm-cache-session]: Wpdm-Cache Session"],"exclude":[]}}
*/
type batchExecutionSchema struct {
	Group string                     `json:"group"`
	Name  string                     `json:"name"`
	Query batchExecutionSchemaFilter `json:"query"`
}
type batchExecutionSchemaFilter struct {
	Types   string   `json:"type"`
	Tags    string   `json:"tags"`
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

func NewMenuItemByBatchExecuteConfig(raw interface{}) (*MenuItem, error) {
	jsonBody := utils.InterfaceToBytes(raw)
	var schema batchExecutionSchema
	err := json.Unmarshal(jsonBody, &schema)
	if err != nil {
		return nil, utils.Errorf("cannot load json to (menuItem): %s", err)
	}

	queryFilter, err := json.Marshal(schema.Query)
	if err != nil {
		return nil, utils.Errorf("loading query filter failed: %s", err)
	}
	item := &MenuItem{
		Group:                 schema.Group,
		Verbose:               schema.Name,
		BatchPluginFilterJson: strconv.Quote(string(queryFilter)),
	}
	return item, nil
}

func (m *MenuItem) CalcHash() string {
	key := m.Verbose
	if key == "" {
		key = m.YakScriptName
	}
	if key == "" {
		key = codec.Sha256(m.BatchPluginFilterJson)
	}
	return utils.CalcSha1(m.Group, m.Mode, key)
}

func (m *MenuItem) BeforeSave() error {
	if m.Group == "" {
		m.Group = "UserDefined"
	}

	m.Hash = m.CalcHash()
	return nil
}

func CreateOrUpdateMenuItem(db *gorm.DB, hash string, i interface{}) error {
	db = UserDataAndPluginDatabaseScope(db)

	db = db.Model(&MenuItem{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&MenuItem{}); db.Error != nil {
		return utils.Errorf("create/update MenuItem failed: %s", db.Error)
	}

	return nil
}

func GetMenuItemById(db *gorm.DB, id int64) (*MenuItem, error) {
	db = UserDataAndPluginDatabaseScope(db)

	var req MenuItem
	if db := db.Model(&MenuItem{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get MenuItem failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteMenuItemByID(db *gorm.DB, id int64) error {
	db = UserDataAndPluginDatabaseScope(db)

	if db := db.Model(&MenuItem{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&MenuItem{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteMenuItem(db *gorm.DB, group string, name string, mode string) error {
	db = UserDataAndPluginDatabaseScope(db)
	db = db.Model(&MenuItem{}).Where(
		"`group` = ? AND yak_script_name = ?", group, name,
	)
	if mode != "" {
		db = db.Where("mode = ?", mode)
	}
	db = db.Unscoped().Delete(&MenuItem{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteMenuItemAll(db *gorm.DB) error {
	db = UserDataAndPluginDatabaseScope(db)

	if db := db.Model(&MenuItem{}).Where(
		"true",
	).Unscoped().Delete(&MenuItem{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func GetAllMenuItem(db *gorm.DB) []*MenuItem {
	db = UserDataAndPluginDatabaseScope(db)

	var items []*MenuItem
	if db := db.Model(&MenuItem{}).Where("true").Find(&items); db.Error != nil {
		return nil
	}
	return items
}

func GetMenuItem(db *gorm.DB, group string, name string) (*MenuItem, error) {
	db = UserDataAndPluginDatabaseScope(db)

	var req MenuItem
	if db := db.Model(&MenuItem{}).Where("`group` = ? AND yak_script_string = ?", group, name).First(&req); db.Error != nil {
		return nil, utils.Errorf("get MenuItem failed: %s", db.Error)
	}

	return &req, nil
}

func QueryAllMenuItemByWhere(db *gorm.DB, req *ypb.QueryAllMenuItemRequest) []*MenuItem {
	var items []*MenuItem
	db = UserDataAndPluginDatabaseScope(db)
	db = db.Model(&MenuItem{})
	if req.Mode != "" {
		db = db.Where("mode = ?", req.Mode)
	} else {
		db = db.Where("mode IS NULL OR mode = '' ")
	}
	if req.Group != "" {
		db = db.Where("`group` = ?", req.Group)
	}
	db = db.Order("menu_sort, group_sort asc ").Scan(&items)
	if db.Error != nil {
		return nil
	}
	return items
}
