package syntaxflow_scan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const maxTaskLocalRuleInputFileBytes int64 = 256 << 20

func loadTaskLocalSyntaxFlowRules(
	config *ssaconfig.SyntaxFlowRuleConfig,
) ([]*schema.SyntaxFlowRule, map[string]*schema.SyntaxFlowRule, error) {
	if config == nil {
		return nil, nil, utils.Error("task-local rule config is required")
	}
	path := strings.TrimSpace(config.TaskLocalInputFile)
	expectedSHA := strings.TrimSpace(config.TaskLocalInputSHA256)
	if path == "" || expectedSHA == "" || config.TaskLocalInputCount <= 0 {
		return nil, nil, utils.Error("task-local rule input file identity is incomplete")
	}
	decodedSHA, err := hex.DecodeString(expectedSHA)
	if err != nil || len(decodedSHA) != sha256.Size || expectedSHA != strings.ToLower(expectedSHA) {
		return nil, nil, utils.Error("task-local rule input sha256 must be lowercase hexadecimal")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, nil, utils.Wrap(err, "open task-local rule input file")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, utils.Wrap(err, "stat task-local rule input file")
	}
	if !info.Mode().IsRegular() {
		return nil, nil, utils.Error("task-local rule input must be a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, nil, utils.Error("task-local rule input file permissions must be 0600 or stricter")
	}
	if info.Size() > maxTaskLocalRuleInputFileBytes {
		return nil, nil, utils.Error("task-local rule input file exceeds the size limit")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxTaskLocalRuleInputFileBytes+1))
	if err != nil {
		return nil, nil, utils.Wrap(err, "read task-local rule input file")
	}
	if int64(len(raw)) > maxTaskLocalRuleInputFileBytes {
		return nil, nil, utils.Error("task-local rule input file exceeds the size limit")
	}
	actualSHA := sha256.Sum256(raw)
	if hex.EncodeToString(actualSHA[:]) != expectedSHA {
		return nil, nil, utils.Error("task-local rule input file sha256 mismatch")
	}

	var payload ssaconfig.TaskLocalRuleInputFile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return nil, nil, utils.Wrap(err, "decode task-local rule input file")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, nil, utils.Error("decode task-local rule input file: trailing JSON value")
	}
	if payload.Version != ssaconfig.TaskLocalRuleInputFileVersionV1 {
		return nil, nil, utils.Errorf("unsupported task-local rule input file version: %s", payload.Version)
	}
	if len(payload.Rules) != config.TaskLocalInputCount {
		return nil, nil, utils.Errorf(
			"task-local rule input count mismatch: expected=%d actual=%d",
			config.TaskLocalInputCount,
			len(payload.Rules),
		)
	}
	if len(payload.Metadata) != config.TaskLocalInputCount {
		return nil, nil, utils.Errorf(
			"task-local rule metadata count mismatch: expected=%d actual=%d",
			config.TaskLocalInputCount,
			len(payload.Metadata),
		)
	}
	return parseTaskLocalSyntaxFlowRulesWithMetadata(payload.Rules, payload.Metadata)
}

