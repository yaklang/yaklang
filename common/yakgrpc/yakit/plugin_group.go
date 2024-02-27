package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

type PluginGroup struct {
	gorm.Model

	YakScriptName string `json:"yak_script_name" gorm:"index"`
	Group         string `json:"group"`
	Hash          string `json:"hash" gorm:"unique_index"`
}

func (p *PluginGroup) CalcHash() string {
	return utils.CalcSha1(p.YakScriptName, p.Group)
}

func CreateOrUpdatePluginGroup(db *gorm.DB, hash string, i interface{}) error {
	yakScriptOpLock.Lock()
	db = db.Model(&PluginGroup{})
	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&PluginGroup{}); db.Error != nil {
		return utils.Errorf("create/update PluginGroup failed: %s", db.Error)
	}
	yakScriptOpLock.Unlock()
	return nil
}

func DeletePluginGroupByHash(db *gorm.DB, hash string) error {
	db = db.Model(&PluginGroup{}).Where("hash = ?", hash).Unscoped().Delete(&PluginGroup{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}

func GetPluginByGroup(db *gorm.DB, group string) (req []*PluginGroup, err error) {
	db = db.Model(&PluginGroup{}).Where("`group` = ?", group).Scan(&req)
	if db.Error != nil {
		return nil, db.Error
	}
	return req, nil
}

func DeletePluginGroup(db *gorm.DB, group string) error {
	db = db.Model(&PluginGroup{})
	if group != "" {
		db = db.Where(" `group` = ?", group)
	}
	db = db.Unscoped().Delete(&PluginGroup{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}

func GroupCount(db *gorm.DB) (req []*TagAndTypeValue, err error) {
	db = db.Model(&PluginGroup{}).Select(" `group` as value, count(*) as count ")
	db = db.Joins("INNER JOIN yak_scripts Y on Y.script_name = plugin_groups.yak_script_name ")
	//db = db.Where("yak_script_name IN (SELECT DISTINCT(script_name) FROM yak_scripts)")
	db = db.Group(" `group` ").Order(`count desc`).Scan(&req)
	if db.Error != nil {
		return nil, utils.Errorf("type group rows failed: %s", db.Error)
	}

	return req, nil
}

func GetGroup(db *gorm.DB, scriptNames []string) (req []*PluginGroup, err error) {
	db = db.Model(&PluginGroup{}).Select(" `group`")
	if len(scriptNames) > 0 {
		db = db.Joins("inner join yak_scripts Y on Y.script_name = plugin_groups.yak_script_name ")
		db = bizhelper.ExactQueryStringArrayOr(db, "plugin_groups.yak_script_name", scriptNames)
		db = db.Group(" `group` ").Having("COUNT(DISTINCT yak_script_name) = ?", len(scriptNames))
		db = db.Scan(&req)
	} else {
		db = db.Joins("inner join yak_scripts Y on Y.script_name = plugin_groups.yak_script_name ")
		db = db.Group(" `group` ").Scan(&req)
	}
	if db.Error != nil {
		return nil, utils.Errorf("GetGroup failed: %s", db.Error)
	}

	return req, nil
}

func DeletePluginGroupByScriptName(db *gorm.DB, scriptName []string) error {
	db = db.Model(&PluginGroup{})
	db = bizhelper.ExactQueryStringArrayOr(db, "yak_script_name", scriptName).Unscoped().Delete(&PluginGroup{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}
