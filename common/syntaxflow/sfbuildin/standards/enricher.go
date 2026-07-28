package standards

import (
	"embed"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v3"
)

//go:embed mappings.yaml
var mappingsFS embed.FS

// StandardMappings 标准映射配置
type StandardMappings struct {
	Version           string                       `yaml:"version"`
	UpdatedAt         string                       `yaml:"updated_at"`
	CWEToOWASP2021    map[string][]string          `yaml:"cwe_to_owasp_2021"`
	CWEToOWASP2017    map[string][]string          `yaml:"cwe_to_owasp_2017"`
	CWENames          map[string]string            `yaml:"cwe_names"`
	FrameworkGroups   []FrameworkGroupDef          `yaml:"framework_groups"`
	CWETop25_2023     []string                     `yaml:"cwe_top_25_2023"`
}

// FrameworkGroupDef 框架分组定义
type FrameworkGroupDef struct {
	GroupName    string   `yaml:"group_name"`
	Description  string   `yaml:"description"`
	PathPatterns []string `yaml:"path_patterns"`
	Tags         []string `yaml:"tags"`
}

// RuleMetadataEnricher 规则元数据增强器
type RuleMetadataEnricher struct {
	mappings *StandardMappings
	mu       sync.RWMutex
}

var (
	globalEnricher     *RuleMetadataEnricher
	enricherInitOnce   sync.Once
	enricherInitError  error
)

// GetGlobalEnricher 获取全局单例增强器
func GetGlobalEnricher() (*RuleMetadataEnricher, error) {
	enricherInitOnce.Do(func() {
		globalEnricher, enricherInitError = NewRuleMetadataEnricher()
	})
	return globalEnricher, enricherInitError
}

// NewRuleMetadataEnricher 创建新的元数据增强器
func NewRuleMetadataEnricher() (*RuleMetadataEnricher, error) {
	data, err := mappingsFS.ReadFile("mappings.yaml")
	if err != nil {
		return nil, utils.Wrapf(err, "read mappings.yaml failed")
	}

	var mappings StandardMappings
	if err := yaml.Unmarshal(data, &mappings); err != nil {
		return nil, utils.Wrapf(err, "unmarshal mappings.yaml failed")
	}

	log.Infof("loaded standard mappings v%s (updated: %s)", mappings.Version, mappings.UpdatedAt)
	log.Infof("  - CWE to OWASP 2021: %d mappings", len(mappings.CWEToOWASP2021))
	log.Infof("  - CWE names: %d entries", len(mappings.CWENames))
	log.Infof("  - Framework groups: %d groups", len(mappings.FrameworkGroups))

	return &RuleMetadataEnricher{
		mappings: &mappings,
	}, nil
}

// EnrichGroupNames 为规则增强分组名称（已废弃：请使用 EnrichAtomicTags）
// 保留供迁移期测试；sync 路径不再写入 Group。
func (e *RuleMetadataEnricher) EnrichGroupNames(
	ruleName string,
	ruleFilePath string,
	cwes []string,
) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var groupNames []string

	// 1. 根据 CWE 映射到 OWASP 2021 分组
	owaspGroups := e.mapCWEToOWASP(cwes, e.mappings.CWEToOWASP2021)
	groupNames = append(groupNames, owaspGroups...)

	// 3. 检查是否属于 CWE Top 25
	if e.isCWETop25(cwes) {
		groupNames = append(groupNames, "CWE Top 25 (2023)")
	}

	// 4. 根据路径匹配框架分组
	frameworkGroups := e.matchFrameworkGroups(ruleFilePath)
	groupNames = append(groupNames, frameworkGroups...)

	// 5. 去重
	groupNames = utils.RemoveRepeatedWithStringSlice(groupNames)

	return groupNames
}

