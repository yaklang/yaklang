package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// GetAIReActRecommendedSkills returns the fixed set of built-in skills that
// the product recommends for direct selection when starting a ReAct session.
func (s *Server) GetAIReActRecommendedSkills(
	_ context.Context,
	_ *ypb.Empty,
) (*ypb.GetAIReActRecommendedSkillsResponse, error) {
	skills, err := aireact.GetRecommendedBuiltinSkills()
	if err != nil {
		return nil, err
	}

	response := &ypb.GetAIReActRecommendedSkillsResponse{
		Data: make([]*ypb.AIReActRecommendedSkill, 0, len(skills)),
	}
	for _, skill := range skills {
		response.Data = append(response.Data, toYPBRecommendedSkill(skill))
	}

	return response, nil
}

// UpdateAIReActRecommendedSkill saves the user-editable Markdown body while
// preserving the embedded Skill metadata.
func (s *Server) UpdateAIReActRecommendedSkill(
	_ context.Context,
	req *ypb.UpdateAIReActRecommendedSkillRequest,
) (*ypb.AIReActRecommendedSkill, error) {
	skill, err := aireact.UpdateRecommendedBuiltinSkill(req.GetName(), req.GetContent())
	if err != nil {
		return nil, err
	}
	return toYPBRecommendedSkill(skill), nil
}

// ResetAIReActRecommendedSkill restores the current release's embedded Skill.
func (s *Server) ResetAIReActRecommendedSkill(
	_ context.Context,
	req *ypb.ResetAIReActRecommendedSkillRequest,
) (*ypb.AIReActRecommendedSkill, error) {
	skill, err := aireact.ResetRecommendedBuiltinSkill(req.GetName())
	if err != nil {
		return nil, err
	}
	return toYPBRecommendedSkill(skill), nil
}

func toYPBRecommendedSkill(skill aireact.RecommendedSkill) *ypb.AIReActRecommendedSkill {
	return &ypb.AIReActRecommendedSkill{
		Name:            skill.Name,
		Type:            aicommon.EnabledCapabilityTypeSkill,
		DisplayNameZhCN: skill.DisplayNameZhCN,
		Description:     skill.Description,
		Content:         skill.Content,
		IsModified:      skill.IsModified,
	}
}
