package pcaputil

import (
	"strings"
	"testing"
)

func TestOPCUAStructuredValues(t *testing.T) {
	for _, field := range []string{"Read", "Write", "Call", "Browse", "BrowseNext"} {
		t.Run(field, func(t *testing.T) {
			root := "opcua-" + strings.ToLower(field) + "-structured-values"
			t.Run("Codec", func(t *testing.T) { opcuaSelectedCodecCases(t, root, field, field == "Browse") })
			t.Run("Sealed", func(t *testing.T) {
				profiles := 24
				if field == "Browse" || field == "BrowseNext" {
					profiles = 4
				}
				opcuaSelectedSealed(t, root, field, profiles, field == "Browse")
			})
		})
	}
}