// EnrichAtomicTags builds standard short tags for a rule (docs/design/rule-package.md):
//   - cwe:<num> for each CWE (preferred vuln dimension)
//   - framework/component short tags (spring, shiro, sca, ...)
//   - tech-point tags (sqli, xss, ...) only when no CWE is present
// Never emits language / severity / purpose.
func (e *RuleMetadataEnricher) EnrichAtomicTags(
	ruleName string,
	ruleFilePath string,
	cwes []string,
) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var tags []string
	hasCWE := false
	for _, cwe := range cwes {
		formatted := formatCWENumber(cwe)
		if formatted == "" {
			continue
		}
		hasCWE = true
		num := strings.TrimPrefix(formatted, "CWE-")
		tags = append(tags, "cwe:"+num)
	}

	normalizedPath := filepath.ToSlash(strings.ToLower(ruleFilePath))
	for _, fg := range e.mappings.FrameworkGroups {
		matched := false
		for _, pattern := range fg.PathPatterns {
			if matchPathPattern(normalizedPath, strings.ToLower(pattern)) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if short := frameworkShortTag(fg.GroupName); short != "" {
			tags = append(tags, short)
		}
		// keep non-language tags from mapping (e.g. sca, dependency, framework)
		for _, t := range fg.Tags {
			t = strings.ToLower(strings.TrimSpace(t))
			if t == "" || t == "language" || t == "java" || t == "php" || t == "python" || t == "golang" || t == "javascript" {
				continue
			}
			tags = append(tags, t)
		}
	}

	if !hasCWE {
		tags = append(tags, inferTechPointTags(ruleFilePath, ruleName)...)
	}

	return utils.RemoveRepeatedWithStringSlice(tags)
}

func frameworkShortTag(groupName string) string {
	name := strings.TrimSpace(groupName)
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "shiro"):
		return "shiro"
	case strings.Contains(lower, "spring"):
		return "spring"
	case strings.Contains(lower, "struts"):
		return "struts"
	case strings.Contains(lower, "laravel"):
		return "laravel"
	case strings.Contains(lower, "thinkphp"):
		return "thinkphp"
	case strings.Contains(lower, "django"):
		return "django"
	case strings.Contains(lower, "sca"):
		return "sca"
	default:
		return ""
	}
}

func inferTechPointTags(ruleFilePath, ruleName string) []string {
	blob := strings.ToLower(ruleFilePath + " " + ruleName)
	var tags []string
	add := func(cond bool, tag string) {
		if cond {
			tags = append(tags, tag)
		}
	}
	add(strings.Contains(blob, "sql-injection") || strings.Contains(blob, "sqli") || strings.Contains(blob, "sql_inject"), "sqli")
	add(strings.Contains(blob, "xss") || strings.Contains(blob, "cross-site"), "xss")
	add(strings.Contains(blob, "ssrf"), "ssrf")
	add(strings.Contains(blob, "xxe") || strings.Contains(blob, "xml-external"), "xxe")
	add(strings.Contains(blob, "rce") || strings.Contains(blob, "command-inject") || strings.Contains(blob, "os-command"), "rce")
	add(strings.Contains(blob, "path-traversal") || strings.Contains(blob, "path_traversal") || strings.Contains(blob, "lfi"), "path-traversal")
	add(strings.Contains(blob, "deserialize") || strings.Contains(blob, "unserialize") || strings.Contains(blob, "pickle"), "deserialization")
	add(strings.Contains(blob, "open-redirect") || strings.Contains(blob, "redirect"), "open-redirect")
	add(strings.Contains(blob, "ssti") || strings.Contains(blob, "template-injection"), "ssti")
	add(strings.Contains(blob, "ldap"), "ldap-injection")
	return tags
}

// mapCWEToOWASP 将 CWE 列表映射到 OWASP 分组
func (e *RuleMetadataEnricher) mapCWEToOWASP(cwes []string, cweToOwasp map[string][]string) []string {
	var groups []string
	seen := make(map[string]bool)

	for _, cwe := range cwes {
		cwe = strings.TrimSpace(cwe)
		if cwe == "" {
			continue
		}

		// 确保格式统一（CWE-89 大写）
		cwe = formatCWENumber(cwe)

		if owaspList, ok := cweToOwasp[cwe]; ok {
			for _, owasp := range owaspList {
				if !seen[owasp] {
					groups = append(groups, owasp)
					seen[owasp] = true
				}
			}
		}
	}

	return groups
}

