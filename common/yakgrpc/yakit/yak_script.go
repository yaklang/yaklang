package yakit

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/gorm"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
	"strings"
	"sync"
)

var yakScriptOpLock = new(sync.Mutex)

type YakScript struct {
	gorm.Model

	ScriptName string `json:"script_name" gorm:"unique_index"`
	Type       string `json:"type" gorm:"index"`
	Content    string `json:"content"`
	Level      string `json:"level"`
	Params     string `json:"params"`
	Help       string `json:"help"`
	Author     string `json:"author"`
	Tags       string `json:"tags,omitempty"`
	Ignored    bool   `json:"ignore"`

	// 加载本地的数据
	FromLocal bool   `json:"from_local"`
	LocalPath string `json:"local_path"`

	// History string
	IsHistory bool `json:"is_history"`

	// Force Interactive
	// Means that this script will be executed in interactive mode
	// cannot load as a plugin or a module by mix caller
	ForceInteractive bool `json:"force_interactive"`

	FromStore bool `json:"from_store"`

	IsGeneralModule      bool   `json:"is_general_module"`
	GeneralModuleVerbose string `json:"general_module_verbose"`
	GeneralModuleKey     string `json:"general_module_key"`
	FromGit              string `json:"from_git"`

	// 这个是自动填写的，一般不需要自己来填写
	// 条件是 Params 中有一个名字为 target 的必填参数
	IsBatchScript bool `json:"is_batch_script"`
	IsExternal    bool `json:"is_external"`

	EnablePluginSelector bool   `json:"enable_plugin_selector"`
	PluginSelectorTypes  string `json:"plugin_selector_types"`

	// Online ID: 线上插件的 ID
	OnlineId           int64  `json:"online_id"`
	OnlineScriptName   string `json:"online_script_name"`
	OnlineContributors string `json:"online_contributors"`
	OnlineIsPrivate    bool   `json:"online_is_private"`

	// 这个插件所属用户 ID
	UserId int64 `json:"user_id"`
	// 这个插件的 UUID
	Uuid           string      `json:"uuid"`
	HeadImg        string      `json:"head_img"`
	OnlineBaseUrl  string      `json:"online_base_url"`
	BaseOnlineId   int64       `json:"BaseOnlineId"`
	OnlineOfficial bool        `json:"online_official"`
	OnlineGroup    string      `json:"online_group"`
	sourceScript   interface{} // 用于存储原始的 script(可能是由原类型是NaslScript)

	IsCorePlugin bool `json:"is_core_plugin"` // 判断是否是核心插件

}

func (s *YakScript) BeforeSave() error {
	if s.ScriptName == "" {
		return utils.Errorf("empty script name is denied")
	}

	if utils.MatchAnyOfSubString(s.ScriptName, "|") {
		return utils.Errorf("invalid scriptName, do not contains '|'")
	}

	resRaw, _ := strconv.Unquote(s.Params)
	if resRaw != "" {
		var res interface{}
		_ = json.Unmarshal([]byte(resRaw), &res)
		switch ret := res.(type) {
		case []interface{}:
			for _, rawIf := range ret {
				i, ok := rawIf.(map[string]interface{})
				if !ok {
					continue
				}
				if utils.MapGetString(i, "Field") == "target" && utils.MapGetBool(i, "Required") {
					s.IsBatchScript = true
					break
				}
			}
		}
	}
	return nil
}

