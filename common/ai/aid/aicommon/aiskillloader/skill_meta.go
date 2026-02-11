package aiskillloader

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v3"
)

// SkillMeta represents the metadata parsed from a SKILL.md frontmatter.
// Compatible with the Cursor Agent Skills standard.
type SkillMeta struct {
	// Name is the unique identifier for this skill.
	// Lowercase letters, numbers, and hyphens only.
	Name string `yaml:"name" json:"name"`

	// Description describes what the skill does and when to use it.
	// Used by the agent to determine relevance.
	Description string `yaml:"description" json:"description"`

	// License is the license name or reference to a bundled license file.
	License string `yaml:"license,omitempty" json:"license,omitempty"`

	// Compatibility describes environment requirements.
	Compatibility string `yaml:"compatibility,omitempty" json:"compatibility,omitempty"`

	// Metadata is arbitrary key-value mapping for additional metadata.
	Metadata map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`

	// DisableModelInvocation when true means the skill is only included when explicitly invoked.
	DisableModelInvocation bool `yaml:"disable-model-invocation,omitempty" json:"disable_model_invocation,omitempty"`

	// Body is the markdown content after the frontmatter.
	Body string `yaml:"-" json:"body,omitempty"`
}

// Validate checks that the required fields are set.
func (m *SkillMeta) Validate() error {
	if m.Name == "" {
		return utils.Error("skill meta: name is required")
	}
	if m.Description == "" {
		return utils.Error("skill meta: description is required")
	}
	return nil
}

// BriefString returns a concise summary suitable for folded display.
func (m *SkillMeta) BriefString() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Skill: %s\n", m.Name))
	buf.WriteString(fmt.Sprintf("Description: %s\n", m.Description))
	if m.License != "" {
		buf.WriteString(fmt.Sprintf("License: %s\n", m.License))
	}
	if m.Compatibility != "" {
		buf.WriteString(fmt.Sprintf("Compatibility: %s\n", m.Compatibility))
	}
	return buf.String()
}

const frontmatterDelimiter = "---"

// ParseSkillMeta parses a SKILL.md content into SkillMeta.
// The content must start with YAML frontmatter delimited by "---".
func ParseSkillMeta(content string) (*SkillMeta, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, frontmatterDelimiter) {
		return nil, utils.Error("skill meta: content must start with YAML frontmatter (---)")
	}

	// Find the closing delimiter
	rest := content[len(frontmatterDelimiter):]
	idx := strings.Index(rest, "\n"+frontmatterDelimiter)
	if idx < 0 {
		return nil, utils.Error("skill meta: missing closing frontmatter delimiter (---)")
	}

	frontmatter := strings.TrimSpace(rest[:idx])
	body := strings.TrimSpace(rest[idx+len("\n"+frontmatterDelimiter):])

	meta := &SkillMeta{}
	if err := yaml.Unmarshal([]byte(frontmatter), meta); err != nil {
		return nil, utils.Wrapf(err, "skill meta: failed to parse YAML frontmatter")
	}

	meta.Body = body

	if err := meta.Validate(); err != nil {
		log.Warnf("skill meta validation warning: %v", err)
	}

	return meta, nil
}
