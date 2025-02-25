package mustpass

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestDisabledDomain(t *testing.T) {
	forbiddenDomain := utils.RandStringBytes(10) + ".com"
	_, err := netx.DialX(utils.RandStringBytes(5)+"."+forbiddenDomain+":8080", netx.DialX_WithDNSOptions(netx.WithDNSDisabledDomain("*."+forbiddenDomain)))
	require.Error(t, err)
	require.Contains(t, err.Error(), "disallow domain")
}
