package pcaputil

import (
	"context"
	"errors"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/log"
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

type SafeHandle struct {
	Handle   *pcap.Handle
	refCount int32
	mu       sync.Mutex
}

func NewSafeHandle(handle *pcap.Handle) *SafeHandle {
	return &SafeHandle{
		Handle: handle,
	}
}

func (h *SafeHandle) AddRef() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.refCount++
	log.Warnf("AddRef refCount: %d", h.refCount)
}

func (h *SafeHandle) DecRef() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.refCount--
	log.Warnf("DecRef refCount: %d", h.refCount)
	if h.refCount == 0 {
		h.Handle.Close()
	} else if h.refCount < 0 {
		log.Errorf("refCount < 0: %d", h.refCount)
	}
}

func (h *SafeHandle) SetBPFFilter(filter string) error {
	return h.Handle.SetBPFFilter(filter)
}

func (h *SafeHandle) Close() {
	h.DecRef()
}

func (h *SafeHandle) WritePacketData(data []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.refCount > 0 {
		return h.Handle.WritePacketData(data)
	}
	return errors.New("handle closed")
}
func (h *SafeHandle) LinkType() layers.LinkType {
	return h.Handle.LinkType()
}

func (h *SafeHandle) ReadPacketData() (data []byte, ci gopacket.CaptureInfo, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.refCount > 0 {
		return h.Handle.ReadPacketData()
	}
	return nil, gopacket.CaptureInfo{}, errors.New("handle closed")
}
