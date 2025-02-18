package arp

import (
	"bytes"
	"context"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/waiter"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"net/netip"
)

type Client struct {
	stack *stack.Stack
	ep    stack.LinkAddressResolverCapture
	wq    waiter.Queue
}

func NewClient(
	s *stack.Stack,
	nicID tcpip.NICID,
) (*Client, error) {
	networkEp, err := s.GetNetworkEndpoint(nicID, header.ARPProtocolNumber)
	if err != nil {
		return nil, utils.Errorf("failed to get nic by id: %v", err)
	}
	ep, ok := networkEp.(stack.LinkAddressResolverCapture)
	if !ok {
		return nil, utils.Error("network endpoint is not a link address resolver")
	}
	c := &Client{
		stack: s,
		ep:    ep,
	}
	return c, nil
}

type ArpReply struct {
	SenderMacAddress tcpip.LinkAddress
	SenderIPAddress  tcpip.Address
	TargetMacAddress tcpip.LinkAddress
	TargetIPAddress  tcpip.Address
}

// ArpRequest sends an ARP request to the given IP address. if linkAddress is empty, the ARP request will be broadcast.
func (c *Client) ArpRequest(ctx context.Context, ipAddress string, linkAddress tcpip.LinkAddress) (*ArpReply, error) {
	if c.ep == nil {
		return nil, utils.Error("arp client is not initialized")
	}

	arpEp := c.ep
	if !utils.IsIPv4(ipAddress) {
		ipAddress = netx.LookupFirst(ipAddress) // maybe lan domain
	}

	ipv4Ins, parseErr := netip.ParseAddr(ipAddress)
	if parseErr != nil {
		return nil, utils.Errorf("parse addr fail: %v", parseErr)
	}
	remoteAddr := tcpip.AddrFrom4(ipv4Ins.As4())

	wq := arpEp.GetCaptureWaitQueue()
	we, in := waiter.NewChannelEntry(waiter.EventIn)
	wq.EventRegister(&we)
	defer wq.EventUnregister(&we)

	// start capture
	arpEp.StartCapture()
	defer arpEp.StopCapture()

	// send arp request
	err := arpEp.LinkAddressRequest(remoteAddr, tcpip.Address{}, linkAddress)
	if err != nil {
		return nil, utils.Errorf("arp request fail: %v", err)
	}

	var b bytes.Buffer
	for {
		b.Reset()
		select {
		case <-ctx.Done():
			return nil, utils.Errorf("context done: %v", ctx.Err())
		case <-in:
			err := arpEp.ReadPacket(&b)
			if err != nil {
				return nil, utils.Errorf("read packet fail: %v", err)
			}
			h := header.ARP(b.Bytes())
			if !h.IsValid() || h.Op() != header.ARPReply {
				continue
			}
			if bytes.Equal(h.ProtocolAddressSender(), remoteAddr.AsSlice()) {
				return &ArpReply{
					SenderMacAddress: tcpip.LinkAddress(h.HardwareAddressSender()),
					SenderIPAddress:  tcpip.AddrFromSlice(h.ProtocolAddressSender()),
					TargetMacAddress: tcpip.LinkAddress(h.HardwareAddressTarget()),
					TargetIPAddress:  tcpip.AddrFromSlice(h.ProtocolAddressTarget()),
				}, nil
			}
		}
	}

}
