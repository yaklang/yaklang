package yakit

import (
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
	"strings"
	"sync"
)

type NaslScript struct {
	gorm.Model
	OriginFileName  string `json:"origin_file_name"`
	Group           string `json:"group"`
	Hash            string `json:"hash" gorm:"unique_index"`
	OID             string `json:"oid"`
	CVE             string `json:"cve"`
	ScriptName      string `json:"script_name"`
	Script          string `json:"script"`
	Tags            string `json:"tags,omitempty"`
	Version         string `json:"version"`
	Category        string `json:"category"`
	Family          string `json:"family"`
	Copyright       string `json:"copyright"`
	Dependencies    string `json:"dependencies,omitempty"`
	RequirePorts    string `json:"require_ports,omitempty"`
	RequireUdpPorts string `json:"require_udp_ports,omitempty"`
	ExcludeKeys     string `json:"exclude_keys,omitempty"`
	Xref            string `json:"xref,omitempty"`
	Preferences     string `json:"preferences,omitempty"`
	BugtraqId       string `json:"bugtraqId,omitempty"`
	MandatoryKeys   string `json:"mandatory_keys,omitempty"`
	Timeout         int    `json:"timeout,omitempty"`
	RequireKeys     string `json:"require_keys,omitempty"`
}

var createNaslScript = new(sync.Mutex)

func NewEmptyNaslScript() *NaslScript {
	return &NaslScript{}
}
func NewNaslScript(name, content string) *NaslScript {
	obj := NewEmptyNaslScript()
	obj.ScriptName = name
	obj.Script = content
	obj.Hash = obj.CalcHash()
	return obj
}
func FilterRootScriptsWithDbModelType(scripts []*NaslScript) []*NaslScript {
	newScripts := []*NaslScript{}
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
func QueryRootNaslScriptByYakScriptRequest(db *gorm.DB, params *ypb.QueryYakScriptRequest) (*bizhelper.Paginator, []*NaslScript, error) {
	p, scripts, err := QueryNaslScriptByYakScriptRequest(db, params)
	if err != nil {
		return nil, nil, err
	}
	return p, FilterRootScriptsWithDbModelType(scripts), nil
}
func QueryNaslScriptByYakScriptRequest(db *gorm.DB, params *ypb.QueryYakScriptRequest) (*bizhelper.Paginator, []*NaslScript, error) {
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
	db = db.Model(&NaslScript{}).Order(orderOrdinary)
	db = FilterNaslScript(db, params)

	var ret []*NaslScript
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

func QueryNaslScriptByOID(db *gorm.DB, oid string) (*NaslScript, error) {
	req := &NaslScript{}
	if db := db.Model(&NaslScript{}).Where("o_id = ?", oid).First(req); db.Error != nil {
		return nil, utils.Errorf("get NaslScript failed: %s", db.Error)
	}
	return req, nil
}
func QueryNaslScriptByName(db *gorm.DB, name string) (*NaslScript, error) {
	req := &NaslScript{}
	if db := db.Model(&NaslScript{}).Where("script_name = ?", name).First(req); db.Error != nil {
		return nil, utils.Errorf("get NaslScript failed: %s", db.Error)
	}
	return req, nil
}

func (p *NaslScript) CalcHash() string {
	return utils.CalcSha1(p.Script)
}
func (p *NaslScript) CreateOrUpdateNaslScript(db *gorm.DB) error {
	p.Hash = p.CalcHash()
	if p.OID == "" {
		return utils.Error("empty oid")
	}
	createNaslScript.Lock()
	defer createNaslScript.Unlock()
	db = db.Model(&NaslScript{})
	if db := db.Where("hash = ?", p.Hash).Assign(p).FirstOrCreate(&NaslScript{}); db.Error != nil {
		return utils.Errorf("create/update YakScript failed: %s", db.Error)
	}
	return nil
}
func (p *NaslScript) ToYakScript() *YakScript {
	params := []*ypb.YakScriptParam{}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil
	}
	paramsStr := strconv.Quote(string(raw))
	return &YakScript{
		ScriptName:           "__NaslScript__" + p.OriginFileName,
		Type:                 "nasl",
		Content:              p.Script,
		Level:                "info",
		Params:               paramsStr,
		Help:                 "",
		Author:               "",
		Tags:                 p.Tags,
		Ignored:              false,
		FromLocal:            false,
		LocalPath:            "",
		IsHistory:            false,
		FromStore:            false,
		IsGeneralModule:      false,
		FromGit:              "",
		IsBatchScript:        false,
		IsExternal:           false,
		EnablePluginSelector: false,
		PluginSelectorTypes:  "",
		sourceScript:         p,
	}
}
