package yakit

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var pocBuiltInGroups = map[string]string{
	"ThinkPHP":      "thinkphp",
	"Shiro":         "shiro",
	"FastJSON":      "fastjson",
	"Struts":        "struts",
	"Tomcat":        "tomcat",
	"Weblogic":      "weblogic",
	"Spring":        "spring,springboot,springcloud,springframework",
	"Jenkins":       "jenkins",
	"IIS":           "iis",
	"ElasticSearch": "elastic",
	"致远 OA":         "seeyou,seeyon,zhiyuan",
	"Exchange":      "exchange",
	"通达 OA":         "tongda",
	"PhpMyAdmin":    "phpmyadmin",
	"Nexus":         "nexus",
	"Laravel":       "laravel",
	"JBoss":         "jboss",
	"ColdFusion":    "coldfusion",
	"ActiveMQ":      "activemq",
	"Wordpress":     "wordpress",
	"Java":          "java",
	"PHP":           "php",
	"Python":        "python",
	"Nginx":         "nginx",

	"网络设备与OA系统":  "锐捷,若依,金和,金山,金蝶,致远,Seeyou,seeyou,通达,tonged,Tongda,银澎,浪潮,泛微,方维,帆软,向日葵,ecshop,dahua,huawei,zimbra,coremail,Coremail,邮件服务器,",
	"安全产品":       "防火墙,行为管理,绿盟,天擎,tianqing,防篡改,网御星云,安防,审计系统,天融信,安全系统",
	"Log4j":      "Log4j,log4j,Log4shell,log4shell,Log4Shell",
	"远程代码执行（扫描）": "RCE,rce",
	"XSS":        "xss,XSS",
	"SQL注入":      "sql注入",
}

func init() {
	RegisterPostInitDatabaseFunction(func() error {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("DeletePluginGroupsWithNonEmptyTemporaryId failed: %s", err)
			}
		}()
		if db := consts.GetGormProfileDatabase(); db != nil {
			err := DeletePluginGroupsWithNonEmptyTemporaryId(db)
			if err != nil {
				return err
			}
			var count int
			allGroups, err := GroupCount(db)
			for _, g := range allGroups {
				if g.IsPocBuiltIn {
					count++
				}
			}
			if count >= len(pocBuiltInGroups)-1 {
				return nil
			}

			db = db.Model(&schema.YakScript{})
			for group, keywords := range pocBuiltInGroups {
				filterDb := FilterYakScript(db, &ypb.QueryYakScriptRequest{
					Keyword: keywords,
				})
				yakScripts := bizhelper.YieldModel[*schema.YakScript](context.Background(), filterDb)
				for yakScript := range yakScripts {
					res, err := GetYakScriptByName(consts.GetGormProfileDatabase(), yakScript.ScriptName)
					if err != nil {
						log.Errorf("GetYakScriptByName failed: %s", err)
						continue

					}
					if res == nil {
						continue
					}

					saveData := &schema.PluginGroup{
						YakScriptName: yakScript.ScriptName,
						Group:         group,
						IsPocBuiltIn:  true,
					}
					saveData.Hash = saveData.CalcHash()
					log.Debugf("Save YakScriptGroup [%v] [%v]", yakScript.ScriptName, group)
					err = CreateOrUpdatePluginGroup(consts.GetGormProfileDatabase(), saveData.Hash, saveData)
					if err != nil {
						log.Errorf("[%v] Save YakScriptGroup [%v] err %s", yakScript.ScriptName, group, err.Error())
					}
				}
			}

		}
		return nil
	})
}

func CreateOrUpdatePluginGroup(db *gorm.DB, hash string, i interface{}) error {
	yakScriptOpLock.Lock()
	db = db.Model(&schema.PluginGroup{})
	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.PluginGroup{}); db.Error != nil {
		return utils.Errorf("create/update PluginGroup failed: %s", db.Error)
	}
	yakScriptOpLock.Unlock()
	return nil
}

