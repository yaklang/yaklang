package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestComputeRiskHash_Deterministic(t *testing.T) {
	h1 := ComputeRiskHash("https://example.com/login", "", 0, "sqli", "username")
	h2 := ComputeRiskHash("https://example.com/login", "", 0, "sqli", "username")
	require.Equal(t, h1, h2)
	require.Len(t, h1, 32) // 16 bytes hex = 32 chars
}

func TestComputeRiskHash_TargetNormalization(t *testing.T) {
	h1 := ComputeRiskHash("https://example.com/login", "", 0, "sqli", "username")
	h2 := ComputeRiskHash("https://example.com:443/login/", "", 0, "sqli", "username")
	require.Equal(t, h1, h2, "scheme/default-port/trailing-slash should normalize to same hash")

	h3 := ComputeRiskHash("http://example.com:80/login", "", 0, "sqli", "username")
	require.Equal(t, h1, h3, "http + default port should normalize to same hash")
}

func TestComputeRiskHash_DifferentFields(t *testing.T) {
	base := ComputeRiskHash("https://example.com/login", "", 0, "sqli", "username")

	// Different type → different hash
	differentType := ComputeRiskHash("https://example.com/login", "", 0, "xss", "username")
	require.NotEqual(t, base, differentType)

	// Different parameter → different hash
	differentParam := ComputeRiskHash("https://example.com/login", "", 0, "sqli", "password")
	require.NotEqual(t, base, differentParam)

	// Different target → different hash
	differentTarget := ComputeRiskHash("https://example.com/admin", "", 0, "sqli", "username")
	require.NotEqual(t, base, differentTarget)
}

func TestComputeRiskHash_HostPortFallback(t *testing.T) {
	h1 := ComputeRiskHash("", "192.168.1.10", 22, "weak-pass", "password")
	h2 := ComputeRiskHash("", "192.168.1.10", 22, "weak-pass", "password")
	require.Equal(t, h1, h2)

	// Default port stripped
	h3 := ComputeRiskHash("", "example.com", 443, "info", "")
	h4 := ComputeRiskHash("https://example.com/", "", 0, "info", "")
	require.Equal(t, h3, h4, "host:443 should normalize same as https://host/")
}

func TestCreateRisk_DeterministicHash(t *testing.T) {
	r1 := CreateRisk("https://example.com/login",
		WithRiskParam_RiskType("sqli"),
		WithRiskParam_Parameter("username"),
		WithRiskParam_Title("SQL注入"),
	)
	r2 := CreateRisk("https://example.com/login",
		WithRiskParam_RiskType("sqli"),
		WithRiskParam_Parameter("username"),
		WithRiskParam_Title("SQL注入 (re-verify)"), // different title
	)
	require.Equal(t, r1.Hash, r2.Hash, "same target+type+param should produce same hash regardless of title")
}

func TestCreateRisk_DifferentParamDifferentHash(t *testing.T) {
	r1 := CreateRisk("https://example.com/login",
		WithRiskParam_RiskType("sqli"),
		WithRiskParam_Parameter("username"),
	)
	r2 := CreateRisk("https://example.com/login",
		WithRiskParam_RiskType("sqli"),
		WithRiskParam_Parameter("password"),
	)
	require.NotEqual(t, r1.Hash, r2.Hash, "different parameter should produce different hash")
}

func TestCreateRisk_NoRandomUUID(t *testing.T) {
	r := CreateRisk("https://example.com/login",
		WithRiskParam_RiskType("sqli"),
		WithRiskParam_Parameter("username"),
	)
	// Hash should be a 32-char hex (sha256[:16]), not a UUID
	require.Len(t, r.Hash, 32)
	require.NotContains(t, r.Hash, "-", "hash should not be a UUID with dashes")
}

func TestSchemaRisk_BeforeSave_DeterministicHash(t *testing.T) {
	r := &schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
	}
	// BeforeSave should set a deterministic hash
	err := r.BeforeSave()
	require.NoError(t, err)
	require.NotEmpty(t, r.Hash)
	require.Len(t, r.Hash, 32)

	// Same fields → same hash
	r2 := &schema.Risk{
		Url:       "https://example.com:443/login/",
		RiskType:  "sqli",
		Parameter: "username",
	}
	err = r2.BeforeSave()
	require.NoError(t, err)
	require.Equal(t, r.Hash, r2.Hash, "normalized targets should produce same hash in BeforeSave")
}