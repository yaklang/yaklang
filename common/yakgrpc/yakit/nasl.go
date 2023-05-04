package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
)

type NaslScript struct {
	gorm.Model

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
func QueryScriptByOID(db *gorm.DB, oid string) (*NaslScript, error) {
	req := &NaslScript{}
	if db := db.Model(&NaslScript{}).Where("o_id = ?", oid).First(req); db.Error != nil {
		return nil, utils.Errorf("get NaslScript failed: %s", db.Error)
	}
	return req, nil
}
func QueryScriptByName(db *gorm.DB, name string) (*NaslScript, error) {
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