func DeletePluginGroupByHash(db *gorm.DB, hash string) error {
	db = db.Model(&schema.PluginGroup{}).Where("hash = ?", hash).Unscoped().Delete(&schema.PluginGroup{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}

func DeletePluginGroupsWithNonEmptyTemporaryId(db *gorm.DB) error {
	db = db.Model(&schema.PluginGroup{}).Where("temporary_id != ''").Unscoped().Delete(&schema.PluginGroup{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}

func GetPluginByGroup(db *gorm.DB, group string) (req []*schema.PluginGroup, err error) {
	db = db.Model(&schema.PluginGroup{}).Where("`group` = ?", group).Scan(&req)
	if db.Error != nil {
		return nil, db.Error
	}
	return req, nil
}

func DeletePluginGroup(db *gorm.DB, group string) error {
	db = db.Model(&schema.PluginGroup{})
	if group != "" {
		db = db.Where(" `group` = ?", group)
	}
	db = db.Unscoped().Delete(&schema.PluginGroup{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}

func GroupCount(db *gorm.DB) (req []*TagAndTypeValue, err error) {
	db = db.Model(&schema.PluginGroup{}).Select(" `group` as value, count(*) as count, `temporary_id` as temporary_id, `is_poc_built_in` as is_poc_built_in")
	db = db.Joins("INNER JOIN yak_scripts Y on Y.script_name = plugin_groups.yak_script_name ")
	//db = db.Where("yak_script_name IN (SELECT DISTINCT(script_name) FROM yak_scripts)")
	db = db.Group(" `group`,`temporary_id`,`is_poc_built_in` ").Order(`count desc`).Scan(&req)
	if db.Error != nil {
		return nil, utils.Wrap(db.Error, "GroupCount failed")
	}

	return req, nil
}

func GetGroup(db *gorm.DB, scriptNames []string) (req []*schema.PluginGroup, err error) {
	db = db.Model(&schema.PluginGroup{}).Select(" DISTINCT(`group`)")
	if len(scriptNames) > 0 {
		db = db.Joins("inner join yak_scripts Y on Y.script_name = plugin_groups.yak_script_name ")
		db = bizhelper.ExactQueryStringArrayOr(db, "plugin_groups.yak_script_name", scriptNames)
		db = db.Where("is_poc_built_in = false")
		db = db.Group(" `group` ").Having("COUNT(DISTINCT yak_script_name) = ?", len(scriptNames))
		db = db.Scan(&req)
	}
	if db.Error != nil {
		return nil, utils.Errorf("GetGroup failed: %s", db.Error)
	}

	return req, nil
}

func DeletePluginGroupByScriptName(db *gorm.DB, scriptName []string) error {
	db = db.Model(&schema.PluginGroup{})
	db = bizhelper.ExactQueryStringArrayOr(db, "yak_script_name", scriptName).Unscoped().Delete(&schema.PluginGroup{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryGroupCount(db *gorm.DB, excludeType []string, isMITMParamPlugins int64) (req []*TagAndTypeValue, err error) {
	db = db.Model(&schema.PluginGroup{}).Select(" `group` as value, COUNT(Y.script_name) as count, `temporary_id` as temporary_id, `is_poc_built_in` as is_poc_built_in")
	db = db.Joins("LEFT JOIN yak_scripts Y on Y.script_name = plugin_groups.yak_script_name ")
	db = bizhelper.ExactQueryExcludeStringArrayOr(db, "Y.type", excludeType)
	switch isMITMParamPlugins {
	case 1:
		db = db.Where("Y.params!='\"null\"' and Y.params is not null and LENGTH(Y.params)>0")
	case 2:
		db = db.Where("(Y.params='\"null\"' or Y.params is null or LENGTH(Y.params)<=0) or Y.type!='mitm'")
	}
	db = db.Group(" `group`,`temporary_id`,`is_poc_built_in` ").Order(`count desc`).Scan(&req)
	if db.Error != nil {
		return nil, utils.Wrap(db.Error, "GroupCount failed")
	}

	return req, nil
}
