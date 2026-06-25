package loop_ssa_api_discovery

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

const routingProfileSchemaVersion = 1

// RoutingProfileV1 与 routing_profile.json / DiscoverySession.RoutingProfileJSON 对齐的 v1 形态。
type RoutingProfileV1 struct {
	SchemaVersion       int                     `json:"schema_version"`
	ValidatedAt         string                  `json:"validated_at"`
	ValidationStatus    string                  `json:"validation_status"`
	Target              RoutingProfileTarget    `json:"target"`
	URLSpaces           []RoutingURLSpace       `json:"url_spaces"`
	DefaultSpaceID      string                  `json:"default_space_id"`
	PathConventions     RoutingPathConventions  `json:"path_conventions"`
	CalibrationProbes   []RoutingCalibrationProbe `json:"calibration_probes"`
	EffectiveBases      []RoutingEffectiveBase  `json:"effective_bases"`
}

// RoutingProfileTarget 摘要目标与 context-path。
type RoutingProfileTarget struct {
	Raw              string `json:"raw"`
	EffectiveOrigin  string `json:"effective_origin"`
	ContextPath      string `json:"context_path"`
	Notes            string `json:"notes"`
}

// RoutingURLSpace 一条逻辑 URL 空间（如 public / admin）。
type RoutingURLSpace struct {
	ID                  string           `json:"id"`
	Label               string          `json:"label"`
	MountPrefix         string          `json:"mount_prefix"`
	Confidence          float64         `json:"confidence"`
	Evidence            []RoutingEvidence `json:"evidence"`
	DefaultForPackages  []string        `json:"default_for_packages"`
}

// RoutingEvidence 推断依据指针。
type RoutingEvidence struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
	Hint string `json:"hint"`
}

// RoutingPathConventions 静态 harvest 风险标记。
type RoutingPathConventions struct {
	DuplicateSegmentRisk string `json:"duplicate_segment_risk"`
	AntPatternsInHarvest bool   `json:"ant_patterns_in_harvest"`
	Notes                string `json:"notes"`
}

// RoutingCalibrationProbe 校准阶段探针记录。
type RoutingCalibrationProbe struct {
	URL        string `json:"url"`
	Purpose    string `json:"purpose"`
	StatusCode int    `json:"status_code"`
	Outcome    string `json:"outcome"`
	Notes      string `json:"notes"`
}

// RoutingEffectiveBase 某 url_space 下用于 JoinProbeURL 的完整 base（可含 path）。
type RoutingEffectiveBase struct {
	SpaceID string `json:"space_id"`
	BaseURL string `json:"base_url"`
}

// ParseRoutingProfileJSON 解析但不强校验。
func ParseRoutingProfileJSON(raw string) (*RoutingProfileV1, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var p RoutingProfileV1
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, utils.Wrapf(err, "routing profile json")
	}
	return &p, nil
}

// RoutingProfileFromSession 从会话字段读取。
func RoutingProfileFromSession(sess *store.DiscoverySession) (*RoutingProfileV1, error) {
	if sess == nil {
		return nil, nil
	}
	return ParseRoutingProfileJSON(sess.RoutingProfileJSON)
}

// ValidateRoutingProfileForCommit 提交前校验。
func ValidateRoutingProfileForCommit(p *RoutingProfileV1) error {
	if p == nil {
		return utils.Error("routing profile is nil")
	}
	if p.SchemaVersion != routingProfileSchemaVersion {
		return utils.Errorf("schema_version must be %d", routingProfileSchemaVersion)
	}
	st := strings.TrimSpace(strings.ToLower(p.ValidationStatus))
	switch st {
	case "confirmed", "provisional", "failed":
		p.ValidationStatus = st
	default:
		return utils.Errorf("validation_status must be confirmed|provisional|failed, got %q", p.ValidationStatus)
	}
	if st == "failed" {
		return nil
	}
	if len(p.EffectiveBases) == 0 {
		return utils.Error("effective_bases must be non-empty unless validation_status is failed")
	}
	seenSpace := make(map[string]struct{})
	for _, eb := range p.EffectiveBases {
		b := strings.TrimSpace(eb.BaseURL)
		if b == "" {
			return utils.Error("effective_bases[].base_url required")
		}
		if _, err := url.Parse(b); err != nil {
			return utils.Wrapf(err, "invalid base_url %q", eb.BaseURL)
		}
		sid := strings.TrimSpace(eb.SpaceID)
		if sid == "" {
			return utils.Error("effective_bases[].space_id required")
		}
		if _, ok := seenSpace[sid]; ok {
			return utils.Errorf("duplicate space_id in effective_bases: %s", sid)
		}
		seenSpace[sid] = struct{}{}
	}
	return nil
}

