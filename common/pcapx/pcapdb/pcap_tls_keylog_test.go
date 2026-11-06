package pcapdb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestPCAPDBTLSKeyLogReindexesExistingCapture(t *testing.T) {
	fixture := filepath.Join("..", "..", "..", "testdata", "enterprise-traffic-analysis", "https-keylog-dridex")
	capture, err := trafficfixture.Materialize(filepath.Join(fixture, "capture.pcap"), t.TempDir())
	require.NoError(t, err)
	keyLogPath, err := trafficfixture.Materialize(filepath.Join(fixture, "capture.keys"), t.TempDir())
	require.NoError(t, err)
	keyLog, err := os.ReadFile(keyLogPath)
	require.NoError(t, err)

	m := testManager(t)
	encrypted, err := m.GetOrCreate(capture, WithProtocols(true))
	require.NoError(t, err)
	encryptedMeta, err := encrypted.Metadata()
	require.NoError(t, err)
	require.Empty(t, encryptedMeta.TLSKeyLogSHA256)
	encryptedHTTP, err := encrypted.QueryProtocols(QueryProtocol("http"), QueryLimit(100))
	require.NoError(t, err)

	decrypted, err := m.GetOrCreate(capture, WithProtocols(true), WithTLSKeyLog(string(keyLog)))
	require.NoError(t, err)
	decryptedMeta, err := decrypted.Metadata()
	require.NoError(t, err)
	require.Equal(t, encrypted.ID, decrypted.ID, "adding a keylog must rebuild the content-addressed dataset in place")
	require.NotEmpty(t, decryptedMeta.TLSKeyLogSHA256)
	httpMessages, err := decrypted.QueryProtocols(QueryProtocol("http"), QueryLimit(200))
	require.NoError(t, err)
	require.Greater(t, len(httpMessages), len(encryptedHTTP), "keylog replay must add decrypted HTTP messages")

	var foundDownload, foundPost bool
	for _, message := range httpMessages {
		foundDownload = foundDownload || strings.Contains(message.Summary, "GET /invest_20.dll")
		foundPost = foundPost || strings.Contains(message.Summary, "POST /docs.php")
	}
	require.True(t, foundDownload)
	require.True(t, foundPost)

	reopened, err := m.GetOrCreate(capture, WithProtocols(true))
	require.NoError(t, err)
	reopenedMeta, err := reopened.Metadata()
	require.NoError(t, err)
	require.Equal(t, decryptedMeta.TLSKeyLogSHA256, reopenedMeta.TLSKeyLogSHA256)
}

func TestPCAPDBTLSKeyLogRequiresValidatedProtocolReplay(t *testing.T) {
	m := testManager(t)
	capture := writeCapture(t, classicCapture(t, 1))

	_, err := m.GetOrCreate(capture, WithTLSKeyLog("invalid"))
	require.ErrorContains(t, err, "invalid TLS key log")

	_, err = m.GetOrCreate(capture, WithTLSKeyLog("CLIENT_RANDOM 00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff 00112233445566778899aabbccddeeff"))
	require.ErrorContains(t, err, "requires withProtocols(true)")
}