// isCWETop25 检查 CWE 是否在 Top 25 列表中
func (e *RuleMetadataEnricher) isCWETop25(cwes []string) bool {
	for _, cwe := range cwes {
		cwe = formatCWENumber(cwe)
		for _, topCWE := range e.mappings.CWETop25_2023 {
			if cwe == topCWE {
				return true
			}
		}
	}
	return false
}

// matchFrameworkGroups 根据路径匹配框架分组
func (e *RuleMetadataEnricher) matchFrameworkGroups(ruleFilePath string) []string {
	var matched []string

	// 标准化路径分隔符
	normalizedPath := filepath.ToSlash(strings.ToLower(ruleFilePath))

	for _, fg := range e.mappings.FrameworkGroups {
		for _, pattern := range fg.PathPatterns {
			// 简化的模式匹配（支持 ** 通配符）
			pattern = strings.ToLower(pattern)
			if matchPathPattern(normalizedPath, pattern) {
				matched = append(matched, fg.GroupName)
				break // 一个规则只匹配一次该分组
			}
		}
	}

	return matched
}

// GetCWEName 获取 CWE 的标准名称
func (e *RuleMetadataEnricher) GetCWEName(cwe string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	cwe = formatCWENumber(cwe)
	if name, ok := e.mappings.CWENames[cwe]; ok {
		return name
	}
	return cwe // 如果没有映射，返回 CWE 编号本身
}

// GetOWASPByCWE 获取 CWE 对应的 OWASP 分类
func (e *RuleMetadataEnricher) GetOWASPByCWE(cwe string) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	cwe = formatCWENumber(cwe)
	if owasp, ok := e.mappings.CWEToOWASP2021[cwe]; ok {
		return owasp
	}
	return nil
}

// GetAllFrameworkGroups 获取所有框架分组定义
func (e *RuleMetadataEnricher) GetAllFrameworkGroups() []FrameworkGroupDef {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.mappings.FrameworkGroups
}

// formatCWENumber 标准化 CWE 编号格式
// "cwe-89" -> "CWE-89"
// "89" -> "CWE-89"
// "CWE-89" -> "CWE-89"
func formatCWENumber(cwe string) string {
	cwe = strings.TrimSpace(cwe)
	if cwe == "" {
		return ""
	}

	// 移除可能的前缀
	cwe = strings.TrimPrefix(strings.ToUpper(cwe), "CWE-")
	cwe = strings.TrimPrefix(cwe, "cwe-")

	// 添加标准前缀
	return "CWE-" + cwe
}

// matchPathPattern 简化的路径模式匹配
// 支持 ** (任意层级目录) 和 * (单层目录或文件名)
func matchPathPattern(path, pattern string) bool {
	// 移除开头的 **/
	pattern = strings.TrimPrefix(pattern, "**/")

	// 如果模式包含 **，使用 Contains 匹配
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		for _, part := range parts {
			part = strings.Trim(part, "/")
			if part == "" {
				continue
			}
			if !strings.Contains(path, part) {
				return false
			}
		}
		return true
	}

	// 简单的 * 通配符匹配
	if strings.Contains(pattern, "*") {
		// 使用 filepath.Match（需要转换回系统路径）
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		if matched {
			return true
		}
		// 检查路径中是否包含模式（去除通配符后）
		patternWithoutWildcard := strings.ReplaceAll(pattern, "*", "")
		return strings.Contains(path, patternWithoutWildcard)
	}

	// 精确匹配或包含匹配
	return strings.Contains(path, pattern)
}

// ExtractCWEFromTags 从 tags 中提取 CWE 编号
// 这是一个工具函数，用于辅助规则导入流程
func ExtractCWEFromTags(tags []string) []string {
	var cwes []string
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if strings.HasPrefix(strings.ToUpper(tag), "CWE-") {
			cwes = append(cwes, formatCWENumber(tag))
		}
	}
	return utils.RemoveRepeatedWithStringSlice(cwes)
}
