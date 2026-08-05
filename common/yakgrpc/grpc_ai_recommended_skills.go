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
		response.Data = append(response.Data, &ypb.AIReActRecommendedSkill{
			Name:            skill.Name,
			Type:            aicommon.EnabledCapabilityTypeSkill,
			DisplayNameZhCN: skill.DisplayNameZhCN,
			Description:     skill.Description,
		})
	}

	return response, nil
}
