package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPCMUSTPASS_GLOBAL_NETWORK_DNS_CONFIG(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})

	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	if err != nil {
		panic(err)
	}
	config.CustomDNSServers = []string{"127.0.0.1"}
	spew.Dump(config)
	defer client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	if err != nil {
		panic(err)
	}
	check := false
	for _, i := range netx.NewDefaultReliableDNSConfig().SpecificDNSServers {
		if !check {
			if i == "127.0.0.1" {
				check = true
			}
		}
	}
	if !check {
		panic("set global network dns config failed")
	}
	client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	check = false
	for _, i := range netx.NewDefaultReliableDNSConfig().SpecificDNSServers {
		if !check {
			if i == "127.0.0.1" {
				check = true
			}
		}
	}
	if check {
		panic("set (reset) global network dns config failed")
	}
}
