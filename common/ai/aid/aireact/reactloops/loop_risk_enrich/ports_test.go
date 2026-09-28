package loop_risk_enrich

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestSearchPortsFindsOldFingerprintInsideFixedBoundary(t *testing.T) {
	db := riskTestDB(t)
	r := &schema.Risk{Hash: "port-search-risk", Host: "192.0.2.1", RuntimeId: "port-tool"}
	require.NoError(t, db.Create(r).Error)
	hit := &schema.Port{Host: "192.0.2.1", Port: 22, RuntimeId: "port-tool", ServiceType: "ssh", Fingerprint: "OpenSSH_9.0"}
	require.NoError(t, db.Create(hit).Error)
	for i := 0; i < 350; i++ {
		p := &schema.Port{Host: "192.0.2.1", Port: 10000 + i, RuntimeId: "port-tool", ServiceType: "http", Fingerprint: "web-" + strconv.Itoa(i)}
		require.NoError(t, db.Create(p).Error)
	}
	outside := &schema.Port{Host: "192.0.2.1", Port: 22, RuntimeId: "another-tool", ServiceType: "ssh", Fingerprint: "OpenSSH_9.0"}
	require.NoError(t, db.Create(outside).Error)
	env := &riskEnvironment{RiskID: int64(r.ID), RuntimeIDs: []string{r.RuntimeId}, Scope: scopeForRisk(r)}
	got, _, more, err := searchPorts(db, env, portSearchCriteria{ServiceContains: "ssh", FingerprintContains: "OpenSSH"})
	require.NoError(t, err)
	require.False(t, more)
	require.Len(t, got, 1)
	require.Equal(t, hit.ID, got[0].ID)
	_, _, _, err = searchPorts(db, nil, portSearchCriteria{Port: 22})
	require.ErrorContains(t, err, "environment is unavailable")
}
