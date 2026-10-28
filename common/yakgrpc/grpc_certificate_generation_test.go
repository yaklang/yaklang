package yakgrpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_CertificateGenerationEMudhra(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	_, err = crep.NewMITMServer()
	require.NoError(t, err)
	defaultCA, err := os.ReadFile(crep.GetDefaultCaFilePath())
	require.NoError(t, err)
	code := coreplugin.GetCorePluginData("证书生成")
	require.NotEmpty(t, code)
	before := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	after := before.AddDate(1, 0, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	parameters, err := client.YaklangInspectInformation(ctx, &ypb.YaklangInspectInformationRequest{
		YakScriptType: "yak", YakScriptCode: string(code),
	})
	require.NoError(t, err)
	found := false
	for _, parameter := range parameters.GetCliParameter() {
		if parameter.GetField() == "type_info" {
			require.Contains(t, parameter.GetJsonSchema(), "EMudhraServer")
			found = true
		}
	}
	require.True(t, found, "existing clients must receive the new certificate type through CLI metadata")
	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code: string(code), PluginType: "yak",
		ExecParams: []*ypb.KVPair{
			{Key: "type_info", Value: `{"kind":"EMudhraServer"}`},
			{Key: "commonName", Value: "example.test"},
			{Key: "org", Value: "TLS Test"},
			{Key: "dns", Value: " example.test , "},
			{Key: "ip", Value: " 127.0.0.1, [::1] , "},
			{Key: "notBefore", Value: before.Format(time.RFC3339)},
			{Key: "notAfter", Value: after.Format(time.RFC3339)},
		},
	})
	require.NoError(t, err)
	paths := make(map[string]string)
	for {
		result, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if !result.IsMessage {
			continue
		}
		var message struct {
			Type    string          `json:"type"`
			Content json.RawMessage `json:"content"`
		}
		require.NoError(t, json.Unmarshal(result.Message, &message))
		var card yaklib.YakitStatusCard
		if message.Type == "status-card" {
			require.NoError(t, json.Unmarshal(message.Content, &card))
		} else if message.Type == "log" {
			var log yaklib.YakitLog
			require.NoError(t, json.Unmarshal(message.Content, &log))
			if log.Level != "feature-status-card-data" {
				continue
			}
			require.NoError(t, json.Unmarshal([]byte(log.Data), &card))
		}
		if card.Id == "CA 文件地址" || card.Id == "私钥 文件地址" {
			paths[card.Id] = card.Data
			t.Cleanup(func() { _ = os.Remove(card.Data) })
		}
	}
	require.Len(t, paths, 2, "the plugin must produce both downloadable files")
	chain, err := os.ReadFile(paths["CA 文件地址"])
	require.NoError(t, err)
	key, err := os.ReadFile(paths["私钥 文件地址"])
	require.NoError(t, err)
	pair, err := tls.X509KeyPair(chain, key)
	require.NoError(t, err)
	require.Len(t, pair.Certificate, 3)
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	require.NoError(t, err)
	require.Equal(t, before, leaf.NotBefore)
	require.Equal(t, after, leaf.NotAfter)
	require.Equal(t, "EM OV TLS CA - G3C", leaf.Issuer.CommonName)
	for _, host := range []string{"example.test", "127.0.0.1", "::1"} {
		require.NoError(t, leaf.VerifyHostname(host))
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(paths["私钥 文件地址"])
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
	currentCA, err := os.ReadFile(crep.GetDefaultCaFilePath())
	require.NoError(t, err)
	require.Equal(t, defaultCA, currentCA)
}

func TestGRPCMUSTPASS_CertificateGenerationRejectsInvalidSAN(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	code := coreplugin.GetCorePluginData("证书生成")
	for _, tc := range []struct{ name, dns, ip, want string }{
		{"invalid-ip", "", "127.0.0.999", "无效的 IP 地址"},
		{"invalid-ip-with-dns", "example.test", "127.0.0.1, invalid", "无效的 IP 地址"},
		{"empty-lists", " , ", " , ", "生成服务器证书需要填写"},
		{"missing-san", "", "", "生成服务器证书需要填写"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
				Code: string(code), PluginType: "yak",
				ExecParams: []*ypb.KVPair{
					{Key: "type_info", Value: `{"kind":"EMudhraServer"}`},
					{Key: "dns", Value: tc.dns}, {Key: "ip", Value: tc.ip},
				},
			})
			require.NoError(t, err)
			output := ""
			for {
				result, err := stream.Recv()
				if err != nil {
					if err != io.EOF {
						output += err.Error()
					}
					break
				}
				output += string(result.Message)
			}
			require.Contains(t, output, tc.want)
			require.NotContains(t, output, "CA 文件地址", "invalid input must not produce certificate files")
			require.NotContains(t, output, "私钥 文件地址", "invalid input must not produce key files")
		})
	}
}
