package aiprojection

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// ProjectionSectionKind identifies the presentation concern of a section.
// Only cache projection has a policy in this phase. The other kinds reserve a
// stable vocabulary for later request projections.
type ProjectionSectionKind string

const (
	ProjectionSectionCache    ProjectionSectionKind = "cache"
	ProjectionSectionTool     ProjectionSectionKind = "tool"
	ProjectionSectionSkill    ProjectionSectionKind = "skill"
	ProjectionSectionTimeline ProjectionSectionKind = "timeline"
	ProjectionSectionRaw      ProjectionSectionKind = "raw"
)

// ProjectionSection carries exact source bytes. Parse sets Kind and the private
// cache metadata; callers can also provide Kind and Raw as input fragments.
// Kind describes presentation, never tool or skill authorization.
type ProjectionSection struct {
	Kind    ProjectionSectionKind
	Raw     string
	content string
}

// ProjectionSections is the parser-to-projection representation. Items retain
// source order; cache contains typed, byte-exact candidates for cache policy.
// Manually constructed Items are normalized by Parse before projection.
type ProjectionSections struct {
	Original string
	Items    []ProjectionSection
	cache    cacheProjectionSections
	parsed   bool
}

// cacheProjectionSections contains the cache-related views of Items. Parse
// prepares candidate user boundaries; the projection policy chooses a layout.
type cacheProjectionSections struct {
	staticParts []ProjectionSection
	hasOther    bool
	userRaw     string
	frozen      []ProjectionSection
	semi        []ProjectionSection
	semi2       []ProjectionSection
	timeline    []ProjectionSection
}

// ProjectionInput describes material already selected by the caller. Sections
// take precedence over Prompt; explicit RawMessages take precedence over both.
type ProjectionInput struct {
	Prompt      string
	RawMessages []aispec.ChatDetail
	ActionTools []aispec.Tool
	Sections    *ProjectionSections
}

type ProjectionMetadata struct {
	CacheProjected       bool
	RawMessagesPreserved bool
}

type ProjectionResult struct {
	Messages []aispec.ChatDetail
	Tools    []aispec.Tool
	Metadata ProjectionMetadata
}

// Project determines the provider-visible message layout without recording
// cache statistics or changing action/tool availability. Unknown section kinds
// remain ordinary prompt text; they are never dropped.
func Project(input ProjectionInput) ProjectionResult {
	result := ProjectionResult{Tools: append([]aispec.Tool(nil), input.ActionTools...)}
	if len(input.RawMessages) > 0 {
		result.Messages = append([]aispec.ChatDetail(nil), input.RawMessages...)
		result.Metadata.RawMessagesPreserved = true
		return result
	}

	sections := input.Sections
	if sections == nil {
		sections = Parse(input.Prompt).Sections()
	} else if !sections.parsed {
		prompt := sections.Original
		if len(sections.Items) > 0 {
			var builder strings.Builder
			for _, section := range sections.Items {
				builder.WriteString(section.Raw)
			}
			prompt = builder.String()
		}
		sections = Parse(prompt).Sections()
	}
	if sections.Original == "" {
		return result
	}

	if projected := projectCache(sections); projected != nil && projected.IsHijacked {
		result.Messages = projected.Messages
		result.Metadata.CacheProjected = true
		return result
	}
	result.Messages = []aispec.ChatDetail{aispec.NewUserChatDetail(sections.Original)}
	return result
}
