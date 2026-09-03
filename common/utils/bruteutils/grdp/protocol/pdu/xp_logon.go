package pdu

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"

	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/glog"
)

// ncrack 从 XP/2003 登录对话框的绘图订单里抠出来的固定字节串
// （modules/ncrack_rdp.cc LOGON_MESSAGE_*）。
var (
	xpLogonAuthFailed    = []byte{0xfe, 0x00, 0x00}
	xpLogonFailedXP      = []byte{0x17, 0x00, 0x18, 0x06, 0x10, 0x06, 0x1a, 0x09, 0x1b, 0x05, 0x1a, 0x06, 0x1c, 0x05, 0x10, 0x04, 0x1d, 0x06}
	xpLogonFailed2K3     = []byte{0x11, 0x00, 0x12, 0x06, 0x13, 0x06, 0x15, 0x09, 0x16, 0x05, 0x15, 0x06, 0x17, 0x05, 0x13, 0x04, 0x18, 0x06}
	xpLogonCurrentUserXP = []byte{0x12, 0x00, 0x13, 0x07, 0x10, 0x05, 0x14, 0x06, 0x0e, 0x07, 0x0d, 0x06, 0x16, 0x06, 0x10, 0x08, 0x17, 0x06}
	xpLogonLockedXP      = []byte{0x17, 0x00, 0x0e, 0x07, 0x0d, 0x06, 0x18, 0x06, 0x11, 0x06, 0x10, 0x02, 0x1a, 0x09, 0x1b, 0x04, 0x11, 0x09}
	xpLogonDisabledXP    = []byte{0x17, 0x00, 0x18, 0x06, 0x19, 0x06, 0x1a, 0x06, 0x0d, 0x07, 0x0f, 0x06, 0x0f, 0x05, 0x18, 0x05, 0x19, 0x06}
	xpLogonExpiredXP     = []byte{0x17, 0x00, 0x18, 0x06, 0x19, 0x06, 0x0d, 0x09, 0x1b, 0x06, 0x10, 0x04, 0x1b, 0x09, 0x10, 0x04, 0x1c, 0x06}
	xpLogonMustChangeXP  = []byte{0x17, 0x00, 0x18, 0x06, 0x19, 0x06, 0x0d, 0x09, 0x1b, 0x06, 0x10, 0x04, 0x1b, 0x09, 0x10, 0x04, 0x1c, 0x06}
	// 账密对，但策略不允许交互/远程登录 —— ncrack 仍记为成功。
	xpLogonNotInRDPGroup = []byte{0x11, 0x00, 0x12, 0x06, 0x14, 0x09, 0x12, 0x02, 0x15, 0x06, 0x12, 0x09, 0x16, 0x06, 0x17, 0x09, 0x12, 0x04}
	xpLogonNoInteractive = []byte{0x00, 0x00, 0x01, 0x06, 0x02, 0x06, 0x04, 0x09, 0x05, 0x02, 0x06, 0x06, 0x07, 0x05, 0x04, 0x06, 0x08, 0x05}
)

type xpHint int

const (
	xpHintNone xpHint = iota
	xpHintFail
	xpHintSuccess
)

func xpLogonHint(raw []byte) xpHint {
	if len(raw) == 0 {
		return xpHintNone
	}
	if bytes.Contains(raw, xpLogonFailedXP) || bytes.Contains(raw, xpLogonFailed2K3) ||
		bytes.Contains(raw, xpLogonAuthFailed) {
		return xpHintFail
	}
	if bytes.Contains(raw, xpLogonLockedXP) || bytes.Contains(raw, xpLogonDisabledXP) {
		return xpHintFail
	}
	if bytes.Contains(raw, xpLogonCurrentUserXP) || bytes.Contains(raw, xpLogonExpiredXP) ||
		bytes.Contains(raw, xpLogonMustChangeXP) || bytes.Contains(raw, xpLogonNotInRDPGroup) ||
		bytes.Contains(raw, xpLogonNoInteractive) {
		return xpHintSuccess
	}
	return xpHintNone
}

// classicShareRestart reports FontMap 之后的会话切换：Demand Active(1)
// 或 Deactivate All(6)。真机 XP AUTOLOGON 成功走这条，不发 0x26。
func classicShareRestart(kind uint16) bool {
	return kind == 0x1 || kind == 0x6
}

func remainingShareType(s []byte, left int) uint16 {
	if left < 6 || left > len(s) {
		return 0
	}
	off := len(s) - left
	return binary.LittleEndian.Uint16(s[off+2:]) & 0x000f
}

// xpDesktopBitmap reports the live XP wallpaper tiles we captured.
// Do NOT treat this as success: the wrong-password path paints the same
// 2818-byte UPDATETYPE_BITMAP tiles before the fail dialog.

func dumpSharePDUs(s []byte) {
	off := 0
	for off+6 <= len(s) {
		total := int(binary.LittleEndian.Uint16(s[off:]))
		pduType := binary.LittleEndian.Uint16(s[off+2:]) & 0x000f
		t2 := byte(0xff)
		if off+15 <= len(s) {
			t2 = s[off+14]
		}
		glog.Infof("share off=%d total=%d ctrl=%d type2=0x%02x", off, total, pduType, t2)
		if total < 6 {
			glog.Infof("share stop: bad total at %d", off)
			break
		}
		if off+total > len(s) {
			glog.Infof("share truncated: need %d have %d", off+total, len(s))
			break
		}
		off += total
	}
	if off < len(s) {
		n := len(s) - off
		if n > 16 {
			n = 16
		}
		glog.Infof("share unused %d %s", len(s)-off, hex.EncodeToString(s[off:off+n]))
	}
	for i := 0; i+15 <= len(s); i++ {
		if s[i+2] == 0x17 && s[i+3] == 0x00 && s[i+14] == 0x26 {
			glog.Infof("FOUND 0x26 SAVE_SESSION_INFO at offset %d", i)
		}
	}
}
