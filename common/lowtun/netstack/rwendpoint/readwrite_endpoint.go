package rwendpoint

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/buffer"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
)

const defaultOutQueueLen = 1 << 10

type ReadWriteEndpoint struct {
	*channel.Endpoint

	name   string
	ctx    context.Context
	cancel context.CancelFunc
	rw     io.ReadWriteCloser
	mtu    uint32
	offset int
	once   sync.Once
	wg     sync.WaitGroup
}

func NewReadWriteCloserEndpoint(rw io.ReadWriteCloser, mtu uint32, offset int) (*ReadWriteEndpoint, error) {
	return NewReadWriteCloserEndpointContext(context.Background(), rw, mtu, offset)
}

func NewWireGuardDeviceEndpoint(device lowtun.Device) (*ReadWriteEndpoint, error) {
	offset := 4

	mtuInt, err := device.MTU()
	if err != nil {
		return nil, err
	}
	name, err := device.Name()
	if err != nil {
		return nil, err
	}

	mtu := uint32(mtuInt)
	result, err := NewReadWriteCloserEndpoint(NewWireGuardReadWriteCloserWrapper(device, mtu, offset), mtu, offset)
	if err != nil {
		return nil, err
	}
	result.name = name
	return result, nil
}

func NewReadWriteCloserEndpointContext(ctx context.Context, rw io.ReadWriteCloser, mtu uint32, offset int) (*ReadWriteEndpoint, error) {
	if mtu == 0 {
		return nil, errors.New("MTU size is zero")
	}

	if rw == nil {
		return nil, errors.New("RW interface is nil")
	}

	if offset < 0 {
		return nil, errors.New("offset must be non-negative")
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	return &ReadWriteEndpoint{
		Endpoint: channel.New(defaultOutQueueLen, mtu, ""),
		ctx:      ctx,
		cancel:   cancel,
		rw:       rw,
		mtu:      mtu,
		offset:   offset,
	}, nil
}

func (e *ReadWriteEndpoint) Close() {
	e.cancel()
	_ = e.rw.Close()
}

func (e *ReadWriteEndpoint) Attach(dispatcher stack.NetworkDispatcher) {
	e.Endpoint.Attach(dispatcher)
	e.once.Do(func() {
		log.Info("start to attach readwrite endpoint")
		if e.ctx == nil {
			e.ctx, e.cancel = context.WithCancel(context.Background())
		}
		e.wg.Add(2)
		go func() {
			defer func() {
				e.wg.Done()
				e.cancel()
			}()
			e.outboundLoop(e.ctx)
		}()
		go func() {
			defer func() {
				e.wg.Done()
				e.cancel()
			}()
			e.dispatchLoop(e.ctx)
		}()
	})
}

func (e *ReadWriteEndpoint) Wait() {
	e.wg.Wait()
}

func (e *ReadWriteEndpoint) dispatchLoop(ctx context.Context) {
	offset, mtu := e.offset, int(e.mtu)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		data := make([]byte, offset+mtu)
		n, err := e.rw.Read(data)
		if err != nil {
			return
		}

		if n == 0 || n > mtu {
			continue
		}

		if !e.IsAttached() {
			continue
		}

		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(data[offset : n+offset]),
		})
		switch header.IPVersion(data[offset:]) {
		case header.IPv4Version:
			e.InjectInbound(header.IPv4ProtocolNumber, pkt)
		case header.IPv6Version:
			e.InjectInbound(header.IPv6ProtocolNumber, pkt)
		}
		pkt.DecRef()
	}
}

// outboundLoop reads outbound packets from channel, and then it calls
// writePacket to send those packets back to lower layer.
func (e *ReadWriteEndpoint) outboundLoop(ctx context.Context) {
	for {
		pkt := e.ReadContext(ctx)
		if pkt == nil {
			break
		}
		e.writePacket(pkt)
	}
}

// writePacket writes outbound packets to the io.Writer.
func (e *ReadWriteEndpoint) writePacket(pkt *stack.PacketBuffer) tcpip.Error {
	defer pkt.DecRef()

	buf := pkt.ToBuffer()
	defer buf.Release()
	if e.offset != 0 {
		v := buffer.NewViewWithData(make([]byte, e.offset))
		_ = buf.Prepend(v)
	}

	if _, err := e.rw.Write(buf.Flatten()); err != nil {
		return &tcpip.ErrInvalidEndpointState{}
	}
	return nil
}
