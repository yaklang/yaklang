package cybertunnel

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx/dns_lookup"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
)

func TestDNSLogServer(t *testing.T) {
	token1, token2, token3 := strings.ToLower(utils.RandStringBytes(5)), strings.ToLower(utils.RandStringBytes(5)), strings.ToLower(utils.RandStringBytes(5))
	randInt := utils.GetRandomAvailableTCPPort()
	client, err := NewDNSLogServerWithListeningPort(strings.Join([]string{
		token1 + ".org",
		token2 + ".xyz",
		token3 + ".eu.org",
	}, ","), "127.0.0.1", randInt)
	if err != nil {
		t.Fatal(err)
	}
	var (
		check1 = false
		check2 = false
		check3 = false
	)
	for i := 0; i < 100; i++ {
		rsp, err := client.RequireDomain(context.Background(), &tpb.RequireDomainParams{
			Mode: "",
		})
		if err != nil {
			t.Fatal(err)
		}

		log.Infof("assign domain: %v", rsp.GetDomain())

		if !check1 {
			if strings.Contains(rsp.GetDomain(), token1) {
				dns_lookup.LookupFirst(rsp.GetDomain(), dns_lookup.WithDNSServers("127.0.0.1:"+fmt.Sprint(randInt)))
				dnslogResult, err := client.QueryExistedDNSLog(context.Background(), &tpb.QueryExistedDNSLogParams{
					Token: rsp.GetToken(),
					Mode:  "",
				})
				if err != nil {
					t.Fatal(err)
				}
				assert.GreaterOrEqual(t, len(dnslogResult.Events), 1)
				check1 = true
			}
		}

		if !check2 {
			if strings.Contains(rsp.GetDomain(), token2) {
				dns_lookup.LookupFirst(rsp.GetDomain(), dns_lookup.WithDNSServers("127.0.0.1:"+fmt.Sprint(randInt)))
				dnslogResult, err := client.QueryExistedDNSLog(context.Background(), &tpb.QueryExistedDNSLogParams{
					Token: rsp.GetToken(),
				})
				if err != nil {
					t.Fatal(err)
				}
				assert.GreaterOrEqual(t, len(dnslogResult.Events), 1)
				check2 = true
			}
		}

		if !check3 {
			if strings.Contains(rsp.GetDomain(), token3) {
				dns_lookup.LookupFirst(rsp.GetDomain(), dns_lookup.WithDNSServers("127.0.0.1:"+fmt.Sprint(randInt)))
				dnslogResult, err := client.QueryExistedDNSLog(context.Background(), &tpb.QueryExistedDNSLogParams{
					Token: rsp.GetToken(),
				})
				if err != nil {
					t.Fatal(err)
				}
				assert.GreaterOrEqual(t, len(dnslogResult.Events), 1)
				check3 = true
			}
		}

		if check1 && check2 && check3 {
			break
		}
	}

	if !check1 || !check2 || !check3 {
		t.Fatal("not all dnslogs are checked")
	}
}