func parseTaskLocalSyntaxFlowRulesWithMetadata(
	inputs []*ypb.SyntaxFlowRuleInput,
	metadata map[string]ssaconfig.TaskLocalRuleMetadata,
) ([]*schema.SyntaxFlowRule, map[string]*schema.SyntaxFlowRule, error) {
	rules, _, err := parseTaskLocalSyntaxFlowRules(inputs)
	if err != nil {
		return nil, nil, err
	}
	libraries := make(map[string]*schema.SyntaxFlowRule)
	for _, rule := range rules {
		item, ok := metadata[rule.RuleName]
		if !ok {
			return nil, nil, utils.Errorf("task-local rule metadata is missing for %s", rule.RuleName)
		}
		if strings.TrimSpace(item.AssetID) == "" {
			return nil, nil, utils.Errorf("task-local rule %s asset_id is required", rule.RuleName)
		}
		rule.RuleId = item.AssetID
		rule.Version = strings.TrimSpace(item.Version)
		rule.Title = strings.TrimSpace(item.Title)
		if rule.Title == "" {
			rule.Title = rule.RuleName
		}
		rule.TitleZh = strings.TrimSpace(item.TitleZh)
		language := strings.TrimSpace(item.Language)
		if language == "" {
			rule.Language = ssaconfig.General
		} else {
			validated, err := ssaconfig.ValidateLanguage(language)
			if err != nil {
				return nil, nil, utils.Wrapf(err, "validate task-local rule %s metadata language", rule.RuleName)
			}
			rule.Language = validated
		}
		rule.Purpose = schema.SyntaxFlowRulePurposeType(strings.TrimSpace(item.Purpose))
		rule.Tag = strings.TrimSpace(item.Tag)
		rule.CWE = schema.StringArray(append([]string(nil), item.CWE...))
		rule.CVE = strings.TrimSpace(item.CVE)
		rule.RiskType = strings.TrimSpace(item.RiskType)
		rule.Type = schema.ValidRuleType(item.Type)
		rule.Severity = schema.ValidSeverityType(item.Severity)
		rule.Description = strings.TrimSpace(item.Description)
		rule.Solution = strings.TrimSpace(item.Solution)
		rule.IsBuildInRule = item.IsBuiltin
		rule.Verified = item.Verified
		rule.AllowIncluded = item.AllowIncluded
		rule.IncludedName = strings.TrimSpace(item.IncludedName)
		rule.Groups = make([]*schema.SyntaxFlowGroup, 0, len(item.Groups))
		for _, groupName := range item.Groups {
			if groupName = strings.TrimSpace(groupName); groupName != "" {
				rule.Groups = append(rule.Groups, &schema.SyntaxFlowGroup{GroupName: groupName})
			}
		}
		rule.Hash = strings.TrimSpace(item.ContentHash)
		rule.NeedUpdate = false
		// Metadata is authoritative for snapshot-derived dispatch: a synced tag
		// such as `source|secrets` must restore source mode even if an older
		// compiler produced an empty Mode/Tag on the parsed rule object.
		if itemTag := strings.TrimSpace(item.Tag); itemTag != "" {
			rule.Tag = itemTag
			for _, part := range strings.Split(itemTag, "|") {
				switch strings.ToLower(strings.TrimSpace(part)) {
				case "source", "pattern", "sfpattern":
					rule.Mode = schema.SFR_MODE_SOURCE
				}
			}
		}
		// AlertDesc is a derived projection of the canonical rule content. Keep
		// the parser-produced value when an older/minimal bundle omits it;
		// otherwise a matching `alert` still executes but cannot materialize a
		// risk. A present JSON object remains authoritative and replaces it.
		if raw := bytes.TrimSpace(item.AlertDesc); len(raw) > 0 && !bytes.Equal(raw, []byte("null")) {
			var alertDesc schema.MapEx[string, *schema.SyntaxFlowDescInfo]
			if err := json.Unmarshal(raw, &alertDesc); err != nil {
				return nil, nil, utils.Wrapf(err, "decode task-local rule %s alert metadata", rule.RuleName)
			}
			rule.AlertDesc = alertDesc
		}
		if rule.AllowIncluded {
			for _, libraryName := range []string{rule.Title, rule.IncludedName} {
				libraryName = strings.TrimSpace(libraryName)
				if libraryName == "" {
					continue
				}
				if existing, exists := libraries[libraryName]; exists && existing != rule {
					return nil, nil, utils.Errorf("duplicate task-local syntaxflow library name: %s", libraryName)
				}
				libraries[libraryName] = rule
			}
		}
	}
	return rules, libraries, nil
}

