package base

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConsumedResultSnapshotMatchesPublicLookups(t *testing.T) {
	for _, extensions := range []int{0, 1, 32} {
		t.Run(fmt.Sprint(extensions), func(t *testing.T) {
			cfg := NewEmptyConfig()
			for i := 0; i < extensions; i++ {
				cfg.SetItem(fmt.Sprint(i), i)
			}
			check := func() {
				want, present := cfg.LookupItem(CfgNodeResult)
				var override any
				var overridden bool
				if present {
					override, overridden = cfg.LookupItem("consumed bits")
				}
				value, consumed, hasResult, hasConsumed := cfg.LookupConsumedResult()
				require.Equal(t, []any{want, override, present, overridden}, []any{value, consumed, hasResult, hasConsumed})
			}
			check()
			cfg.SetItem("consumed bits", uint64(9))
			check() // no observed result: ignore an unrelated override
			cfg.SetItem(CfgNodeResult, nil)
			check() // present nil is still a result
			cfg.SetItem(CfgNodeResult, [2]uint64{3, 8})
			for _, value := range []any{uint64(0), nil, "invalid", int(-1)} {
				cfg.BaseKV.SetItem("consumed bits", value)
				check() // retain exact dynamic values for the existing conversion
			}
			cfg.GetItem(CfgOptionFuns)
			check() // public history exposure
			cfg.DeleteItem("consumed bits")
			check()
			cfg.DeleteItem(CfgNodeResult)
			check()
		})
	}
}
