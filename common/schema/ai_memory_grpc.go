package schema

import (
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (a *AIMemoryEntity) ToGRPC() *ypb.AIMemoryEntity {
	if a == nil {
		return nil
	}

	return &ypb.AIMemoryEntity{
		Id:                 int64(a.ID),
		CreatedAt:          a.CreatedAt.Unix(),
		UpdatedAt:          a.UpdatedAt.Unix(),
		MemoryID:           a.MemoryID,
		SessionID:          a.SessionID,
		Content:            a.Content,
		Tags:               []string(a.Tags),
		PotentialQuestions: []string(a.PotentialQuestions),
		CScore:             a.C_Score,
		OScore:             a.O_Score,
		RScore:             a.R_Score,
		EScore:             a.E_Score,
		PScore:             a.P_Score,
		AScore:             a.A_Score,
		TScore:             a.T_Score,
		CorePactVector:     []float32(a.CorePactVector),
	}
}

func GRPC2AIMemoryEntity(m *ypb.AIMemoryEntity) *AIMemoryEntity {
	if m == nil {
		return nil
	}

	entity := &AIMemoryEntity{
		MemoryID:           m.GetMemoryID(),
		SessionID:          m.GetSessionID(),
		Content:            m.GetContent(),
		Tags:               StringArray(m.GetTags()),
		PotentialQuestions: StringArray(m.GetPotentialQuestions()),
		C_Score:            m.GetCScore(),
		O_Score:            m.GetOScore(),
		R_Score:            m.GetRScore(),
		E_Score:            m.GetEScore(),
		P_Score:            m.GetPScore(),
		A_Score:            m.GetAScore(),
		T_Score:            m.GetTScore(),
		CorePactVector:     FloatArray(m.GetCorePactVector()),
	}

	// Best-effort: allow caller to set timestamps for import cases.
	if m.GetCreatedAt() > 0 {
		entity.CreatedAt = time.Unix(m.GetCreatedAt(), 0)
	}
	if m.GetUpdatedAt() > 0 {
		entity.UpdatedAt = time.Unix(m.GetUpdatedAt(), 0)
	}

	if m.GetId() > 0 {
		entity.ID = uint(m.GetId())
	}

	return entity
}
