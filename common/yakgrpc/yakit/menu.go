package yakit

import (
	"encoding/json"
	"strconv"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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

func NewMenuItemByBatchExecuteConfig(raw interface{}) (*schema.MenuItem, error) {
	jsonBody := utils.InterfaceToBytes(raw)
	var executionSchema batchExecutionSchema
	err := json.Unmarshal(jsonBody, &executionSchema)
	if err != nil {
		return nil, utils.Errorf("cannot load json to (menuItem): %s", err)
	}

	queryFilter, err := json.Marshal(executionSchema.Query)
	if err != nil {
		return nil, utils.Errorf("loading query filter failed: %s", err)
	}
	item := &schema.MenuItem{
		Group:                 executionSchema.Group,
		Verbose:               executionSchema.Name,
		BatchPluginFilterJson: strconv.Quote(string(queryFilter)),
	}
	return item, nil
}

func CreateOrUpdateMenuItem(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&schema.MenuItem{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.MenuItem{}); db.Error != nil {
		return utils.Errorf("create/update MenuItem failed: %s", db.Error)
	}

	return nil
}

func GetMenuItemById(db *gorm.DB, id int64) (*schema.MenuItem, error) {
	var req schema.MenuItem
	if db := db.Model(&schema.MenuItem{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get MenuItem failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteMenuItemByID(db *gorm.DB, id int64) error {
	if db := db.Model(&schema.MenuItem{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&schema.MenuItem{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteMenuItem(db *gorm.DB, group string, name string, mode string) error {
	db = db.Model(&schema.MenuItem{}).Where(
		"`group` = ? AND yak_script_name = ?", group, name,
	)
	if mode != "" {
		db = db.Where("mode = ?", mode)
	}
	db = db.Unscoped().Delete(&schema.MenuItem{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteMenuItemAll(db *gorm.DB) error {
	if db := db.Model(&schema.MenuItem{}).Where(
		"true",
	).Unscoped().Delete(&schema.MenuItem{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func GetAllMenuItem(db *gorm.DB) []*schema.MenuItem {
	var items []*schema.MenuItem
	if db := db.Model(&schema.MenuItem{}).Where("true").Find(&items); db.Error != nil {
		return nil
	}
	return items
}

func GetMenuItem(db *gorm.DB, group string, name string) (*schema.MenuItem, error) {
	var req schema.MenuItem
	if db := db.Model(&schema.MenuItem{}).Where("`group` = ? AND yak_script_string = ?", group, name).First(&req); db.Error != nil {
		return nil, utils.Errorf("get MenuItem failed: %s", db.Error)
	}

	return &req, nil
}

func QueryAllMenuItemByWhere(db *gorm.DB, req *ypb.QueryAllMenuItemRequest) []*schema.MenuItem {
	var items []*schema.MenuItem

	db = db.Model(&schema.MenuItem{})
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