// parseTaskLocalSyntaxFlowRules compiles immutable dispatch input without
// calling yakit.ParseSyntaxFlowInput, whose legacy behavior migrates rules into
// the process-wide profile database.
func parseTaskLocalSyntaxFlowRules(
	inputs []*ypb.SyntaxFlowRuleInput,
) ([]*schema.SyntaxFlowRule, map[string]*schema.SyntaxFlowRule, error) {
	rules := make([]*schema.SyntaxFlowRule, 0, len(inputs))
	libraries := make(map[string]*schema.SyntaxFlowRule)
	for index, input := range inputs {
		if input == nil {
			return nil, nil, utils.Errorf("task-local rule input %d is nil", index)
		}
		content := strings.TrimSpace(input.GetContent())
		if content == "" {
			return nil, nil, utils.Errorf("task-local rule input %d content is required", index)
		}
		rule, err := sfdb.CheckSyntaxFlowRuleContent(content)
		if err != nil {
			return nil, nil, utils.Wrapf(err, "compile task-local rule input %d", index)
		}
		if rule == nil {
			return nil, nil, utils.Errorf("compile task-local rule input %d returned no rule", index)
		}

		rule.Content = content
		if name := strings.TrimSpace(input.GetRuleName()); name != "" {
			rule.RuleName = name
			if strings.TrimSpace(rule.Title) == "" {
				rule.Title = name
			}
		}
		if strings.TrimSpace(rule.RuleName) == "" {
			return nil, nil, utils.Errorf("task-local rule input %d rule name is required", index)
		}
		if language := strings.TrimSpace(input.GetLanguage()); language != "" {
			validated, err := ssaconfig.ValidateLanguage(language)
			if err != nil {
				return nil, nil, utils.Wrapf(err, "validate task-local rule %s language", rule.RuleName)
			}
			rule.Language = validated
		} else if rule.Language == "" {
			rule.Language = ssaconfig.General
		}
		if len(input.GetTags()) > 0 {
			rule.Tag = strings.Join(input.GetTags(), "|")
		}
		if description := strings.TrimSpace(input.GetDescription()); description != "" {
			rule.Description = description
		}

		rule.NormalizeMode()
		rules = append(rules, rule)
		if rule.AllowIncluded {
			for _, libraryName := range []string{rule.Title, rule.IncludedName} {
				if name := strings.TrimSpace(libraryName); name != "" {
					if existing, exists := libraries[name]; exists && existing != rule {
						return nil, nil, utils.Errorf("duplicate task-local syntaxflow library name: %s", name)
					}
					libraries[name] = rule
				}
			}
		}
	}
	return rules, libraries, nil
}

func filterTaskLocalSyntaxFlowRulesByMode(
	rules []*schema.SyntaxFlowRule,
	modes []string,
) []*schema.SyntaxFlowRule {
	if len(rules) == 0 || len(modes) == 0 {
		return rules
	}
	normalized := make([]string, 0, len(modes))
	for _, mode := range modes {
		if trimmed := strings.TrimSpace(mode); trimmed != "" {
			normalized = append(normalized, string(schema.ValidRuleMode(trimmed)))
		}
	}
	if len(normalized) == 0 {
		return rules
	}
	filtered := make([]*schema.SyntaxFlowRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		rule.NormalizeMode()
		if slices.Contains(normalized, string(rule.Mode)) {
			filtered = append(filtered, rule)
		}
	}
	return filtered
}

// filterTaskLocalSyntaxFlowRulesByNames applies the launch-frozen rule-name
// subset to an immutable task-local snapshot. Empty name filters keep the whole
// snapshot (subject to the mode filter).
func filterTaskLocalSyntaxFlowRulesByNames(
	rules []*schema.SyntaxFlowRule,
	config *ssaconfig.SyntaxFlowRuleConfig,
) []*schema.SyntaxFlowRule {
	if config == nil {
		return rules
	}
	names := config.RuleFilter.GetRuleNames()
	if len(names) == 0 {
		names = config.RuleNames
	}
	if len(names) == 0 {
		return rules
	}
	selected := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			selected[name] = struct{}{}
		}
	}
	if len(selected) == 0 {
		return rules
	}
	filtered := make([]*schema.SyntaxFlowRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		if _, ok := selected[rule.RuleName]; ok {
			filtered = append(filtered, rule)
		}
	}
	return filtered
}

func ruleInputResultKind(taskLocal bool) schema.SyntaxflowResultKind {
	if taskLocal {
		return schema.SFResultKindScan
	}
	return schema.SFResultKindDebug
}
