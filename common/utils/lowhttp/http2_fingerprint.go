package lowhttp

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// HTTP2FingerprintChrome151 mirrors the HTTP/2 framing of Chrome 151, whose
// Akamai fingerprint is 1:65536;2:0;4:6291456;6:262144|15663105|0|m,a,s,p
const HTTP2FingerprintChrome151 = "chrome-151"

const (
	chromeH2HeaderTableSize     = 65536
	chromeH2InitialWindowSize   = 6291456
	chromeH2MaxHeaderListSize   = 262144
	chromeH2ConnWindowIncrement = 15663105
	// HTTP/2 encodes a stream weight of 256 as the value 255 on the wire.
	chromeH2HeadersWeight = 255
)

// http2Profile describes the HTTP/2 framing behaviour of a specific client.
//
// A profile only changes the bytes this client writes. Flow-control
// bookkeeping (initialWindowSize, connWindowControl) tracks the windows the
// peer advertises to us and is deliberately left untouched: SETTINGS and
// WINDOW_UPDATE written here advertise our receive capacity, which is a
// separate direction from the send windows those fields account for.
type http2Profile struct {
	id                string
	settings          []http2.Setting
	connWindowUpdate  uint32
	pseudoHeaderOrder []string
	headersPriority   http2.PriorityParam
	// endStreamOnHeaders carries END_STREAM on HEADERS for body-less requests
	// instead of appending an empty DATA frame. Browsers do this; some
	// non-conforming servers reject it, which is why it is opt-in only.
	endStreamOnHeaders bool
}

var http2Profiles = map[string]*http2Profile{
	HTTP2FingerprintChrome151: {
		id: HTTP2FingerprintChrome151,
		settings: []http2.Setting{
			{ID: http2.SettingHeaderTableSize, Val: chromeH2HeaderTableSize},
			{ID: http2.SettingEnablePush, Val: 0},
			{ID: http2.SettingInitialWindowSize, Val: chromeH2InitialWindowSize},
			{ID: http2.SettingMaxHeaderListSize, Val: chromeH2MaxHeaderListSize},
		},
		connWindowUpdate: chromeH2ConnWindowIncrement,
		// Chrome sends :method :authority :scheme :path, which differs from the
		// alphabetical order produced by the Go standard library.
		pseudoHeaderOrder: []string{":method", ":authority", ":scheme", ":path"},
		headersPriority: http2.PriorityParam{
			StreamDep: 0,
			Exclusive: true,
			Weight:    chromeH2HeadersWeight,
		},
		endStreamOnHeaders: true,
	},
}

func getHTTP2Profile(name string) (*http2Profile, error) {
	profile, ok := http2Profiles[name]
	if !ok {
		return nil, fmt.Errorf("unknown HTTP/2 fingerprint profile %q (available: %v)", name, AvailableHTTP2Profiles())
	}
	return profile, nil
}

// ValidateHTTP2Profile reports whether name refers to a built-in HTTP/2
// fingerprint profile. An empty name is valid and means the default framing.
func ValidateHTTP2Profile(name string) error {
	if name == "" {
		return nil
	}
	_, err := getHTTP2Profile(name)
	return err
}

// AvailableHTTP2Profiles lists the built-in HTTP/2 fingerprint profile names.
func AvailableHTTP2Profiles() []string {
	profiles := make([]string, 0, len(http2Profiles))
	for name := range http2Profiles {
		profiles = append(profiles, name)
	}
	sort.Strings(profiles)
	return profiles
}

// reorderPseudoHeaders returns fields with pseudo headers moved to the front in
// the profile's order. Pseudo headers the profile does not name keep their
// relative order behind the named ones, and regular headers follow untouched.
func (p *http2Profile) reorderPseudoHeaders(fields []hpack.HeaderField) []hpack.HeaderField {
	if p == nil || len(p.pseudoHeaderOrder) == 0 {
		return fields
	}
	out := make([]hpack.HeaderField, 0, len(fields))
	picked := make(map[string]bool, len(p.pseudoHeaderOrder))
	for _, name := range p.pseudoHeaderOrder {
		for _, field := range fields {
			if field.Name == name {
				out = append(out, field)
				picked[name] = true
				break
			}
		}
	}
	for _, field := range fields {
		if strings.HasPrefix(field.Name, ":") && !picked[field.Name] {
			out = append(out, field)
		}
	}
	for _, field := range fields {
		if !strings.HasPrefix(field.Name, ":") {
			out = append(out, field)
		}
	}
	return out
}
