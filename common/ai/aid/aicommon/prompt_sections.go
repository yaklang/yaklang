package aicommon

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/ytoken"
)

// PromptSectionRole classifies assembled prompt content by stability. These
// values are also part of the prompt_profile event contract.
type PromptSectionRole string

const (
	PromptSectionRoleHighStatic   PromptSectionRole = "high_static"
	PromptSectionRoleFrozenBlock  PromptSectionRole = "frozen_block"
	PromptSectionRoleSemiDynamic1 PromptSectionRole = "semi_dynamic_1"
	PromptSectionRoleSemiDynamic2 PromptSectionRole = "semi_dynamic_2"
	PromptSectionRoleTimelineOpen PromptSectionRole = "timelineOpen"
	PromptSectionRoleDynamic      PromptSectionRole = "dynamic"
)

type PromptSectionRoleZH string

const (
	PromptSectionRoleZHHighStatic   PromptSectionRoleZH = "高静态段"
	PromptSectionRoleZHFrozenBlock  PromptSectionRoleZH = "冻结块"
	PromptSectionRoleZHSemiDynamic1 PromptSectionRoleZH = "半动态段1"
	PromptSectionRoleZHSemiDynamic2 PromptSectionRoleZH = "半动态段2"
	PromptSectionRoleZHTimelineOpen PromptSectionRoleZH = "时间线开放段"
	PromptSectionRoleZHDynamic      PromptSectionRoleZH = "动态段"
)

func (role PromptSectionRole) Zh() PromptSectionRoleZH {
	switch role {
	case PromptSectionRoleHighStatic:
		return PromptSectionRoleZHHighStatic
	case PromptSectionRoleFrozenBlock:
		return PromptSectionRoleZHFrozenBlock
	case PromptSectionRoleSemiDynamic1:
		return PromptSectionRoleZHSemiDynamic1
	case PromptSectionRoleSemiDynamic2:
		return PromptSectionRoleZHSemiDynamic2
	case PromptSectionRoleTimelineOpen:
		return PromptSectionRoleZHTimelineOpen
	case PromptSectionRoleDynamic:
		return PromptSectionRoleZHDynamic
	default:
		return ""
	}
}

// PromptSectionObservation is the shared, typed section tree returned by prompt
// assembly. UI status construction and event publication belong to the caller.
type PromptSectionObservation struct {
	Key          string              `json:"key"`
	Label        string              `json:"label"`
	Role         PromptSectionRole   `json:"role"`
	RoleZh       PromptSectionRoleZH `json:"role_zh"`
	Included     bool                `json:"included"`
	Compressible bool                `json:"compressible"`
	Bytes        int                 `json:"bytes"`
	Lines        int                 `json:"lines"`
	// Bytes and lines include the node's content and all children. Container
	// token estimates sum the children, using the prompt budget's Qwen BPE.
	EstimatedTokens int                         `json:"estimated_tokens,omitempty"`
	Content         string                      `json:"content,omitempty"`
	Children        []*PromptSectionObservation `json:"children,omitempty"`
}

func NewPromptSectionObservation(key, label string, role PromptSectionRole, compressible bool, content string) *PromptSectionObservation {
	section := &PromptSectionObservation{
		Key: key, Label: label, Role: role, RoleZh: role.Zh(),
		Compressible: compressible, Content: aiprojection.RedactNonce(content),
	}
	section.refreshMetrics()
	return section
}

func NewPromptContainerSection(key, label string, role PromptSectionRole) *PromptSectionObservation {
	return &PromptSectionObservation{Key: key, Label: label, Role: role, RoleZh: role.Zh()}
}

func FinalizePromptContainerSection(section *PromptSectionObservation) *PromptSectionObservation {
	section.refreshMetrics()
	return section
}

func (s *PromptSectionObservation) IsIncluded() bool {
	return s != nil && s.Included
}

func (s *PromptSectionObservation) ContentBytes() int {
	if s == nil {
		return 0
	}
	return s.Bytes
}

func (s *PromptSectionObservation) LineCount() int {
	if s == nil {
		return 0
	}
	return s.Lines
}

func (s *PromptSectionObservation) refreshMetrics() {
	if s == nil {
		return
	}
	s.Bytes = len(s.Content)
	s.Lines = 0
	if s.Content != "" {
		s.Lines = strings.Count(s.Content, "\n") + 1
	}
	s.Included = strings.TrimSpace(s.Content) != ""
	s.EstimatedTokens = 0
	if len(s.Children) == 0 {
		s.EstimatedTokens = ytoken.CalcTokenCount(s.Content)
		return
	}
	for _, child := range s.Children {
		if child == nil {
			continue
		}
		child.refreshMetrics()
		if child.Included {
			s.Included = true
		}
		s.Bytes += child.Bytes
		s.Lines += child.Lines
		s.EstimatedTokens += child.EstimatedTokens
	}
}
