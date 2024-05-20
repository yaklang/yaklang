package schema

import (
	"encoding/json"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
	"sync"
)

var (
	createNaslScriptMutex = new(sync.Mutex)
)

type NaslScript struct {
	gorm.Model
	OriginFileName  string `json:"origin_file_name"`
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

func (p *NaslScript) CalcHash() string {
	return utils.CalcSha1(p.Script)
}
func (p *NaslScript) CreateOrUpdateNaslScript(db *gorm.DB) error {
	p.Hash = p.CalcHash()
	if p.OID == "" {
		return utils.Error("empty oid")
	}
	createNaslScriptMutex.Lock()
	defer createNaslScriptMutex.Unlock()
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
		ScriptName:           utils.EscapeInvalidUTF8Byte([]byte(p.OriginFileName)),
		Type:                 "nasl",
		Content:              utils.EscapeInvalidUTF8Byte([]byte(p.Script)),
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
