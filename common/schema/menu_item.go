package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type MenuItem struct {
	gorm.Model

	Group         string `json:"group" `
	Verbose       string `json:"verbose"`
	YakScriptName string `json:"yak_script_name"`
	Hash          string `json:"-" gorm:"unique_index"`

	// quoted json
	BatchPluginFilterJson string `json:"batch_plugin_filter_json"`
	Mode                  string `json:"mode"`
	MenuSort              int64  `json:"menu_sort"`
	GroupSort             int64  `json:"group_sort"`
}

func (m *MenuItem) CalcHash() string {
	key := m.Verbose
	if key == "" {
		key = m.YakScriptName
	}
	if key == "" {
		key = codec.Sha256(m.BatchPluginFilterJson)
	}
	return utils.CalcSha1(m.Group, m.Mode, key)
}

func (m *MenuItem) BeforeSave() error {
	if m.Group == "" {
		m.Group = "UserDefined"
	}

	m.Hash = m.CalcHash()
	return nil
}