func (s *YakScript) ToGRPCModel() *ypb.YakScript {
	var params []*ypb.YakScriptParam
	if s.Params != "" && s.Params != `""` {
		r, _ := strconv.Unquote(s.Params)
		err := json.Unmarshal([]byte(r), &params)
		if err != nil {
			log.Errorf("unmarshal params failed: %s", err)
			spew.Dump([]byte(r))
		}
	}

	script := &ypb.YakScript{
		Id:                   int64(s.ID),
		Content:              s.Content,
		Type:                 s.Type,
		Params:               params,
		CreatedAt:            s.CreatedAt.Unix(),
		ScriptName:           s.ScriptName,
		Help:                 s.Help,
		Level:                s.Level,
		Author:               s.Author,
		Tags:                 s.Tags,
		IsHistory:            s.IsHistory,
		IsIgnore:             s.Ignored,
		IsGeneralModule:      s.IsGeneralModule,
		GeneralModuleVerbose: s.GeneralModuleVerbose,
		GeneralModuleKey:     s.GeneralModuleKey,
		FromGit:              s.FromGit,
		EnablePluginSelector: s.EnablePluginSelector,
		PluginSelectorTypes:  s.PluginSelectorTypes,
		OnlineId:             s.OnlineId,
		OnlineScriptName:     s.OnlineScriptName,
		OnlineContributors:   s.OnlineContributors,
		OnlineIsPrivate:      s.OnlineIsPrivate,
		UserId:               s.UserId,
		UUID:                 s.Uuid,
		HeadImg:              s.HeadImg,
		OnlineBaseUrl:        s.OnlineBaseUrl,
		BaseOnlineId:         s.BaseOnlineId,
		OnlineOfficial:       s.OnlineOfficial,
		OnlineGroup:          s.OnlineGroup,
	}
	if s.Type == "mitm" {
		script.Params = mitmPluginDefaultPlugins
	}
	return script
}

func CreateOrUpdateYakScript(db *gorm.DB, id int64, i interface{}) error {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()

	db = UserDataAndPluginDatabaseScope(db)
	db = db.Model(&YakScript{})

	if db := db.Where("id = ?", id).Assign(i).FirstOrCreate(&YakScript{}); db.Error != nil {
		return utils.Errorf("create/update YakScript failed: %s", db.Error)
	}

	return nil
}

var downloadOnlineId = new(sync.Mutex)

func DeleteYakScriptByOnlineId(db *gorm.DB, onlineId int64) error {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()

	db = UserDataAndPluginDatabaseScope(db)
	if db := db.Model(&YakScript{}).Where(
		"online_id = ?", onlineId,
	).Unscoped().Delete(&YakScript{}); db.Error != nil {
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
	db = UserDataAndPluginDatabaseScope(db)

	db = db.Model(&YakScript{})

	_ = DeleteYakScriptByOnlineId(db, onlineId)
	if db := db.Where("online_id = ?", onlineId).Assign(i).FirstOrCreate(&YakScript{}); db.Error != nil {
		return utils.Errorf("create/update YakScript failed: %s", db.Error)
	}

	switch ret := i.(type) {
	case *YakScript:
		return UpdateGeneralModuleFromByYakScriptName(db, ret.ScriptName, ret.IsGeneralModule)
	case YakScript:
		return UpdateGeneralModuleFromByYakScriptName(db, ret.ScriptName, ret.IsGeneralModule)
	}

	return nil
}

func CreateOrUpdateYakScriptByName(db *gorm.DB, scriptName string, i interface{}) error {
	db = UserDataAndPluginDatabaseScope(db)

	db = db.Model(&YakScript{})

	// 锁住更新步骤，太快容易整体被锁
	yakScriptOpLock.Lock()
	if db := db.Where("script_name = ?", scriptName).Assign(i).FirstOrCreate(&YakScript{}); db.Error != nil {
		yakScriptOpLock.Unlock()
		return utils.Errorf("create/update YakScript failed: %s", db.Error)
	}
	yakScriptOpLock.Unlock()

	switch ret := i.(type) {
	case *YakScript:
		return UpdateGeneralModuleFromByYakScriptName(db, scriptName, ret.IsGeneralModule)
	case YakScript:
		return UpdateGeneralModuleFromByYakScriptName(db, scriptName, ret.IsGeneralModule)
	}

	return nil
}

func CreateTemporaryYakScript(t string, code string) (string, error) {
	var name = fmt.Sprintf("tmp-%v", ksuid.New().String())
	var err = CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), name, &YakScript{
		ScriptName: name,
		Type:       t,
		Content:    code,
		Author:     "temp",
		Ignored:    true,
	})
	if err != nil {
		return "", err
	}
	return name, nil
}

func UpdateGeneralModuleFromByYakScriptName(db *gorm.DB, scriptName string, i bool) error {
	db = UserDataAndPluginDatabaseScope(db)

	return CreateOrUpdateYakScriptByName(db, scriptName, map[string]interface{}{
		"is_general_module": i,
	})
}

