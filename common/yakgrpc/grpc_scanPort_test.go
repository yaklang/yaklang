package yakgrpc

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_PortScan(t *testing.T) {
	client, err := NewLocalClient()
	require.Nil(t, err)

	host, port := utils.DebugMockHTTP([]byte{})

	r, err := client.PortScan(context.Background(), &ypb.PortScanRequest{
		Targets:     host,
		Ports:       strconv.Itoa(port),
		Mode:        "fp",
		Proto:       []string{"tcp"},
		Concurrent:  50,
		Active:      false,
		ScriptNames: []string{},
	})
	_ = r
	require.Nil(t, err)
	for {
		result, err := r.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				require.Nilf(t, err, "stream error: %v", err)
			}
			break
		}
		spew.Dump(result)
	}
}
func TestServer_CustomFingerprint(t *testing.T) {
	client, err := NewLocalClient()
	require.Nil(t, err)

	host, port := utils.DebugMockHTTP([]byte("test CustomFingerprint"))

	f, err := os.CreateTemp(os.TempDir(), "yakit-test-fingerprint-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`- methods:
    - keywords:
        - product: 测试
          regexp: "test CustomFingerprint"`)
	f.Close()

	r, err := client.PortScan(context.Background(), &ypb.PortScanRequest{
		UserFingerprintFiles: []string{f.Name()},
		Targets:              host,
		Ports:                strconv.Itoa(port),
		Mode:                 "fingerprint",
		Proto:                []string{"tcp"},
		Concurrent:           50,
		Active:               false,
		ScriptNames:          []string{},
	})
	_ = r
	require.Nil(t, err)
	ok := false
	for {
		result, err := r.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				require.Nilf(t, err, "stream error: %v", err)
			}
			break
		}
		if strings.Contains(string(result.Message), "http/测试") {
			ok = true
		}
		spew.Dump(result)
	}
	if !ok {
		t.FailNow()
	}
}

//func TestServer_PortScanUDP(t *testing.T) {
//	client, err := NewLocalClient()
//	if err != nil {
//		panic(err)
//	}
//
//	r, err := client.PortScan(context.Background(), &ypb.PortScanRequest{
//		Targets:    "cybertunnel.run",
//		Ports:      "53",
//		Mode:       "fp",
//		Proto:      []string{"udp"},
//		Concurrent: 50,
//		Active:     true,
//	})
//	_ = r
//	if err != nil {
//		panic(err)
//	}
//	for {
//		result, err := r.Recv()
//		if err != nil {
//			break
//		}
//		spew.Dump(result)
//	}
//}
