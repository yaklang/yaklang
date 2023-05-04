package hostsparser

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"strings"
)

type hostsBlock interface {
	Size() int
	Contains(raw string) bool
	Hosts() chan string
}

type stringBlock struct {
	ctx context.Context
	hostsBlock

	data string
}

func newStringBlock(ctx context.Context, raw string) *stringBlock {
	return &stringBlock{
		ctx:  ctx,
		data: raw,
	}
}

func (s *stringBlock) Size() int {
	return 1
}

func (s *stringBlock) Contains(raw string) bool {
	return raw == s.data
}

func (s *stringBlock) Hosts() chan string {
	c := make(chan string)
	go func() {
		defer close(c)

		select {
		case <-s.ctx.Done():
		case c <- s.data:
		}
	}()
	return c
}

func newCIDRBlock(ctx context.Context, raw string) (*ipRangeBlock, error) {
	start, netBlock, err := net.ParseCIDR(raw)
	if err != nil {
		return nil, utils.Errorf("parse cidr"+
			"[%v] failed: %s", raw, err)
	}

	if start.To4() == nil {
		return nil, errors.Errorf("ipv6 is not implemented")
	}

	low, err := utils.IPv4ToUint32(netBlock.IP)
	if err != nil {
		return nil, errors.Errorf("parse ip[%v] to int failed: %s", netBlock.IP, err)
	}

	ones, lent := netBlock.Mask.Size()
	if lent < ones {
		return nil, errors.Errorf("BUG: mask invalid: %s", raw)
	}
	var size = (1 << uint(lent-ones)) - 1
	return newIPRangeBlock(ctx, fmt.Sprintf("%v-%v", netBlock.IP, utils.InetNtoA(int64(low)+int64(size))))
}

type HostsParser struct {
	hostsBlock

	ctx    context.Context
	Blocks []hostsBlock
}

func NewHostsParser(ctx context.Context, raw string) *HostsParser {
	var blocks []hostsBlock
	for _, i := range strings.Split(raw, ",") {
		b, _ := newIPRangeBlock(ctx, i)
		if b != nil {
			blocks = append(blocks, b)
			continue
		}

		b, _ = newCIDRBlock(ctx, i)
		if b != nil {
			blocks = append(blocks, b)
			continue
		}

		s := newStringBlock(ctx, i)
		blocks = append(blocks, s)
	}

	return &HostsParser{
		ctx:    ctx,
		Blocks: blocks,
	}
}

func (h *HostsParser) Size() int {
	ret := 0
	for _, b := range h.Blocks {
		ret += b.Size()
	}
	return ret
}

func (h *HostsParser) Contains(raw string) bool {
	for _, b := range h.Blocks {
		if b.Contains(raw) {
			return true
		}
	}
	return false
}

func (h *HostsParser) Hosts() chan string {
	c := make(chan string)
	go func() {
		defer close(c)

	GEN:
		for _, b := range h.Blocks {
			outC := b.Hosts()
		GEN2:
			for {
				select {
				case <-h.ctx.Done():
					break GEN
				default:
				}

				select {
				case data, ok := <-outC:
					if !ok {
						break GEN2
					}
					c <- data
				}

			}
		}
	}()
	return c
}
