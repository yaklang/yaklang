package netstackvm

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/icmp"
	"testing"
)

func TestNewSystemNetStackVMManager(t *testing.T) {
	m, err := NewSystemNetStackVMManager()
	if err != nil {
		t.Errorf("NewSystemNetStackVMManager() failed: %v", err)
	}
	//target := "127.0.0.1,192.168.3.1/24,183.2.172.185/24"
	target := "183.2.172.185/24"
	res, err := icmp.NewClient(m.GetStack()).PingScan(context.Background(), target)
	require.NoError(t, err)
	count := 0
	for r := range res {
		if r == nil {
			continue
		}
		//fmt.Printf("[%s]: icmp type: %d, code: %d, id: %d\n", r.Address, r.MessageType, r.MessageCode, r.MessageID)
		count++
	}
	fmt.Printf("total alive: %d\n", count)
}
