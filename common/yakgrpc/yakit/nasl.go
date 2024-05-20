package yakit

import (
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"sync"
)

var createNaslScript = new(sync.Mutex)

func NewEmptyNaslScript() *schema.NaslScript {
	return &schema.NaslScript{}
}
func NewNaslScript(name, content string) *schema.NaslScript {
	obj := NewEmptyNaslScript()
	obj.ScriptName = name
	obj.Script = content
	obj.Hash = obj.CalcHash()
	return obj
}
func FilterRootScriptsWithDbModelType(scripts []*schema.NaslScript) []*schema.NaslScript {
	newScripts := []*schema.NaslScript{}
	tmp := map[string]struct{}{}
	for _, script := range scripts {
		var dep []string
		err := json.Unmarshal([]byte(script.Dependencies), &dep)
		if err != nil {
			continue
		}
		for _, d := range dep {
			tmp[d] = struct{}{}
		}
	}
	for _, script := range scripts {
		if _, ok := tmp[script.OriginFileName]; !ok {
			newScripts = append(newScripts, script)
		}
	}
	return newScripts
}
func QueryRootNaslScriptByYakScriptRequest(db *gorm.DB, params *ypb.QueryYakScriptRequest) (*bizhelper.Paginator, []*schema.NaslScript, error) {
	p, scripts, err := QueryNaslScriptByYakScriptRequest(db, params)
	if err != nil {
		return nil, nil, err
	}
	return p, FilterRootScriptsWithDbModelType(scripts), nil
}
func QueryNaslScriptByYakScriptRequest(db *gorm.DB, params *ypb.QueryYakScriptRequest) (*bizhelper.Paginator, []*schema.NaslScript, error) {
	if params == nil {
		params = &ypb.QueryYakScriptRequest{}
	}

	/*pagination*/
	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}

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

	p := params.Pagination
	db = db.Model(&schema.NaslScript{}).Order(orderOrdinary)
	db = FilterNaslScript(db, params)

	var ret []*schema.NaslScript
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return paging, ret, nil
}

// FilterNaslScript 过滤nasl脚本，支持关键词搜索，family过滤，排除和指定脚本名
func FilterNaslScript(db *gorm.DB, params *ypb.QueryYakScriptRequest) *gorm.DB {
	if params.GetKeyword() != "" {
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
			"origin_file_name", "o_id", "cve", "script_name", "script", "family", "category", "tags",
		}, strings.Split(params.GetKeyword(), ","), false)
	}

	familys := utils.StringArrayFilterEmpty(params.GetFamily())
	if len(familys) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "family", familys)
	}

	// 排除特定脚本
	db = bizhelper.ExactQueryExcludeStringArrayOr(db, "script_name", params.GetExcludeScriptNames())
	if len(params.GetIncludedScriptNames()) > 0 {
		if len(utils.StringArrayFilterEmpty(params.GetExcludeScriptNames())) > 0 {
			//db = db.Or("script_name IN(?)", params.GetIncludedScriptNames())
			db = bizhelper.ExactOrQueryStringArrayOr(db, "script_name", params.GetIncludedScriptNames())
		} else {
			//db = db.Where("script_name IN(?)", params.GetIncludedScriptNames())
			db = bizhelper.ExactQueryStringArrayOr(db, "script_name", params.GetIncludedScriptNames())
		}
	}

	return db
}

func QueryNaslScriptByOID(db *gorm.DB, oid string) (*schema.NaslScript, error) {
	req := &schema.NaslScript{}
	if db := db.Model(&schema.NaslScript{}).Where("o_id = ?", oid).First(req); db.Error != nil {
		return nil, utils.Errorf("get NaslScript failed: %s", db.Error)
	}
	return req, nil
}
func QueryNaslScriptByName(db *gorm.DB, name string) (*schema.NaslScript, error) {
	req := &schema.NaslScript{}
	if db := db.Model(&schema.NaslScript{}).Where("script_name = ?", name).First(req); db.Error != nil {
		return nil, utils.Errorf("get NaslScript failed: %s", db.Error)
	}
	return req, nil
}
