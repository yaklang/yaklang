package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestUpdateRiskHTTPFlowEvidence(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.Risk{}).Error)

	risk := &schema.Risk{Hash: "risk-update-test"}
	require.NoError(t, db.Create(risk).Error)
	request, response := `"GET / HTTP/1.1"`, `"HTTP/1.1 200 OK"`
	require.NoError(t, UpdateRiskHTTPFlowEvidence(db, int64(risk.ID), &RiskHTTPFlowEvidenceUpdate{
		PacketPairs:    schema.PacketPairList{{HTTPFlowId: 42, Url: "https://example.test/"}},
		QuotedRequest:  &request,
		QuotedResponse: &response,
	}))

	updated, err := GetRisk(db, int64(risk.ID))
	require.NoError(t, err)
	require.Len(t, updated.PacketPairs, 1)
	require.Equal(t, int64(42), updated.PacketPairs[0].HTTPFlowId)
	require.Equal(t, request, updated.QuotedRequest)
	require.Equal(t, response, updated.QuotedResponse)

}