func GetYakScript(db *gorm.DB, id int64) (*YakScript, error) {
	var req YakScript
	db = UserDataAndPluginDatabaseScope(db)

	if db := db.Model(&YakScript{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get YakScript failed: %s", db.Error)
	}

	return &req, nil
}

func GetYakScriptIdOrName(db *gorm.DB, id int64, name string) (*YakScript, error) {
	var req YakScript
	db = UserDataAndPluginDatabaseScope(db)

	if db := db.Model(&YakScript{}).Where("(id = ?) OR (script_name = ?)", id, name).First(&req); db.Error != nil {
		return nil, utils.Errorf("get YakScript failed: %s", db.Error)
	}

	return &req, nil
}

func GetYakScriptByName(db *gorm.DB, name string) (*YakScript, error) {
	var req YakScript
	db = UserDataAndPluginDatabaseScope(db)

	if db := db.Model(&YakScript{}).Where("script_name = ?", name).First(&req); db.Error != nil {
		return nil, utils.Errorf("get YakScript failed: %s", db.Error)
	}

	return &req, nil
}

// GetNucleiYakScriptByName
func GetNucleiYakScriptByName(db *gorm.DB, scriptName string) (*YakScript, error) {
	var req YakScript
	db = UserDataAndPluginDatabaseScope(db)
	if db := db.Model(&YakScript{}).Where(
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

func GetYakScriptByOnlineID(db *gorm.DB, onlineId int64) (*YakScript, error) {
	var req YakScript
	db = UserDataAndPluginDatabaseScope(db)

	if db := db.Model(&YakScript{}).Where("online_id = ?", onlineId).First(&req); db.Error != nil {
		return nil, utils.Errorf("get YakScript failed: %s", db.Error)
	}

	return &req, nil
}

func GetYakScriptByUUID(db *gorm.DB, uuid string) (*YakScript, error) {
	var req YakScript
	db = UserDataAndPluginDatabaseScope(db)

	if db := db.Model(&YakScript{}).Where("uuid = ?", uuid).First(&req); db.Error != nil {
		return nil, utils.Errorf("get YakScript failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteYakScriptByID(db *gorm.DB, id int64) error {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()
	db = UserDataAndPluginDatabaseScope(db)

	if db := db.Model(&YakScript{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&YakScript{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteYakScriptByName(db *gorm.DB, s string) error {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()
	db = UserDataAndPluginDatabaseScope(db)

	if db := db.Model(&YakScript{}).Where(
		"script_name = ?", s,
	).Unscoped().Delete(&YakScript{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteYakScriptByUserID(db *gorm.DB, s int64, onlineBaseUrl string) error {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()
	db = UserDataAndPluginDatabaseScope(db)

	if s <= 0 {
		return nil
	}
	db = db.Model(&YakScript{}).Where(
		"user_id = ? and online_is_private = true", s,
	)
	if onlineBaseUrl != "" {
		db = db.Where("online_base_url = ?", onlineBaseUrl)
	}
	db = db.Unscoped().Delete(&YakScript{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteYakScriptAll(db *gorm.DB) error {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()
	db = UserDataAndPluginDatabaseScope(db)

	if db := db.Model(&YakScript{}).Where(
		"true",
	).Unscoped().Delete(&YakScript{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteYakScriptByWhere(db *gorm.DB, params *ypb.DeleteLocalPluginsByWhereRequest) error {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()
	db = UserDataAndPluginDatabaseScope(db)

	db = db.Model(&YakScript{}).Unscoped()
	if params.GetType() == "" && params.GetKeywords() == "" {
		db = db.Where(
			"true",
		)
	} else {
		if params.GetType() != "" {
			db = bizhelper.ExactQueryStringArrayOr(db, "type", utils.PrettifyListFromStringSplited(params.GetType(), ","))
		}

		if params.GetKeywords() != "" {
			db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
				"script_name", "content", "help", "author", "tags",
			}, strings.Split(params.GetKeywords(), ","), false)
		}

		if params.GetUserId() > 0 {
			db = db.Where("user_id = ?", params.GetUserId())
		}

		if params.GetUserName() != "" {
			db = db.Where("author like ?", "%"+params.GetUserName()+"%")
		}
	}
	if db = db.Delete(&YakScript{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func IgnoreYakScriptByID(db *gorm.DB, id int64, ignored bool) error {
	r, err := GetYakScript(db, id)
	if err != nil {
		return err
	}
	db = UserDataAndPluginDatabaseScope(db)

	_ = r
	return CreateOrUpdateYakScript(db, id, map[string]interface{}{
		"ignored": ignored,
	})
}

func QueryYakScriptByNames(db *gorm.DB, names ...string) []*YakScript {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()
	db = UserDataAndPluginDatabaseScope(db)

	db = db.Model(&YakScript{})
	var all []*YakScript
	for _, i := range utils.SliceGroup(names, 100) {
		var tmp []*YakScript
		nDB := bizhelper.ExactQueryStringArrayOr(db, "script_name", i)
		if err := nDB.Find(&tmp).Error; err != nil {
			log.Errorf("dberror(query yak scripts): %v", err)
		}
		all = append(all, tmp...)
	}
	return all
}

func QueryYakScriptByIsCore(db *gorm.DB, isCore bool) []*YakScript {
	yakScriptOpLock.Lock()
	defer yakScriptOpLock.Unlock()
	db = UserDataAndPluginDatabaseScope(db)

	db = db.Model(&YakScript{})
	var yakScripts []*YakScript
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

	// 判断是否是历史脚本
	if !params.GetIsHistory() {
		db = db.Where("is_history = ?", false)
	}

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
			//db = db.Or("script_name IN(?)", params.GetIncludedScriptNames())
			db = bizhelper.ExactOrQueryStringArrayOr(db, "script_name", params.GetIncludedScriptNames())
		} else {
			//db = db.Where("script_name IN(?)", params.GetIncludedScriptNames())
			db = bizhelper.ExactQueryStringArrayOr(db, "script_name", params.GetIncludedScriptNames())
		}
	}

	return db
}

func QueryYakScript(db *gorm.DB, params *ypb.QueryYakScriptRequest) (*bizhelper.Paginator, []*YakScript, error) {
	if params == nil {
		params = &ypb.QueryYakScriptRequest{}
	}
	if params.Type == "nasl" {
		p, scripts, err := QueryRootNaslScriptByYakScriptRequest(db, params)
		var yakScripts []*YakScript
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
	//db = UserDataAndPluginDatabaseScope(db)
	db = db.Model(&YakScript{}) // .Debug()

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

	var orderOrdinary = "updated_at desc"
	if utils.StringArrayContains([]string{
		"created_at", "updated_at", "id", "script_name",
		"author",
	}, strings.ToLower(params.GetPagination().GetOrderBy())) {
		orderOrdinary = fmt.Sprintf("%v %v", params.GetPagination().GetOrderBy(), params.GetPagination().GetOrder())
		orderOrdinary = strings.TrimSpace(orderOrdinary)
	}

	if !params.GetIgnoreGeneralModuleOrder() {
		if orderOrdinary != "" {
			db = db.Order(`is_general_module desc, ` + orderOrdinary)
		} else {
			db = db.Order(`is_general_module desc, updated_at desc`)
		}
	} else {
		if orderOrdinary != "" {
			db = db.Order(orderOrdinary)
		} else {
			db = db.Order("updated_at desc")
		}
	}

	db = FilterYakScript(db, params) // .LogMode(true).Debug()
	var ret []*YakScript
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
func YieldYakScripts(db *gorm.DB, ctx context.Context) chan *YakScript {

	outC := make(chan *YakScript)

	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*YakScript
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

func GetYakScriptList(db *gorm.DB, id int64, ids []int64) ([]*YakScript, error) {
	var req []*YakScript
	db = UserDataAndPluginDatabaseScope(db)

	db = db.Model(&YakScript{})
	if id > 0 {
		db = db.Where("id = ?", id)
	}
	if len(ids) > 0 {
		db = db.Where("id in (?)", ids)
	}
	db = db.Scan(&req)
	if db.Error != nil {
		return nil, utils.Errorf("get YakScript failed: %s", db.Error)
	}
	return req, nil
}