// CanonicalRoutingProfileJSON 填默认时间戳后序列化（紧凑校验后再存也可）。
func CanonicalRoutingProfileJSON(p *RoutingProfileV1) (string, error) {
	if p.ValidatedAt == "" {
		p.ValidatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// WriteRoutingProfileFile 写入 workdir/ssa_discovery/routing_profile.json。
func WriteRoutingProfileFile(workDir string, rawJSON string) error {
	if workDir == "" {
		return utils.Error("empty workDir")
	}
	path := store.RoutingProfilePath(workDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(rawJSON), 0o644)
}

// packageGlobMatch 将简单 glob（仅 * 通配）匹配到全限定类名。
func packageGlobMatch(handlerClass, pattern string) bool {
	handlerClass = strings.TrimSpace(handlerClass)
	pattern = strings.TrimSpace(pattern)
	if handlerClass == "" || pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "*") {
		return strings.HasPrefix(handlerClass, pattern)
	}
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '*' {
			b.WriteString(".*")
		} else {
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	b.WriteByte('$')
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(handlerClass)
}

// PickURLSpaceIDForHandler 按 url_spaces.default_for_packages 选择空间；否则 default_space_id；再否则 "public"。
func PickURLSpaceIDForHandler(p *RoutingProfileV1, handlerClass string) string {
	if p == nil {
		return "public"
	}
	for _, sp := range p.URLSpaces {
		for _, pat := range sp.DefaultForPackages {
			if packageGlobMatch(handlerClass, pat) {
				return strings.TrimSpace(sp.ID)
			}
		}
	}
	if s := strings.TrimSpace(p.DefaultSpaceID); s != "" {
		return s
	}
	return "public"
}

// EffectiveProbeBaseForHandler 返回用于拼接 path_pattern 的 base（含 context+mount）；无 profile 时用 EffectiveTargetBaseURL。
func EffectiveProbeBaseForHandler(p *RoutingProfileV1, sess *store.DiscoverySession, handlerClass string) string {
	fallback := EffectiveTargetBaseURL(sess)
	if p == nil || len(p.EffectiveBases) == 0 {
		return fallback
	}
	sid := PickURLSpaceIDForHandler(p, handlerClass)
	for _, eb := range p.EffectiveBases {
		if strings.TrimSpace(eb.SpaceID) == sid {
			if b := strings.TrimSpace(eb.BaseURL); b != "" {
				return strings.TrimRight(b, "/")
			}
		}
	}
	for _, eb := range p.EffectiveBases {
		if b := strings.TrimSpace(eb.BaseURL); b != "" {
			return strings.TrimRight(b, "/")
		}
	}
	return fallback
}

// GroupHttpEndpointIDsByProbeBase 将 http_endpoints 按探测 base 分组（用于批量 Yak 多次调用）。
func GroupHttpEndpointIDsByProbeBase(sess *store.DiscoverySession, repo *store.Repository, sessionID uint) (map[string][]uint, error) {
	if repo == nil {
		return nil, utils.Error("nil repo")
	}
	p, _ := RoutingProfileFromSession(sess)
	eps, err := repo.ListHttpEndpoints(sessionID)
	if err != nil {
		return nil, err
	}
	groups := make(map[string][]uint)
	for _, ep := range eps {
		base := EffectiveProbeBaseForHandler(p, sess, ep.HandlerClass)
		if strings.TrimSpace(base) == "" {
			base = EffectiveTargetBaseURL(sess)
		}
		groups[base] = append(groups[base], ep.ID)
	}
	for base := range groups {
		sort.Slice(groups[base], func(i, j int) bool { return groups[base][i] < groups[base][j] })
	}
	return groups, nil
}

// JoinUintCSV 将 id 列表转成 Yak 端 endpoint-ids 参数。
func JoinUintCSV(ids []uint) string {
	if len(ids) == 0 {
		return ""
	}
	var parts []string
	for _, id := range ids {
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	return strings.Join(parts, ",")
}

// SortedProbeBaseKeys 稳定顺序遍历分组键。
func SortedProbeBaseKeys(groups map[string][]uint) []string {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// FormatEffectiveBasesForPrompt 供 Phase3 reactive 展示。
func FormatEffectiveBasesForPrompt(sess *store.DiscoverySession) string {
	p, err := RoutingProfileFromSession(sess)
	if err != nil || p == nil || len(p.EffectiveBases) == 0 {
		return ""
	}
	b, err := json.MarshalIndent(p.EffectiveBases, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}
