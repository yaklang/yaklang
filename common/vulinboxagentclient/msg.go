package vulinboxagentclient

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"reflect"
	"strconv"
)

func (c *Client) Msg() *MsgPrepare {
	return &MsgPrepare{
		c: c,
	}
}

type MsgPrepare struct {
	c *Client
	f func([]byte) error
}

// Send wants a pointer
func (m *MsgPrepare) Send(data any) {
	// reflect may leads to panic
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("vulinbox ws agent send panic: %v", err)
		}
	}()

	// get ActionId
	id := reflect.ValueOf(data).Elem().FieldByName("ActionId").Uint()

	if m.f != nil {
		m.c.ackWaitMap.Set(strconv.FormatUint(id, 10), m.f)
	}
	m.c.Send(data)
}

func (m *MsgPrepare) Callback(f func([]byte) error) *MsgPrepare {
	m.f = f
	return m
}

func (c *Client) Send(data any) {
	bytes := utils.Jsonify(data)
	if len(bytes) == 0 {
		log.Errorf("vulinbox ws agent send data cannot be jsonified: %v", data)
		return
	}
	select {
	case c.sendBuf <- bytes:
	default:
		// buf full or closed, drop data
		log.Errorf("vulinbox ws agent send buf full or closed, drop data: %v", data)
	}
}
