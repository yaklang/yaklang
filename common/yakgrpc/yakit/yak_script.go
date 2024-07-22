package yakit

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/schema"

	"gopkg.in/yaml.v2"

	"github.com/jinzhu/gorm"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var yakScriptOpLock = new(sync.Mutex)

func CreateOrUpdateYakScript(db *gorm.DB, id int64, i interface{}) error {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()

	db = db.Model(&schema.YakScript{})

	if db := db.Where("id = ?", id).Assign(i).FirstOrCreate(&schema.YakScript{}); db.Error != nil {
		return utils.Errorf("create/update YakScript failed: %s", db.Error)
	}

	return nil
}

var downloadOnlineId = new(sync.Mutex)

func DeleteYakScriptByOnlineId(db *gorm.DB, onlineId int64) error {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()

	if db := db.Model(&schema.YakScript{}).Where(
		"online_id = ?", onlineId,
	).Unscoped().Delete(&schema.YakScript{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func CreateOrUpdateYakScriptByOnlineId(db *gorm.DB, onlineId int64, i interface{}) error {
	if onlineId <= 0 {
		return nil
	}

	downloadOnlineId.Lock()
	defer downloadOnlineId.Unlock()

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("error: %s", err)
		}
	}()

	db = db.Model(&schema.YakScript{})

	_ = DeleteYakScriptByOnlineId(db, onlineId)
	if db := db.Where("online_id = ?", onlineId).Assign(i).FirstOrCreate(&schema.YakScript{}); db.Error != nil {
		return utils.Errorf("create/update YakScript failed: %s", db.Error)
	}

	switch ret := i.(type) {
	case *schema.YakScript:
		return UpdateGeneralModuleFromByYakScriptName(db, ret.ScriptName, ret.IsGeneralModule)
	case schema.YakScript:
		return UpdateGeneralModuleFromByYakScriptName(db, ret.ScriptName, ret.IsGeneralModule)
	}

	return nil
}

func CreateOrUpdateYakScriptByName(db *gorm.DB, scriptName string, i interface{}) error {
	db = db.Model(&schema.YakScript{})

	// 锁住更新步骤，太快容易整体被锁
	yakScriptOpLock.Lock()
	if db := db.Where("script_name = ?", scriptName).Assign(i).FirstOrCreate(&schema.YakScript{}); db.Error != nil {
		yakScriptOpLock.Unlock()
		return utils.Errorf("create/update YakScript failed: %s", db.Error)
	}
	yakScriptOpLock.Unlock()

	switch ret := i.(type) {
	case *schema.YakScript:
		return UpdateGeneralModuleFromByYakScriptName(db, scriptName, ret.IsGeneralModule)
	case schema.YakScript:
		return UpdateGeneralModuleFromByYakScriptName(db, scriptName, ret.IsGeneralModule)
	}

	return nil
}

func CreateTemporaryYakScript(t string, code string, suffix ...string) (string, error) {
	script, err := NewTemporaryYakScript(t, code, suffix...)
	if err != nil {
		return "", err
	}
	name := script.ScriptName
	err = CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), name, script)
	if err != nil {
		return "", err
	}
	return name, nil
}

func CreateTemporaryYakScriptEx(t string, code string, suffix ...string) (name string, clear func(), err error) {
	script, err := NewTemporaryYakScript(t, code, suffix...)
	if err != nil {
		return "", nil, err
	}
	name = script.ScriptName
	err = CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), name, script)
	if err != nil {
		return "", nil, err
	}
	return name, func() {
		DeleteYakScriptByName(consts.GetGormProfileDatabase(), name)
	}, nil
}

func NewTemporaryYakScript(t string, code string, suffix ...string) (*schema.YakScript, error) {
	name := fmt.Sprintf("tmp-%v", ksuid.New().String()+strings.Join(suffix, ""))
	if strings.TrimSpace(strings.ToLower(t)) == "nuclei" {
		// nuclei
		tempInfo := make(map[string]any)
		err := yaml.Unmarshal([]byte(code), &tempInfo)
		if err != nil {
			return nil, utils.Errorf("plugin code: %s is not yaml: %v", string(code), err)
		}
		nameInfo := utils.MapGetString(tempInfo, "id")
		name = "[TMP]-" + nameInfo + "-" + ksuid.New().String() + strings.Join(suffix, "-")
	}
	return &schema.YakScript{
		ScriptName: name,
		Type:       t,
		Content:    code,
		Author:     "temp",
		Ignored:    true,
	}, nil
}

