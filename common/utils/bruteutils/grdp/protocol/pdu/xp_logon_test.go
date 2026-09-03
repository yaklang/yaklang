package pdu

import "testing"

func TestXPLogonHintNcrackPatterns(t *testing.T) {
	if xpLogonHint(xpLogonFailedXP) != xpHintFail {
		t.Fatal("failed XP dialog must be fail")
	}
	if xpLogonHint(xpLogonCurrentUserXP) != xpHintSuccess {
		t.Fatal("current user XP dialog is valid creds")
	}
	if xpLogonHint([]byte{0x02, 0x00, 0x01, 0x00}) != xpHintNone {
		t.Fatal("ordinary update is not a logon hint")
	}
	wrapped := append([]byte{0x00, 0x01}, append(xpLogonFailed2K3, 0xff)...)
	if xpLogonHint(wrapped) != xpHintFail {
		t.Fatal("pattern in the middle of an order stream")
	}
}
