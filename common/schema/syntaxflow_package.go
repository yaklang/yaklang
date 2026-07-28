package schema

import (
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SyntaxFlow package source kinds.
const (
	SyntaxFlowPackageSourceEmbed  = "embed"
	SyntaxFlowPackageSourceOnline = "online"
	SyntaxFlowPackageSourceLocal  = "local"
	SyntaxFlowPackageSourceUser   = "user"
)

// Well-known package names.
const (
	SyntaxFlowPackageBuiltin = "builtin"
	SyntaxFlowPackageAgent   = "agent"
	SyntaxFlowPackageCustom  = "custom"
)

// SyntaxFlowPackage is a mutually-exclusive distribution unit for rules
// (import / export / sync / online). See docs/design/rule-package.md.
type SyntaxFlowPackage struct {
	gorm.Model

	// Name is the global package id (builtin, agent, custom, imported-...).
	Name string `json:"name" gorm:"unique_index"`

	// Version is package-level semver; used as the update gate.
	Version string `json:"version" gorm:"index"`

	// Description is human-readable package summary.
	Description string `json:"description"`

	// Source: embed | online | local | user
	Source string `json:"source" gorm:"index"`

	// IsBuiltin marks embed packages that cannot be deleted by users.
	IsBuiltin bool `json:"is_builtin" gorm:"index"`

	// Hash is optional content fingerprint of the installed package payload.
	Hash string `json:"hash" gorm:"index"`
}

func (p *SyntaxFlowPackage) TableName() string {
	return "syntax_flow_packages"
}

func (p *SyntaxFlowPackage) ToGRPCModel(ruleCount int32) *ypb.SyntaxFlowPackage {
	if p == nil {
		return nil
	}
	return &ypb.SyntaxFlowPackage{
		Name:        p.Name,
		Version:     p.Version,
		Description: p.Description,
		Source:      p.Source,
		IsBuiltin:   p.IsBuiltin,
		Hash:        p.Hash,
		RuleCount:   ruleCount,
	}
}
