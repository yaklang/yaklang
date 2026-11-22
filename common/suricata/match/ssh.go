package match

import (
	"bytes"

	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

// sshProtoParser exposes the SSH version banner as the ssh.proto sticky
// buffer. Suricata's SSH parser records the banner's protocol and software
// fields; content matching on the combined banner covers both, including
// legacy rules that use ssh_proto with a software marker such as "PUTTY".
func sshProtoParser(c *matchContext) error {
	if !c.Must(c.Rule.ContentRuleConfig != nil) {
		return nil
	}
	payload := c.overridePayload
	if len(payload) == 0 {
		if c.PK == nil || c.PK.TransportLayer() == nil {
			return nil
		}
		payload = c.PK.TransportLayer().LayerPayload()
	}
	banner := sshVersionBanner(payload)
	if !c.Must(banner != nil) {
		return nil
	}
	software := sshSoftwareField(banner)
	c.SetBufferProvider(func(mdf modifier.Modifier) []byte {
		switch mdf {
		case modifier.SSHProto:
			return banner
		case modifier.SSHSoftware:
			return software
		case modifier.Default:
			return payload
		}
		return nil
	})
	return nil
}

// LooksLikeSSH reports whether payload begins with or contains a valid SSH
// version identification banner line ("SSH-").
func LooksLikeSSH(payload []byte) bool {
	return sshVersionBanner(payload) != nil
}

// sshSoftwareField extracts the software version segment after "SSH-<proto>-"
// (per RFC 4253 / Suricata ssh.software). For example, "SSH-2.0-Erlang/5.2.0"
// yields "Erlang/5.2.0".
func sshSoftwareField(banner []byte) []byte {
	if !bytes.HasPrefix(banner, []byte("SSH-")) {
		return nil
	}
	rest := banner[len("SSH-"):]
	dash := bytes.IndexByte(rest, '-')
	if dash < 0 || dash+1 >= len(rest) {
		return nil
	}
	sw := rest[dash+1:]
	if sp := bytes.IndexByte(sw, ' '); sp >= 0 {
		sw = sw[:sp]
	}
	return sw
}

// sshVersionBanner returns the first "SSH-" banner line. SSH peers may send
// comment lines before the version string, so the marker is searched at the
// start of each line rather than only at offset zero.
func sshVersionBanner(payload []byte) []byte {
	for offset := 0; offset < len(payload); {
		lineEnd := bytes.IndexByte(payload[offset:], '\n')
		if lineEnd < 0 {
			lineEnd = len(payload) - offset
		}
		line := payload[offset : offset+lineEnd]
		line = bytes.TrimSuffix(line, []byte("\r"))
		if bytes.HasPrefix(line, []byte("SSH-")) {
			return append([]byte(nil), line...)
		}
		if offset+lineEnd >= len(payload) {
			break
		}
		offset += lineEnd + 1
	}
	return nil
}
