package pdu

import "testing"

func TestXPLogonHintNcrackPatterns(t *testing.T) {
	if xpLogonHint(xpLogonFailedXP) != xpHintFail {
		t.Fatal("failed XP dialog must be fail")
	}
	if xpLogonHint(xpLogonCurrentUserXP) != xpHintSuccess {
		t.Fatal("current user XP dialog is valid creds")
	}
	if xpLogonHint(xpLogonNotInRDPGroup) != xpHintSuccess {
		t.Fatal("not in remote desktop group is still valid creds")
	}
	if !classicShareRestart(0x1) || !classicShareRestart(0x6) || classicShareRestart(0x7) {
		t.Fatal("share restart is Demand Active(1) or Deactivate All(6) only")
	}
	if xpLogonHint([]byte{0x02, 0x00, 0x01, 0x00}) != xpHintNone {
		t.Fatal("ordinary update is not a logon hint")
	}
	wallpaper := make([]byte, 2818)
	wallpaper[0], wallpaper[1] = 0x01, 0x00
	if xpLogonHint(wallpaper) != xpHintNone {
		t.Fatal("live XP wallpaper tiles are painted on both success and fail")
	}
	wrapped := append([]byte{0x00, 0x01}, append(xpLogonFailed2K3, 0xff)...)
	if xpLogonHint(wrapped) != xpHintFail {
		t.Fatal("pattern in the middle of an order stream")
	}
}
