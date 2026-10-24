package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func needsMoreNBSSPrefix(w []byte, limit int) bool {
	if len(w) == 0 || w[0] != 0x81 {
		return false
	}
	if len(w) < 4 {
		return true
	}
	if w[1]&0xfe != 0 {
		return false
	}
	n := 4 + int(binary.BigEndian.Uint16(w[2:4])) + (int(w[1]&1) << 16)
	return n >= 72 && n <= min(limit, 514) && len(w) < n
}

func (f *binFlow) frameNBSS(dir int, w []byte) (int, *binSpec, error) {
	if len(w) < 4 {
		return 0, nil, nil
	}
	if w[1]&0xfe != 0 {
		return 0, nil, fmt.Errorf("nbss: reserved flags")
	}
	n := 4 + int(binary.BigEndian.Uint16(w[2:4])) + int(w[1]&1)<<16
	if n > f.a.config.MaxMessageBytes {
		return n, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	if w[0] == 0 {
		if n >= 8 && bytes.Equal(w[4:8], []byte{254, 'S', 'M', 'B'}) {
			if f.smb2 == nil {
				f.protocol = "smb2"
				f.smb2 = &binSMB2{client: dir, pending: map[smb2PendingKey]smb2Pending{}, async: map[smb2AsyncKey]smb2PendingKey{}, files: map[smb2FileKey]string{}}
			}
			return f.frameSMB2(w)
		}
		return 0, nil, protocolError(ErrUnsupportedFeature, "NBSS session payload requires a supported SMB2 profile")
	}
	return n, &binSpec{rule: "network_control_fields", entry: "NBSSControl"}, nil
}
