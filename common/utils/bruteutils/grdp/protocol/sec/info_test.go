package sec

import (
	"testing"
)

func TestRDPInfoSerialize_LegacyHasNoExtended(t *testing.T) {
	c := &Client{SEC: &SEC{info: NewRDPInfo()}}
	c.SetUser("Administrator")
	c.SetPwd("RdpPass123!")
	c.SetDomain("")
	legacy := c.info.Serialize(false)
	extended := c.info.Serialize(true)
	if len(extended) <= len(legacy) {
		t.Fatalf("extended (%d) should be longer than legacy (%d)", len(extended), len(legacy))
	}
	if c.info.Flag&INFO_AUTOLOGON == 0 {
		t.Fatal("AUTOLOGON must be set for XP/2003 brute")
	}
	if c.info.Flag&INFO_LOGONNOTIFY == 0 {
		t.Fatal("LOGONNOTIFY must be set so the server sends SAVE_SESSION_INFO")
	}
}
