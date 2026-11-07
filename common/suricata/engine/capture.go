package engine

import (
	"fmt"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// CaptureOption returns the single pcaputil option that binds this engine to a
// capture. The engine owns exactly one default capture binding; create another
// engine when two captures need isolated adapter state.
func (e *StatefulEngine) CaptureOption() pcaputil.CaptureOption {
	return func(config *pcaputil.CaptureConfig) error {
		option, err := e.ensureCapture()
		if err != nil {
			return err
		}
		return option(config)
	}
}

// OpenPcapFile replays a pcap/pcapng file through the engine. Extra options
// keep pcapx semantics and are applied before the engine binding.
func (e *StatefulEngine) OpenPcapFile(filename string, options ...pcaputil.CaptureOption) error {
	option, err := e.ensureCapture()
	if err != nil {
		return err
	}
	return pcaputil.OpenPcapFile(filename, composeCaptureOptions(options, option)...)
}

// StartSniff captures from a live interface through the engine. Extra options
// keep pcapx semantics and are applied before the engine binding.
func (e *StatefulEngine) StartSniff(iface string, options ...pcaputil.CaptureOption) error {
	option, err := e.ensureCapture()
	if err != nil {
		return err
	}
	return pcaputil.Sniff(iface, composeCaptureOptions(options, option)...)
}

// CaptureStats reports adapter counters for the engine's default capture
// binding. It is also included in Stats().Capture.
func (e *StatefulEngine) CaptureStats() AdapterStats {
	if e == nil {
		return AdapterStats{}
	}
	e.captureMu.Lock()
	adapter := e.captureAdapter
	e.captureMu.Unlock()
	if adapter == nil {
		return AdapterStats{}
	}
	return adapter.Stats()
}

func (e *StatefulEngine) ensureCapture() (pcaputil.CaptureOption, error) {
	if e == nil {
		return nil, fmt.Errorf("nil stateful engine")
	}
	e.captureMu.Lock()
	defer e.captureMu.Unlock()
	if e.closed.Load() {
		return nil, fmt.Errorf("stateful engine is closed")
	}
	if e.captureOption != nil {
		return e.captureOption, nil
	}
	adapter, err := NewPcapAdapter(0, func(event InspectionEvent) {
		e.Handle(event)
	})
	if err != nil {
		return nil, err
	}
	parts := adapter.Options()
	option := func(config *pcaputil.CaptureConfig) error {
		for _, part := range parts {
			if part == nil {
				continue
			}
			if err := part(config); err != nil {
				return err
			}
		}
		return nil
	}
	e.captureAdapter = adapter
	e.captureOption = option
	return option, nil
}

func composeCaptureOptions(options []pcaputil.CaptureOption, binding pcaputil.CaptureOption) []pcaputil.CaptureOption {
	composed := make([]pcaputil.CaptureOption, 0, len(options)+1)
	composed = append(composed, options...)
	composed = append(composed, binding)
	return composed
}
