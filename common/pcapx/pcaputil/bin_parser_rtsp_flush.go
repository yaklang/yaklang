package pcaputil

import (
	"strconv"
	"strings"
)

// AirPlay's observed FLUSH form uses seq/rtptime without a URL. Expose only
// header observations; these values never reset an RTP source or install media.
func rtspFlushRTPInfo(values []string) ([]any, error) {
	if len(values) != 1 {
		return nil, protocolError(ErrMalformedMessage, "RTSP FLUSH duplicate RTP-Info")
	}
	items := strings.Split(values[0], ",")
	if len(items) > 64 {
		return nil, protocolError(ErrResourceExceeded, "RTSP FLUSH RTP-Info item budget")
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{"Observed Header Value": item, "Media Effect Verified": false}
		seen := map[string]bool{}
		for _, param := range strings.Split(item, ";") {
			key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
			if !ok || key == "" || value == "" || seen[key] {
				return nil, protocolError(ErrMalformedMessage, "RTSP FLUSH invalid or duplicate RTP-Info parameter")
			}
			seen[key] = true
			switch key {
			case "seq", "rtptime":
				bits := 32
				if key == "seq" {
					bits = 16
				}
				for _, b := range []byte(value) {
					if b < '0' || b > '9' {
						return nil, protocolError(ErrMalformedMessage, "RTSP FLUSH invalid RTP-Info decimal")
					}
				}
				n, err := strconv.ParseUint(value, 10, bits)
				if err != nil {
					return nil, protocolError(ErrMalformedMessage, "RTSP FLUSH RTP-Info integer exceeds wire width")
				}
				entry[key] = n
			case "url":
				entry["url"] = value
			default:
				entry["Uninterpreted Parameter "+key] = value
			}
		}
		out = append(out, entry)
	}
	return out, nil
}
