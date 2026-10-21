package pcaputil

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"sync"
)

const mavlinkSigningTimestampMax = uint64(1<<48 - 1)

// MAVLinkSigningConfig enables strict, opt-in verification for the supported
// common UDP messages. Key must contain exactly 32 bytes. InitialTimestamp is
// an independently supplied receiver clock in 10us ticks since 2015-01-01 UTC,
// not a timestamp inferred from the capture. MaxStreams defaults to 16.
//
// A stream is (capture domain, system ID, component ID, link ID), shared across
// UDP endpoint pairs within that domain. Stream history is never expired or
// evicted during this capture: forgetting it would permit replay. Each domain
// has its own receiver clock. This is the fixed c_library_v2 receiver policy:
// unseen streams may be at most 6,000,000 ticks behind that clock; existing
// streams must strictly increase. No endpoint identity, key provisioning,
// persistence across captures, decryption or physical telemetry is inferred.
// Without this option the historical opaque-signature observation is retained.
type MAVLinkSigningConfig struct {
	Key              []byte
	InitialTimestamp uint64
	MaxStreams       int
}

type mavlinkSigningOptions struct {
	key        [32]byte
	initial    uint64
	maxStreams int
}

func mavlinkSigningOptionsFor(c MAVLinkSigningConfig) (mavlinkSigningOptions, error) {
	var o mavlinkSigningOptions
	if len(c.Key) != len(o.key) || c.InitialTimestamp > mavlinkSigningTimestampMax || c.MaxStreams < 0 || c.MaxStreams > 4096 {
		return o, fmt.Errorf("MAVLink signing requires a 32-byte key, 48-bit receiver timestamp and stream limit in 1..4096")
	}
	copy(o.key[:], c.Key)
	o.initial, o.maxStreams = c.InitialTimestamp, c.MaxStreams
	if o.maxStreams == 0 {
		o.maxStreams = 16
	}
	return o, nil
}

// WithProtocolMAVLinkSigning copies the configuration when the option is made.
// Its private receiver state is fresh for each capture; unsigned and invalidly
// signed supported messages are refused. The key is never emitted in events.
func WithProtocolMAVLinkSigning(config MAVLinkSigningConfig) CaptureOption {
	o, err := mavlinkSigningOptionsFor(config)
	return func(c *CaptureConfig) error {
		if err != nil {
			return err
		}
		owned := o
		c.mavlinkSigning = &owned
		return nil
	}
}

// WithSessionMAVLinkSigning supplies the same strict receiver context to an
// explicitly selected UDP byte session. Options may be supplied in any order.
func WithSessionMAVLinkSigning(config MAVLinkSigningConfig) ProtocolSessionOption {
	o, err := mavlinkSigningOptionsFor(config)
	return func(s *captureSession) error {
		if err != nil {
			return err
		}
		s.f.a.closeMAVLinkSigning()
		s.f.a.mavlinkSigning = &mavlinkSigningState{options: o}
		return nil
	}
}

type mavlinkSigningStream struct{ system, component, link byte }
type mavlinkSigningDomain struct {
	clock   uint64
	streams map[mavlinkSigningStream]uint64
}
type mavlinkSigningState struct {
	mu       sync.Mutex
	options  mavlinkSigningOptions
	domains  map[CaptureDomain]*mavlinkSigningDomain
	count    int
	reserved int64
	closed   bool
}

// Validate the entire datagram and stage timestamps before committing them.
// A refused tail, bad MAC or budget refusal must not advance any stream/clock,
// nor expose an accepted prefix. Caller already validated framing/depth/CRC.
func (a *binParser) verifyMAVLinkDatagram(domain CaptureDomain, w []byte, count int) ([]map[string]any, error) {
	s := a.mavlinkSigning
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, protocolError(ErrContextRequired, "MAVLink signing context is closed")
	}
	d := s.domains[domain]
	clock := s.options.initial
	if d != nil {
		clock = d.clock
	}
	staged := make(map[mavlinkSigningStream]uint64, count)
	observations := make([]map[string]any, 0, count)
	newStreams := 0
	for at := 0; at < len(w); {
		if w[at] != 0xfd || w[at+2]&1 == 0 {
			return nil, protocolError(ErrAuthenticationFailed, "MAVLink signing context requires signed v2 messages")
		}
		n := 10 + int(w[at+1]) + 2 + 13
		raw := w[at : at+n]
		signature := raw[n-13:]
		h := sha256.New()
		h.Write(s.options.key[:])
		h.Write(raw[:n-6]) // magic/header, payload, CRC, link ID and 48-bit clock
		if subtle.ConstantTimeCompare(h.Sum(nil)[:6], signature[7:]) != 1 {
			return nil, protocolError(ErrAuthenticationFailed, "MAVLink signature digest mismatch")
		}
		key := mavlinkSigningStream{raw[5], raw[6], signature[0]}
		stamp := uint64(0)
		for i := 0; i < 6; i++ {
			stamp |= uint64(signature[1+i]) << (8 * i)
		}
		previous, known := staged[key]
		if !known && d != nil {
			previous, known = d.streams[key]
		}
		if known && stamp <= previous {
			return nil, protocolError(ErrAuthenticationFailed, "MAVLink signing stream timestamp did not strictly increase")
		}
		if !known {
			newStreams++
			if s.count+newStreams > min(s.options.maxStreams, a.budget.MaxCollectionElements) {
				return nil, protocolError(ErrResourceExceeded, "MAVLink signing stream limit")
			}
			// Fixed C checks new-stream capacity before the age window.
			// Subtraction avoids overflow near the 48-bit timestamp ceiling.
			if clock > stamp && clock-stamp > 6000000 {
				return nil, protocolError(ErrAuthenticationFailed, "MAVLink new signing stream timestamp is too old")
			}
		}
		observation := map[string]any{
			"Context": "caller-supplied-key-and-receiver-clock", "Scope": "capture-domain/system/component/link",
			"Link ID": int(key.link), "Timestamp": stamp, "Timestamp Units": "10us-since-2015-01-01-UTC",
			"Receiver Timestamp Before": clock, "Previous Timestamp Available": known,
		}
		if known {
			observation["Previous Timestamp"] = previous
		}
		clock = max(clock, stamp)
		observation["Receiver Timestamp After"] = clock
		observations = append(observations, observation)
		staged[key] = stamp
		at += n
	}
	// Retained context/domain/maps are charged to the shared capture budget.
	cost := int64(newStreams * 128)
	if d == nil {
		cost += int64(256 + len(domain.Encapsulation))
	}
	if cost > 0 && !a.reserveEvidence(cost) {
		return nil, protocolError(ErrResourceExceeded, "MAVLink signing retained state budget")
	}
	if d == nil {
		d = &mavlinkSigningDomain{streams: make(map[mavlinkSigningStream]uint64)}
		if s.domains == nil {
			s.domains = make(map[CaptureDomain]*mavlinkSigningDomain)
		}
		s.domains[domain] = d
	}
	for key, stamp := range staged {
		d.streams[key] = stamp
	}
	d.clock = clock
	s.count += newStreams
	s.reserved += cost
	return observations, nil
}

func (a *binParser) closeMAVLinkSigning() {
	s := a.mavlinkSigning
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	clear(s.options.key[:])
	clear(s.domains)
	s.domains = nil
	s.count = 0
	a.buffered.Add(-s.reserved)
	s.reserved = 0
	s.closed = true
}
