package scannode

import "fmt"

type aiFocusRiskJudgementProgressSource interface {
	riskJudgementProgress() (map[string]any, error)
}

// riskJudgementProgressV1 exposes native receipt progress, never a scope or
// completion count proposed by the model. The same immutable result contract
// and active Focus Turn that authorize submission also authorize this read.
func (r *legionServerFocusRuntime) riskJudgementProgressV1(params map[string]any) (map[string]any, error) {
	if len(params) != 0 {
		return nil, fmt.Errorf("risk_judgement.progress.v1 accepts no parameters")
	}
	if _, err := r.activeRiskJudgementResultContract(serverFocusCapabilityRiskJudgementProgressV1); err != nil {
		return nil, err
	}
	sink, ok := r.sink.(aiFocusRiskJudgementProgressSource)
	if !ok {
		return nil, fmt.Errorf("server focus result sink does not provide native risk judgement progress")
	}
	return sink.riskJudgementProgress()
}

func (p *aiSessionResultSinkProxy) riskJudgementProgress() (map[string]any, error) {
	if p == nil {
		return nil, fmt.Errorf("ai session result sink is unavailable")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	sink, ok := p.sink.(aiFocusRiskJudgementProgressSource)
	if !ok {
		return nil, fmt.Errorf("ai session result sink does not provide native risk judgement progress")
	}
	return sink.riskJudgementProgress()
}

func (s *legionAIFocusResultSink) riskJudgementProgress() (map[string]any, error) {
	if s == nil {
		return nil, fmt.Errorf("ai risk judgement result sink is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.riskJudgementScope == nil || s.riskJudgementKind == "" {
		return nil, fmt.Errorf("ai risk judgement progress requires a bound scope and result kind")
	}
	accepted := make([]string, 0, len(s.judgedRiskIDs))
	remaining := make([]string, 0, len(s.riskJudgementScope.AllowedRiskIDs))
	for _, riskID := range s.riskJudgementScope.AllowedRiskIDs {
		if _, ok := s.judgedRiskIDs[riskID]; ok {
			accepted = append(accepted, riskID)
		} else {
			remaining = append(remaining, riskID)
		}
	}
	return map[string]any{
		"required_result_count": int(s.riskJudgementScope.RequiredResultCount),
		"accepted_risk_ids":     accepted,
		"remaining_risk_ids":    remaining,
	}, nil
}
