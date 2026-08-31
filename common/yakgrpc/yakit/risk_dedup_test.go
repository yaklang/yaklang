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

func TestComputeRiskHash_NoNormalization(t *testing.T) {
	// ComputeRiskHash does NOT normalize. Different surface forms of the
	// same logical target produce different hashes. It is the caller's
	// responsibility (e.g. cybersecurity-risk.yak) to normalize inputs
	// before passing them in.
	h1 := ComputeRiskHash("https://example.com/login", "", 0, "sqli", "username")
	h2 := ComputeRiskHash("https://example.com:443/login/", "", 0, "sqli", "username")
	require.NotEqual(t, h1, h2, "un-normalized targets should produce different hashes")

	// But the same string always produces the same hash.
	h3 := ComputeRiskHash("https://example.com:443/login/", "", 0, "sqli", "username")
	require.Equal(t, h2, h3)
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

	// url takes priority over host:port
	h3 := ComputeRiskHash("https://example.com/", "example.com", 443, "info", "")
	h4 := ComputeRiskHash("https://example.com/", "", 0, "info", "")
	require.Equal(t, h3, h4, "url should take priority over host:port")
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
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
	}
	err = r2.BeforeSave()
	require.NoError(t, err)
	require.Equal(t, r.Hash, r2.Hash, "same fields should produce same hash in BeforeSave")
}

func TestSchemaRisk_BeforeSave_NoNormalization(t *testing.T) {
	// BeforeSave does NOT normalize. Different surface forms produce
	// different hashes.
	r1 := &schema.Risk{
		Url:       "https://example.com/login",
		RiskType:  "sqli",
		Parameter: "username",
	}
	r1.BeforeSave()

	r2 := &schema.Risk{
		Url:       "https://example.com:443/login/",
		RiskType:  "sqli",
		Parameter: "username",
	}
	r2.BeforeSave()

	require.NotEqual(t, r1.Hash, r2.Hash, "un-normalized targets should produce different hashes")
}