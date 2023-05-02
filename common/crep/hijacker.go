package crep

import (
	"context"
	uuid "github.com/satori/go.uuid"
	"sync"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	hijackers = new(sync.Map)
)

func GetDefaultHijacker() *Hijacker {
	return GetOrCreateHijacker("default")
}

func GetHijacker(name string) (*Hijacker, error) {
	raw, ok := hijackers.Load(name)
	if !ok {
		return nil, utils.Errorf("get hijacker failed: %s", name)
	}

	return raw.(*Hijacker), nil
}

func GetOrCreateHijacker(name string) *Hijacker {
	if data, err := GetHijacker(name); err == nil {
		return data
	}

	ctx, cancel := context.WithCancel(context.Background())
	hijackers.Store(name, &Hijacker{
		rootCtx:       ctx,
		cancel:        cancel,
		name:          name,
		isRequiring:   utils.NewBool(false),
		messages:      new(sync.Map),
		hijackedQueue: make(chan string),
	})
	return GetOrCreateHijacker(name)
}

func DeleteHijacker(name string) {
	hijackers.Delete(name)
}

type hijackerMsg struct {
	Id            string
	ctx           context.Context
	cancel        context.CancelFunc
	origin, after []byte
}

type Hijacker struct {
	rootCtx     context.Context
	cancel      context.CancelFunc
	name        string
	isRequiring *utils.AtomicBool

	messages          *sync.Map
	hijackedQueue     chan string
	currentHijackedId string
}

func (h *Hijacker) Name() string {
	return h.name
}

func (h *Hijacker) RequireHijack(rootCtx context.Context, data []byte) []byte {
	if !h.isRequiring.IsSet() {
		return data
	}

	ctx, cancel := context.WithCancel(context.Background())

	msg := &hijackerMsg{
		Id:     uuid.NewV4().String(),
		ctx:    ctx,
		cancel: cancel,
		origin: data,
	}
	h.messages.Store(msg.Id, msg)
	defer h.messages.Delete(msg.Id)

	go func() {
		h.hijackedQueue <- msg.Id
	}()

	select {
	case <-ctx.Done():
		return msg.after
	case <-rootCtx.Done():
		return msg.origin
	}
}

func (h *Hijacker) GetHijackingRequestById(id string) (*hijackerMsg, error) {
	data, ok := h.messages.Load(id)
	if ok {
		return data.(*hijackerMsg), nil
	}
	return nil, utils.Errorf("no such hijack message: %s", id)
}

func (h *Hijacker) GetCurrentHijackingRequest() (*hijackerMsg, error) {
	m, err := h.GetHijackingRequestById(h.currentHijackedId)
	if err != nil {
		h.currentHijackedId = ""
		return nil, err
	}
	return m, nil
}

func (h *Hijacker) FinishCurrentHijackingRequest(data []byte) error {
	if h.currentHijackedId == "" {
		return utils.Errorf("no hijacking request")
	}

	msg, err := h.GetCurrentHijackingRequest()
	if err != nil {
		return err
	}

	msg.after = data
	msg.cancel()
	h.messages.Delete(h.currentHijackedId)
	h.currentHijackedId = ""
	return nil
}

func (h *Hijacker) GetHijackingRequest(ctx context.Context) (req []byte, err error) {
	if h.currentHijackedId != "" {
		msg, err := h.GetCurrentHijackingRequest()
		if err != nil {
			return nil, utils.Errorf("get hijacked request by id failed: %s", h.currentHijackedId)
		}
		return msg.origin, nil
	}

	select {
	case <-ctx.Done():
		return nil, utils.Errorf("ctx done")
	case data, ok := <-h.hijackedQueue:
		if !ok {
			return nil, utils.Errorf("maybe hijacked request chan closed")
		}
		h.currentHijackedId = data
		req, err := h.GetCurrentHijackingRequest()
		if err != nil {
			return nil, err
		}
		return req.origin, nil
	}
}
