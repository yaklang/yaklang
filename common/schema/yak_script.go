package schema

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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
	sourceScript   *NaslScript // 用于存储原始的 script(可能是由原类型是NaslScript)

	IsCorePlugin bool `json:"is_core_plugin"` // 判断是否是核心插件
	// 废弃字段
	RiskType string `json:"risk_type"`
	// 漏洞详情 建议，描述，cwe
	RiskDetail string `json:"risk_detail"`
	// 漏洞类型-补充说明 废弃
	RiskAnnotation string `json:"risk_annotation"`
	// 协作者
	CollaboratorInfo string `json:"collaborator_info"`
}

func (s *YakScript) BeforeSave() error {
	if s.ScriptName == "" {
		return utils.Errorf("empty script name is denied")
	}

	if utils.MatchAnyOfSubString(s.ScriptName, "|") {
		s.ScriptName = strings.ReplaceAll(s.ScriptName, "|", "/")
	}

	resRaw, _ := strconv.Unquote(s.Params)
	if resRaw != "" {
		var res []any
		err := json.Unmarshal([]byte(resRaw), &res)
		if err != nil {
			log.Warnf(`json.Unmarshal script.Params failed: %s data: %v`, err, s.Params)
		}
		for _, rawIf := range res {
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
	return nil
}

func (s *YakScript) AfterCreate(tx *gorm.DB) (err error) {
	broadcastData.Call("yakscript", "create")
	return nil
}

func (s *YakScript) AfterUpdate(tx *gorm.DB) (err error) {
	broadcastData.Call("yakscript", "update")
	return nil
}

func (s *YakScript) AfterDelete(tx *gorm.DB) (err error) {
	broadcastData.Call("yakscript", "delete")
	return nil
}

func (s *YakScript) GetParams() []*ypb.YakScriptParam {
	var paras []*ypb.YakScriptParam
	params, err := strconv.Unquote(s.Params)
	if err != nil {
		log.Debugf("%v: unquote params string error: %s(%v)", s.ScriptName, err, s.Params)
		return nil
	}
	err = json.Unmarshal([]byte(params), &paras)
	if err != nil {
		log.Errorf("Unmarshal params string error: %s", err)
		return nil
	}
	return paras
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
	var riskDetail []*ypb.YakRiskInfo
	if s.RiskDetail != "" && s.RiskDetail != `""` && s.RiskDetail != "{}" { //"{}"
		r, err := strconv.Unquote(s.RiskDetail)
		if err != nil {
			r = s.RiskDetail
		}
		err = json.Unmarshal([]byte(r), &riskDetail)
		if err != nil { // errors may occur due to version iterations has break change, so we just ignore it (this field has been deprecated actually)
			// log.Errorf("unmarshal RiskDetail failed: %s", err)
			// spew.Dump([]byte(r))
		}
	}

	var collaboratorInfo []*ypb.Collaborator
	if s.CollaboratorInfo != "" && s.CollaboratorInfo != `""` {
		c, _ := strconv.Unquote(s.CollaboratorInfo)
		err := json.Unmarshal([]byte(c), &collaboratorInfo)
		if err != nil {
			log.Errorf("unmarshal collaboratorInfo failed: %s", err)
			spew.Dump([]byte(c))
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
		UpdatedAt:            s.UpdatedAt.Unix(),
		RiskAnnotation:       s.RiskAnnotation,
		RiskInfo:             riskDetail,
		IsCorePlugin:         s.IsCorePlugin,
	}
	/*if s.Type == "mitm" {
		script.Params = mitmPluginDefaultPlugins
	}*/
	return script
}
