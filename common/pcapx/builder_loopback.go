package pcapx

import (
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/utils"
)

var loopbackLayerExports = map[string]interface{}{
	"loopback_payload": WithLoopback_Payload,
	"loopback_family":  WithLoopback_Family,
}

func init() {
	for k, v := range loopbackLayerExports {
		Exports[k] = v
	}
}

type LoopbackOption func(config *layers.Loopback) error

func WithLoopback_Payload(payload []byte) LoopbackOption {
	return func(config *layers.Loopback) error {
		config.Payload = payload
		return nil
	}
}

func WithLoopback_Family(i any) LoopbackOption {
	return func(config *layers.Loopback) error {
		switch ret := i.(type) {
		case layers.ProtocolFamily:
			config.Family = ret
			return nil
		default:
			return utils.Errorf("invalid link type: %v", i)
		}
	}
}
