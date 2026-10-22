package yaklib

import (
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMitmEMudhraOption(t *testing.T) {
	option := MitmExports["emudhra"].(func(bool) MitmConfigOpt)
	server, err := NewMITMServer(option(true), mitmConfigUseDefault(false))
	require.NoError(t, err)
	block, rest := pem.Decode(server.GetCaCert())
	require.NotNil(t, block)
	ca, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	require.Equal(t, "EM OV TLS CA - G3C", ca.Subject.CommonName)
	require.NotEmpty(t, rest)

	_, err = NewMITMServer(option(true), mitmConfigCertAndKey([]byte("custom CA"), []byte("custom key")))
	require.ErrorContains(t, err, "cannot be used together")

	server, err = NewMITMServer(option(true), option(false))
	require.NoError(t, err)
	block, _ = pem.Decode(server.GetCaCert())
	require.NotNil(t, block)
	ca, err = x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	require.NotEqual(t, "EM OV TLS CA - G3C", ca.Subject.CommonName)
}
