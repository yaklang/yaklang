package pcaputil

import (
	"context"
	"github.com/gopacket/gopacket"
	"github.com/yaklang/yaklang/common/utils/omap"
	"sync"
)

type pcapPacketHandler func(ctx context.Context, packet gopacket.Packet) error

type pcapPacketHandlerContext struct {
	ctx     context.Context
	handler pcapPacketHandler
}

type daemonCache struct {
	handler            PcapHandleOperation
	registeredHandlers *omap.OrderedMap[string, *pcapPacketHandlerContext]
	startOnce          *sync.Once
}
