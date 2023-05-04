package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestServer_PortScan(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	r, err := client.PortScan(context.Background(), &ypb.PortScanRequest{
		Targets:     "47.52.100.1/24",
		Ports:       "22",
		Mode:        "fp",
		Proto:       []string{"tcp"},
		Concurrent:  50,
		Active:      false,
		ScriptNames: []string{"OpenSSH CVE 合规检查：2010-2021"},
	})
	_ = r
	if err != nil {
		panic(err)
	}
	for {
		result, err := r.Recv()
		if err != nil {
			break
		}
		spew.Dump(result)
	}
}

func TestServer_PortScanUDP(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	r, err := client.PortScan(context.Background(), &ypb.PortScanRequest{
		Targets:    "cybertunnel.run",
		Ports:      "53",
		Mode:       "fp",
		Proto:      []string{"udp"},
		Concurrent: 50,
		Active:     true,
	})
	_ = r
	if err != nil {
		panic(err)
	}
	for {
		result, err := r.Recv()
		if err != nil {
			break
		}
		spew.Dump(result)
	}
}
