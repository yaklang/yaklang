package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAttachedRiskResource(t *testing.T) {
	for _, raw := range []string{`123`, `"123"`, `{"id":123}`, `{"id":"123"}`} {
		resource, err := ParseAttachedResourceData(NewAttachedResource(AttachedResourceTypeRiskID, AttachedResourceKeyID, raw))
		require.NoError(t, err)
		risk, ok := resource.(*AttachedRiskResourceData)
		require.True(t, ok)
		require.Equal(t, int64(123), risk.ID)
		require.Contains(t, risk.ToAttachData(nil), "123")
	}
	for _, raw := range []string{`0`, `-1`, `123.4`, `[]`, `{"ids":[1,2]}`, `{"id":"abc"}`} {
		_, err := ParseAttachedResourceData(NewAttachedResource("risk", "id", raw))
		require.Error(t, err, raw)
	}
}
