package pcaputil

import (
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// The dependency passes the entire remaining packet to each TLV decoder. An
// option can consequently borrow bytes from the next header and still succeed.
// Validate and expose options only inside this extension's declared boundary.
type ipv6OptionsHeaderError struct{ message string }

func (e *ipv6OptionsHeaderError) Error() string { return e.message }

type ipv6OptionSpan struct {
	kind       uint8
	start, end int
}

func boundedIPv6Options(data []byte, feedback gopacket.DecodeFeedback) (int, []ipv6OptionSpan, error) {
	fail := func(message string) (int, []ipv6OptionSpan, error) { return 0, nil, &ipv6OptionsHeaderError{message} }
	if len(data) < 2 {
		feedback.SetTruncated()
		return fail("IPv6 options extension requires a two-byte fixed prefix")
	}
	n := (int(data[1]) + 1) * 8
	if n > len(data) {
		feedback.SetTruncated()
		return fail(fmt.Sprintf("IPv6 options extension requires %d bytes, got %d", n, len(data)))
	}
	var options []ipv6OptionSpan
	for at := 2; at < n; {
		end := at + 1
		if data[at] != 0 { // Pad1 has no length octet, including the last header byte.
			if at+2 > n {
				return fail("IPv6 option length octet exceeds its extension header")
			}
			end = at + 2 + int(data[at+1])
			if end > n {
				return fail("IPv6 option value exceeds its extension header")
			}
		}
		options = append(options, ipv6OptionSpan{data[at], at, end})
		at = end
	}
	return n, options, nil
}

func decodeBoundedIPv6HopByHop(data []byte, feedback gopacket.DecodeFeedback) (*layers.IPv6HopByHop, error) {
	n, options, err := boundedIPv6Options(data, feedback)
	if err != nil {
		return nil, err
	}
	h := &layers.IPv6HopByHop{}
	h.NextHeader, h.HeaderLength, h.ActualLength = layers.IPProtocol(data[0]), data[1], n
	h.Contents, h.Payload = data[:n], data[n:]
	for _, span := range options {
		o := &layers.IPv6HopByHopOption{OptionType: span.kind, ActualLength: span.end - span.start}
		if span.kind != 0 {
			o.OptionLength = data[span.start+1]
			o.OptionData = data[span.start+2 : span.end]
		}
		h.Options = append(h.Options, o)
	}
	return h, nil
}

type captureIPv6OptionsDecoder struct{ hop bool }

func (d captureIPv6OptionsDecoder) Decode(data []byte, b gopacket.PacketBuilder) error {
	if d.hop {
		h, err := decodeBoundedIPv6HopByHop(data, b)
		if err != nil {
			return err
		}
		b.AddLayer(h)
		return b.NextDecoder(h.NextHeader)
	}
	n, options, err := boundedIPv6Options(data, b)
	if err != nil {
		return err
	}
	h := &layers.IPv6Destination{}
	h.NextHeader, h.HeaderLength, h.ActualLength = layers.IPProtocol(data[0]), data[1], n
	h.Contents, h.Payload = data[:n], data[n:]
	for _, span := range options {
		o := &layers.IPv6DestinationOption{OptionType: span.kind, ActualLength: span.end - span.start}
		if span.kind != 0 {
			o.OptionLength = data[span.start+1]
			o.OptionData = data[span.start+2 : span.end]
		}
		h.Options = append(h.Options, o)
	}
	b.AddLayer(h)
	return b.NextDecoder(h.NextHeader)
}
