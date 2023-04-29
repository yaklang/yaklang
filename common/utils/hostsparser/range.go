package hostsparser

import (
	"context"
	"github.com/pkg/errors"
	"net"
	"yaklang/common/utils"
	"strconv"
	"strings"
)

type ipRangeBlock struct {
	ctx context.Context
	hostsBlock

	origin            string
	size              int
	containerHandler  func(ip net.IP) bool
	chanHandlerGetter func() chan string
}

func newIPRangeBlock(ctx context.Context, raw string) (*ipRangeBlock, error) {
	if !strings.Contains(raw, "-") {
		return nil, utils.Errorf("ip range should contains '-'")
	}

	rets := strings.Split(raw, "-")
	if len(rets) != 2 {
		return nil, errors.Errorf("range is invalid: %s", raw)
	}

	first, second := rets[0], rets[1]
	ip1 := net.ParseIP(utils.FixForParseIP(first))
	if ip1 == nil {
		return nil, utils.Errorf("first ip block is error: %s", first)
	}

	createFromIPRange := func(i1, i2 net.IP) (*ipRangeBlock, error) {
		end := utils.InetAtoN(i2)
		start := utils.InetAtoN(i1)
		if end > start {
			return &ipRangeBlock{
				ctx:    ctx,
				origin: raw,
				size:   int(end - start + 1),
				containerHandler: func(ip net.IP) bool {
					r := utils.InetAtoN(ip)
					return r >= start && r <= end
				},
				chanHandlerGetter: func() chan string {
					c := make(chan string)
					go func() {
						defer close(c)

					GEN:
						for i := start; true; i++ {
							if i > end {
								break
							}

							select {
							case <-ctx.Done():
								break GEN
							default:
							}

							select {
							case c <- utils.InetNtoA(i).String():
							}
						}
					}()
					return c

				},
			}, nil
		} else {
			return nil, errors.Errorf("second[%v - %v] block should be larger than first[%v - %v]", i2, end, i1, start)
		}
	}

	ip2 := net.ParseIP(utils.FixForParseIP(second))
	if ip2 != nil {
		return createFromIPRange(ip1, ip2)
	}

	secondEnd, err := strconv.ParseInt(second, 10, 64)
	if err != nil {
		return nil, errors.Errorf("second block is not a int: %v", second)
	}

	if secondEnd >= 255 {
		secondEnd = 255
	}

	var rawIp1 []byte = ip1.To4()
	ip2 = net.IP{rawIp1[0], rawIp1[1], rawIp1[2], byte(secondEnd)}
	return createFromIPRange(ip1, ip2)
}

func (p *ipRangeBlock) Size() int {
	return p.size
}

func (p *ipRangeBlock) Contains(raw string) bool {
	if i := net.ParseIP(utils.FixForParseIP(raw)); i != nil {
		return p.containerHandler(i)
	}
	return p.origin == raw
}

func (p *ipRangeBlock) Hosts() chan string {
	return p.chanHandlerGetter()
}
