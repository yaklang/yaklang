package x224

import (
	"encoding/hex"
	"io"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/glog"
)

func TestMain(m *testing.M) {
	glog.SetLogger(log.New(io.Discard, "", 0))
	glog.SetLevel(glog.NONE)
	os.Exit(m.Run())
}

func TestParseConnectionConfirm_ClassicXPNoNegotiation(t *testing.T) {
	// TPKT payload of a pre-RDP 5.2 / XP CC: LI + CC + dst/src/class, no rdpNegData.
	s, _ := hex.DecodeString("06d00000000000")
	got, err := parseConnectionConfirm(s)
	if err != nil {
		t.Fatal(err)
	}
	if got != PROTOCOL_RDP {
		t.Fatalf("selected=%d want PROTOCOL_RDP", got)
	}
}

func TestParseConnectionConfirm_NegRspRDP(t *testing.T) {
	s, _ := hex.DecodeString("0ed000001234000200080000000000")
	got, err := parseConnectionConfirm(s)
	if err != nil {
		t.Fatal(err)
	}
	if got != PROTOCOL_RDP {
		t.Fatalf("selected=%d want PROTOCOL_RDP", got)
	}
}

func TestParseConnectionConfirm_NegRspHybrid(t *testing.T) {
	s, _ := hex.DecodeString("0ed000001234000200080002000000")
	got, err := parseConnectionConfirm(s)
	if err != nil {
		t.Fatal(err)
	}
	if got != PROTOCOL_HYBRID {
		t.Fatalf("selected=%d want PROTOCOL_HYBRID", got)
	}
}

func TestParseConnectionConfirm_SSLNotAllowedFallsBackToRDP(t *testing.T) {
	s, _ := hex.DecodeString("0ed000001234000300080002000000")
	got, err := parseConnectionConfirm(s)
	if err != nil {
		t.Fatal(err)
	}
	if got != PROTOCOL_RDP {
		t.Fatalf("selected=%d want PROTOCOL_RDP fallback", got)
	}
}

func TestParseConnectionConfirm_HybridRequiredIsError(t *testing.T) {
	s, _ := hex.DecodeString("0ed000001234000300080005000000")
	_, err := parseConnectionConfirm(s)
	if err == nil {
		t.Fatal("expected error for HYBRID_REQUIRED_BY_SERVER")
	}
	if !strings.Contains(err.Error(), "negotiation failure code 5") {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestParseConnectionConfirm_TooShort(t *testing.T) {
	if _, err := parseConnectionConfirm([]byte{0x06, 0xd0}); err == nil {
		t.Fatal("expected error")
	}
}
