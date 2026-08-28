package schema

import (
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

// ContextMenuBinding stores the local user's visibility and invocation
// preferences for one action exposed by a context-menu plugin.
type ContextMenuBinding struct {
	gorm.Model

	PluginUUID   string `json:"plugin_uuid" gorm:"index"`
	ActionID     string `json:"action_id" gorm:"index"`
	Enabled      bool   `json:"enabled" gorm:"index"`
	Sort         int64  `json:"sort"`
	Shortcut     string `json:"shortcut"`
	ResultMode   string `json:"result_mode"`
	AskBeforeRun bool   `json:"ask_before_run"`

	Hash string `json:"-" gorm:"unique_index"`
}

func (b *ContextMenuBinding) CalcHash() string {
	return utils.CalcSha1(strings.TrimSpace(b.PluginUUID), strings.TrimSpace(b.ActionID))
}

func (b *ContextMenuBinding) BeforeSave() error {
	b.PluginUUID = strings.TrimSpace(b.PluginUUID)
	b.ActionID = strings.TrimSpace(b.ActionID)
	if b.PluginUUID == "" || b.ActionID == "" {
		return utils.Error("context-menu binding requires plugin UUID and action ID")
	}
	b.Hash = b.CalcHash()
	return nil
}
