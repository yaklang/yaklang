package aihttp_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/aihttp"
)

func TestGatewayCreation(t *testing.T) {
	gw := newTestGateway(t)
	require.NotNil(t, gw)
	require.Equal(t, "/agent", gw.GetRoutePrefix())
	require.NotEmpty(t, gw.GetAddr())
}

func TestJWTEnabled(t *testing.T) {
	gw := newTestGateway(t, aihttp.WithJWTAuth("test-secret-123"))
	require.True(t, gw.IsJWTEnabled())
	require.Equal(t, "test-secret-123", gw.GetJWTSecret())
}

func TestTOTPEnabled(t *testing.T) {
	gw := newTestGateway(t, aihttp.WithTOTP("JBSWY3DPEHPK3PXP"))
	require.True(t, gw.IsTOTPEnabled())
	require.Equal(t, "JBSWY3DPEHPK3PXP", gw.GetTOTPSecret())
}
