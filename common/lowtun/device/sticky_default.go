//go:build !linux

package device

import (
	"github.com/yaklang/yaklang/common/lowtun/conn"
	"github.com/yaklang/yaklang/common/lowtun/rwcancel"
)

func (device *Device) startRouteListener(bind conn.Bind) (*rwcancel.RWCancel, error) {
	return nil, nil
}
