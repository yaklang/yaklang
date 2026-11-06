package pcapx

import (
	"fmt"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// CaptureOptionProvider is implemented by engines that expose one composed
// capture binding instead of leaking their internal callback list.
type CaptureOptionProvider interface {
	CaptureOption() pcaputil.CaptureOption
}

// WithSuricataEngine binds a stateful Suricata engine to a capture. The engine
// owns its adapter and stats; callers do not manage a session or adapter.
//
//	pcapx.OpenPcapFile(path,
//	    pcapx.pcap_suricata(engine),
//	    pcapx.pcap_bpfFilter("tcp port 80"),
//	)
func WithSuricataEngine(engine CaptureOptionProvider) pcaputil.CaptureOption {
	return func(config *pcaputil.CaptureConfig) error {
		if engine == nil {
			return fmt.Errorf("nil suricata engine")
		}
		option := engine.CaptureOption()
		if option == nil {
			return fmt.Errorf("nil suricata capture option")
		}
		return option(config)
	}
}