func RemoveTemporaryYakScriptAll(db *gorm.DB, suffix string) {
	db = db.Model(&schema.YakScript{}).Where("script_name LIKE ?", "[TMP]%"+suffix).Where("ignored = true")
	if db := db.Unscoped().Delete(&schema.YakScript{}); db.Error != nil {
		log.Errorf("remove temporary yak script failed: %s", db.Error)
	}
}

func UpdateGeneralModuleFromByYakScriptName(db *gorm.DB, scriptName string, i bool) error {
	return CreateOrUpdateYakScriptByName(db, scriptName, map[string]interface{}{
		"is_general_module": i,
	})
}

func GetYakScript(db *gorm.DB, id int64) (*schema.YakScript, error) {
	var req schema.YakScript

	if db := db.Model(&schema.YakScript{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get YakScript failed: %s", db.Error)
	}

	return &req, nil
}

func GetYakScriptIdOrName(db *gorm.DB, id int64, name string) (*schema.YakScript, error) {
	var req schema.YakScript

	if db := db.Model(&schema.YakScript{}).Where("(id = ?) OR (script_name = ?)", id, name).First(&req); db.Error != nil {
		return nil, utils.Errorf("get YakScript failed: %s", db.Error)
	}

	return &req, nil
}

func GetYakScriptByName(db *gorm.DB, name string) (*schema.YakScript, error) {
	var req schema.YakScript

	if db := db.Model(&schema.YakScript{}).Where("script_name = ?", name).First(&req); db.Error != nil {
		return nil, utils.Errorf("get YakScript failed: %s", db.Error)
	}

	return &req, nil
}

// GetNucleiYakScriptByName
func GetNucleiYakScriptByName(db *gorm.DB, scriptName string) (*schema.YakScript, error) {
	var req schema.YakScript

	if db := db.Model(&schema.YakScript{}).Where(
		"`type` = 'nuclei'",
	).Where(
		"(script_name LIKE ?) OR (script_name LIKE ?) OR (script_name = ?)",
		"[%]:%"+scriptName,
		`[`+scriptName+`]:%`,
		scriptName,
	).First(&req); db.Error != nil {
		return nil, utils.Errorf("get YakScript failed: %s", db.Error)
	}

	return &req, nil
}

func GetYakScriptByOnlineID(db *gorm.DB, onlineId int64) (*schema.YakScript, error) {
	var req schema.YakScript

	if db := db.Model(&schema.YakScript{}).Where("online_id = ?", onlineId).First(&req); db.Error != nil {
		return nil, utils.Errorf("get YakScript failed: %s", db.Error)
	}

	return &req, nil
}

func GetYakScriptByUUID(db *gorm.DB, uuid string) (*schema.YakScript, error) {
	var req schema.YakScript

	if db := db.Model(&schema.YakScript{}).Where("uuid = ?", uuid).First(&req); db.Error != nil {
		return nil, utils.Errorf("get YakScript failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteYakScriptByID(db *gorm.DB, id int64) error {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()

	if db := db.Model(&schema.YakScript{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&schema.YakScript{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteYakScriptByName(db *gorm.DB, s string) error {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()

	if db := db.Model(&schema.YakScript{}).Where(
		"script_name = ?", s,
	).Unscoped().Delete(&schema.YakScript{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteYakScriptByUserID(db *gorm.DB, s int64, onlineBaseUrl string) error {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()

	if s <= 0 {
		return nil
	}
	db = db.Model(&schema.YakScript{}).Where(
		"user_id = ? and online_is_private = true", s,
	)
	if onlineBaseUrl != "" {
		db = db.Where("online_base_url = ?", onlineBaseUrl)
	}
	db = db.Unscoped().Delete(&schema.YakScript{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteYakScriptAll(db *gorm.DB) error {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()

	if db := db.Model(&schema.YakScript{}).Where(
		"true",
	).Unscoped().Delete(&schema.YakScript{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteYakScriptByWhere(db *gorm.DB) error {
	if db = db.Delete(&schema.YakScript{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func IgnoreYakScriptByID(db *gorm.DB, id int64, ignored bool) error {
	r, err := GetYakScript(db, id)
	if err != nil {
		return err
	}

	_ = r
	return CreateOrUpdateYakScript(db, id, map[string]interface{}{
		"ignored": ignored,
	})
}

func QueryYakScriptByNames(db *gorm.DB, names ...string) []*schema.YakScript {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()

	db = db.Model(&schema.YakScript{})
	var all []*schema.YakScript
	for _, i := range utils.SliceGroup(names, 100) {
		var tmp []*schema.YakScript
		nDB := bizhelper.ExactQueryStringArrayOr(db, "script_name", i)
		if err := nDB.Find(&tmp).Error; err != nil {
			log.Errorf("dberror(query yak scripts): %v", err)
		}
		all = append(all, tmp...)
	}
	return all
}

func QueryYakScriptByIsCore(db *gorm.DB, isCore bool) []*schema.YakScript {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()

	db = db.Model(&schema.YakScript{})
	var yakScripts []*schema.YakScript
	if err := db.Where("is_core_plugin = ?", isCore).Find(&yakScripts).Error; err != nil {
		log.Errorf("dberror(query yak scripts): %v", err)
	}
	return yakScripts
}

func FilterYakScript(db *gorm.DB, params *ypb.QueryYakScriptRequest) *gorm.DB {
	db = db.Where("ignored = ?", params.GetIsIgnore())
	db = bizhelper.ExactQueryStringArrayOr(db, "type", utils.PrettifyListFromStringSplited(params.GetType(), ","))
	if params.GetKeyword() != "" {
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
			"script_name", "content", "help", "author", "tags",
		}, strings.Split(params.GetKeyword(), ","), false)
	}

	// 判断是否是历史脚本 暂时没用
	/*if !params.GetIsHistory() {
		db = db.Where("is_history = ?", false)
	}*/

	tags := utils.StringArrayFilterEmpty(params.GetTag())
	if len(tags) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "tags", tags)
	}

	// 判断是不是通用模块
	if params.GetIsGeneralModule() {
		db = bizhelper.QueryByBool(db, "is_general_module", true)
	}
	// 判断是否是批量脚本
	if params.GetIsBatch() {
		db = bizhelper.QueryByBool(db, "is_batch_script", true)
	}

	// 排除 workflow
	if params.GetExcludeNucleiWorkflow() {
		db = db.Where(
			"(local_path not like ?) AND (local_path not like ?)",
			"%"+"-workflow.yaml", "%"+"-workflow.yml",
		)
	}

	if params.GetUserId() > 0 {
		db = db.Where("user_id = ?", params.GetUserId())
	}

	if params.GetUserName() != "" {
		db = db.Where("author like ?", "%"+params.GetUserName()+"%")
	}

	// 排除特定脚本
	db = bizhelper.ExactQueryExcludeStringArrayOr(db, "script_name", params.GetExcludeScriptNames())
	if len(params.GetIncludedScriptNames()) > 0 {
		if len(utils.StringArrayFilterEmpty(params.GetExcludeScriptNames())) > 0 || len(tags) > 0 {
			db = bizhelper.ExactOrQueryStringArrayOr(db, "script_name", params.GetIncludedScriptNames())
		} else {
			db = bizhelper.ExactQueryStringArrayOr(db, "script_name", params.GetIncludedScriptNames())
		}
	}
	if params.GetUUID() != "" {
		db = db.Where("uuid = ?", params.GetUUID())
	}

	if params.Group != nil {
		if params.Group.UnSetGroup {
			db = db.Not("script_name IN (SELECT DISTINCT(yak_script_name) FROM plugin_groups)")
		} else {
			if len(params.Group.Group) > 0 {
				db = db.Where("yak_scripts.script_name in  (select yak_script_name from plugin_groups where `group` in (?) )", params.Group.Group)
				// db = db.Joins("left join plugin_groups P on yak_scripts.script_name = P.yak_script_name ")
				// db = bizhelper.ExactQueryStringArrayOr(db, "`group`", params.Group.Group)
			}
		}
	}
	db = bizhelper.ExactQueryExcludeStringArrayOr(db, "type", params.GetExcludeTypes())
	return db
}

func QueryYakScript(db *gorm.DB, params *ypb.QueryYakScriptRequest) (*bizhelper.Paginator, []*schema.YakScript, error) {
	if params == nil {
		params = &ypb.QueryYakScriptRequest{}
	}
	if params.Type == "nasl" {
		p, scripts, err := QueryRootNaslScriptByYakScriptRequest(db, params)
		var yakScripts []*schema.YakScript
		for _, i := range scripts {
			yakScript := i.ToYakScript()
			if yakScript == nil {
				log.Errorf("convert nasl script to yak script failed: %v", i)
				continue
			}
			yakScripts = append(yakScripts, yakScript)
		}
		return p, yakScripts, err
	}
	//
	db = db.Model(&schema.YakScript{}) // .Debug()

	/*pagination*/
	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	p := params.Pagination
	if !utils.StringArrayContains([]string{
		"desc", "asc", "",
	}, strings.ToLower(params.GetPagination().GetOrder())) {
		return nil, nil, utils.Error("invalid order")
	}

	orderOrdinary := "updated_at desc"
	if utils.StringArrayContains([]string{
		"created_at", "updated_at", "id", "script_name",
		"author",
	}, strings.ToLower(params.GetPagination().GetOrderBy())) {
		orderOrdinary = fmt.Sprintf("%v %v", params.GetPagination().GetOrderBy(), params.GetPagination().GetOrder())
		orderOrdinary = strings.TrimSpace(orderOrdinary)
	}

	if orderOrdinary != "" {
		db = db.Order(orderOrdinary)
	} else {
		db = db.Order("updated_at desc")
	}

	db = FilterYakScript(db, params) // .LogMode(true).Debug()
	var ret []*schema.YakScript
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

/*
YieldYakScripts no use spec, checking

	calling
*/
func YieldYakScripts(db *gorm.DB, ctx context.Context) chan *schema.YakScript {
	outC := make(chan *schema.YakScript)

	go func() {
		defer close(outC)

		page := 1
		for {
			var items []*schema.YakScript
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}

func GetYakScriptList(db *gorm.DB, id int64, ids []int64) ([]*schema.YakScript, error) {
	var req []*schema.YakScript

	db = db.Model(&schema.YakScript{})
	if id > 0 {
		db = db.Where("id = ?", id)
	}
	db = bizhelper.ExactQueryInt64ArrayOr(db, "id", ids)
	db = db.Scan(&req)
	if db.Error != nil {
		return nil, utils.Errorf("get YakScript failed: %s", db.Error)
	}
	return req, nil
}

func QueryExportYakScript(db *gorm.DB, params *ypb.ExportLocalYakScriptRequest) *gorm.DB {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()

	db = db.Model(&schema.YakScript{}).Unscoped()
	db = bizhelper.ExactQueryStringArrayOr(db, "type", utils.PrettifyListFromStringSplited(params.GetType(), ","))
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
		"script_name", "content", "help", "author", "tags",
	}, strings.Split(params.GetKeywords(), ","), false)
	db = bizhelper.FuzzQueryLike(db, "author", params.GetUserName())
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{"tags"}, strings.Split(params.GetTags(), ","), false)
	db = bizhelper.ExactQueryInt64ArrayOr(db, "id", params.YakScriptIds)
	return db
}

func CountYakScriptByWhere(db *gorm.DB, isGroup bool, req *ypb.QueryYakScriptGroupRequest) (total int64, err error) {
	db = db.Model(&schema.YakScript{})
	db = bizhelper.ExactQueryExcludeStringArrayOr(db, "type", req.ExcludeType)
	if isGroup {
		db = db.Not("script_name IN (SELECT DISTINCT(yak_script_name) FROM plugin_groups)")
	}
	db = db.Count(&total)
	if db.Error != nil {
		return 0, utils.Errorf("get YakScript failed: %s", db.Error)
	}
	return total, nil
}

func DeleteYakScript(db *gorm.DB, params *ypb.DeleteLocalPluginsByWhereRequest) *gorm.DB {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()

	db = db.Model(&schema.YakScript{}).Unscoped()
	db = bizhelper.ExactQueryStringArrayOr(db, "type", utils.PrettifyListFromStringSplited(params.GetType(), ","))
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
		"script_name", "content", "help", "author", "tags",
	}, strings.Split(params.GetKeywords(), ","), false)
	if params.GetUserId() > 0 {
		db = db.Where("user_id = ?", params.GetUserId())
	}
	db = bizhelper.FuzzQueryLike(db, "author", params.GetUserName())
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{"tags"}, strings.Split(params.GetTags(), ","), false)
	db = bizhelper.ExactQueryInt64ArrayOr(db, "id", params.Ids)
	if len(params.Groups) > 0 {
		db = db.Joins("left join plugin_groups P on yak_scripts.script_name = P.yak_script_name ")
		db = bizhelper.ExactQueryStringArrayOr(db, "`group`", params.Groups)
	}
	return db
}

func GetYakScriptByWhere(db *gorm.DB, name string, id int64) (*schema.YakScript, error) {
	var req schema.YakScript

	if db := db.Model(&schema.YakScript{}).Where("script_name = ? AND id <> ?", name, id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get YakScript failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteYakScriptByNameOrUUID(db *gorm.DB, name, uuid string) error {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()

	if db := db.Model(&schema.YakScript{}).Where(
		"script_name = ? or uuid = ?", name, uuid,
	).Unscoped().Delete(&schema.YakScript{}); db.Error != nil {
		return db.Error
	}
	return nil
}
