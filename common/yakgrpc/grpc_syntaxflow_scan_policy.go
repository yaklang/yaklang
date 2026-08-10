//go:build !irify_exclude

package yakgrpc

import (
	"context"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySyntaxFlowScanPolicies(ctx context.Context, req *ypb.QuerySyntaxFlowScanPoliciesRequest) (*ypb.QuerySyntaxFlowScanPoliciesResponse, error) {
	cfg := ssaconfig.GetScanPoliciesConfig()
	if cfg == nil {
		return nil, utils.Error("scan policies config is unavailable")
	}

	want := make(map[string]struct{})
	for _, id := range req.GetPolicyIds() {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = struct{}{}
		}
	}

	ids := make([]string, 0, len(cfg.Policies))
	for id := range cfg.Policies {
		if len(want) > 0 {
			if _, ok := want[id]; !ok {
				continue
			}
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	resp := &ypb.QuerySyntaxFlowScanPoliciesResponse{
		Version: cfg.Version,
	}
	for _, id := range ids {
		def := cfg.Policies[id]
		policy := &ssaconfig.ScanPolicyConfig{PolicyType: id}
		resp.Policies = append(resp.Policies, &ypb.SyntaxFlowScanPolicy{
			Id:          id,
			Name:        def.Name,
			Description: def.Description,
			Icon:        def.Icon,
			Filter:      policy.MapToRuleFilter(),
		})
	}
	for _, cat := range cfg.Categories {
		resp.Categories = append(resp.Categories, &ypb.SyntaxFlowScanPolicyCategory{
			Id:        cat.ID,
			Name:      cat.Name,
			PolicyIds: append([]string{}, cat.Policies...),
		})
	}
	catKeys := make([]string, 0, len(cfg.CustomRuleTags))
	for k := range cfg.CustomRuleTags {
		catKeys = append(catKeys, k)
	}
	sort.Strings(catKeys)
	for _, cat := range catKeys {
		for _, opt := range cfg.CustomRuleTags[cat] {
			resp.CustomTagOptions = append(resp.CustomTagOptions, &ypb.SyntaxFlowScanPolicyTagOption{
				Name:        opt.Name,
				DisplayName: opt.DisplayName,
				Category:    cat,
			})
		}
	}
	return resp, nil
}

func (s *Server) SyntaxFlowScanPolicyToRuleFilter(ctx context.Context, req *ypb.SyntaxFlowScanPolicyToRuleFilterRequest) (*ypb.SyntaxFlowScanPolicyToRuleFilterResponse, error) {
	policyID := strings.TrimSpace(req.GetPolicyId())
	if policyID == "" {
		return nil, utils.Error("policy id is required")
	}
	policy := &ssaconfig.ScanPolicyConfig{PolicyType: policyID}
	if policyID == ssaconfig.PolicyTypeCustom {
		policy.CustomRules = &ssaconfig.CustomRulesConfig{
			Tags:     append([]string{}, req.GetTags()...),
			Severity: append([]string{}, req.GetSeverity()...),
			Purpose:  append([]string{}, req.GetPurpose()...),
		}
	}
	filter := policy.MapToRuleFilter()
	if len(req.GetLanguage()) > 0 {
		filter.Language = append([]string{}, req.GetLanguage()...)
	}
	if len(req.GetGroupNames()) > 0 {
		filter.GroupNames = append([]string{}, req.GetGroupNames()...)
	}
	return &ypb.SyntaxFlowScanPolicyToRuleFilterResponse{Filter: filter}, nil
}
