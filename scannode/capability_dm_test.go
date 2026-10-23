package scannode

import (
	"slices"
	"testing"

	"github.com/yaklang/gorm"
	"github.com/yaklang/gorm/dialects/dameng"
)

func TestDMIRCapabilityRequiresNativeDriverAndHostRole(t *testing.T) {
	t.Parallel()
	// The generic dialect is always registered, including ordinary builds.
	// Registration cannot be used as evidence that the driver is linked.
	_, registered := gorm.GetDialect(dameng.Dialect)
	if !registered {
		t.Fatal("DM dialect should be registered independently of the optional driver")
	}
	for _, kind := range []string{"", "host", "ai_session", " ai_session "} {
		for _, mode := range []string{aiSessionRuntimeModeStateful, aiSessionRuntimeModeStateless} {
			for _, explicit := range []bool{false, true} {
				t.Run(kind+"/"+mode+"/"+map[bool]string{false: "compiled", true: "manifest"}[explicit], func(t *testing.T) {
					var input []string
					if explicit {
						input = []string{" ssa.ir.dm.v1 ", capabilityKeySSAIRDMV1}
					}
					actual := normalizeScanNodeCapabilityKeysForRuntimeAndKind(input, mode, kind)
					want := dameng.NativeEnabled() && kind != "ai_session" && kind != " ai_session "
					if slices.Contains(actual, capabilityKeySSAIRDMV1) != want {
						t.Fatalf("DM IR advertisement does not match native driver and role: native=%v kind=%q keys=%v", dameng.NativeEnabled(), kind, actual)
					}
					count := 0
					for _, key := range actual {
						if key == capabilityKeySSAIRDMV1 {
							count++
						}
					}
					if count > 1 {
						t.Fatalf("duplicate DM IR capability: %v", actual)
					}
				})
			}
		}
	}
}
