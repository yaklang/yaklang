package gcc

import (
	"encoding/binary"
	"io"
	"log"
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/glog"
)

func TestMain(m *testing.M) {
	glog.SetLogger(log.New(io.Discard, "", 0))
	glog.SetLevel(glog.NONE)
	os.Exit(m.Run())
}

func TestMakeConferenceCreateResponse_RoundTrip(t *testing.T) {
	coreData := make([]byte, 8)
	binary.LittleEndian.PutUint32(coreData[0:], 0x00080004)
	secData := make([]byte, 16)
	netData := make([]byte, 4)
	binary.LittleEndian.PutUint16(netData[0:], 1003)
	block := func(typ uint16, data []byte) []byte {
		l := 4 + len(data)
		out := make([]byte, l)
		binary.LittleEndian.PutUint16(out[0:], typ)
		binary.LittleEndian.PutUint16(out[2:], uint16(l))
		copy(out[4:], data)
		return out
	}
	var user []byte
	user = append(user, block(uint16(SC_CORE), coreData)...)
	user = append(user, block(uint16(SC_SECURITY), secData)...)
	user = append(user, block(uint16(SC_NET), netData)...)
	raw := MakeConferenceCreateResponse(user)
	got := ReadConferenceCreateResponse(raw)
	if len(got) != 3 {
		t.Fatalf("blocks=%d want 3", len(got))
	}
	sec, ok := got[1].(*ServerSecurityData)
	if !ok {
		t.Fatalf("block1 type %T", got[1])
	}
	if sec.EncryptionMethod != 0 {
		t.Fatalf("EncryptionMethod=%d want 0", sec.EncryptionMethod)
	}
}
