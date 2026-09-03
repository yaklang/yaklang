package bruteutils

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/core"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/lic"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/pdu"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/t125"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/t125/ber"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/t125/gcc"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/t125/per"
)

// classicRDPServer 模拟 XP/2003 风格 PROTOCOL_RDP：
// X.224 无协商 → MCS（无 RC4）→ Client Info AUTOLOGON → FontMap →
// 密码正确发 SAVE_SESSION_INFO，错误则保持会话但不发 logon。
type classicRDPServer struct {
	listener            net.Listener
	mu                  sync.Mutex
	known               map[string]string
	lastUser            string
	lastPass            string
	lastAuthed          bool
	successWithoutLogon bool // 真机 XP：成功不发 0x26，只维持会话
}

func startClassicRDPServer(t *testing.T, users map[string]string) *classicRDPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &classicRDPServer{listener: ln, known: users}
	go s.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func (s *classicRDPServer) addr() string { return s.listener.Addr().String() }

func (s *classicRDPServer) serve() {
	for {
		c, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}

func (s *classicRDPServer) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	// 1. X.224 CR → CC without rdpNegData（XP 风格）
	pkt, err := readTPKT(conn)
	if err != nil {
		return
	}
	_ = pkt
	cc := []byte{0x06, 0xd0, 0x00, 0x00, 0x12, 0x34, 0x00}
	if _, err := conn.Write(tpktWrap(cc)); err != nil {
		return
	}

	joined := 0
	authed := false
	finalized := false
	for {
		body, err := readTPKT(conn)
		if err != nil {
			return
		}
		payload := stripX224Data(body)
		if len(payload) == 0 {
			return
		}
		if os.Getenv("YAK_RDP_CLASSIC_DEBUG") != "" {
			n := len(payload)
			if n > 16 {
				n = 16
			}
			log.Printf("classic mock got op=%d first=%02x %s", payload[0]>>2, payload[0], hex.EncodeToString(payload[:n]))
		}
		// Connect Initial 是 BER Application 101（0x65）或长标签 0x7F 0x65。
		// 不能用 0x60 掩码：SEND_DATA_REQUEST=0x64 会被误判，再次回 Connect Response。
		if payload[0] == 0x7f || payload[0] == 0x65 {
			if err := s.sendConnectResponse(conn); err != nil {
				return
			}
			continue
		}
		op := payload[0] >> 2
		switch op {
		case 1: // Erect Domain
			continue
		case 10: // Attach User Request
			if err := writeTPKTX224(conn, []byte{0x2c, 0x00, 0x00, 0x01}); err != nil {
				return
			}
		case 14: // Channel Join：MCS header(1)+userId(2)+channel(2) 共 5 字节
			if len(payload) < 5 {
				return
			}
			ch := binary.BigEndian.Uint16(payload[len(payload)-2:])
			buf := &bytes.Buffer{}
			buf.WriteByte(0x3c)
			buf.WriteByte(0x00)
			per.WriteInteger16(1, buf)
			per.WriteInteger16(ch, buf)
			if err := writeTPKTX224(conn, buf.Bytes()); err != nil {
				return
			}
			joined++
		case 25: // Send Data Request (Client Info / Confirm Active / Font List)
			user, pass := parseClientInfoFromMCS(payload)
			if user != "" {
				s.mu.Lock()
				s.lastUser, s.lastPass = user, pass
				want, ok := s.known[strings.ToLower(user)]
				authed = ok && want == pass
				s.lastAuthed = authed
				s.mu.Unlock()
				if err := s.sendLicense(conn); err != nil {
					return
				}
				if err := s.sendDemandActive(conn); err != nil {
					return
				}
				continue
			}
			if finalized {
				continue
			}
			finalized = true
			if err := s.sendServerFinalize(conn, authed); err != nil {
				return
			}
			if !authed {
				// 错密码：FontMap 后关掉连接。客户端已 ready，EOF 记认证失败。
				return
			}
		}
	}
}

func (s *classicRDPServer) sendConnectResponse(conn net.Conn) error {
	coreData := make([]byte, 8)
	binary.LittleEndian.PutUint32(coreData[0:], 0x00080004)
	secData := make([]byte, 16) // EncryptionMethod=0
	netData := make([]byte, 4)
	binary.LittleEndian.PutUint16(netData[0:], 1003)
	var blocks []byte
	blocks = append(blocks, gccBlock(0x0C01, coreData)...)
	blocks = append(blocks, gccBlock(0x0C02, secData)...)
	blocks = append(blocks, gccBlock(0x0C03, netData)...)
	userData := gcc.MakeConferenceCreateResponse(blocks)

	dp := t125.NewDomainParameters(22, 3, 0, 1, 0, 1, 0xfff8, 2)
	inner := &bytes.Buffer{}
	ber.WriteEnumerated(0, inner)
	ber.WriteInteger(0, inner)
	ber.WriteEncodedDomainParams(dp.BER(), inner)
	ber.WriteOctetstring(string(userData), inner)
	out := &bytes.Buffer{}
	ber.WriteApplicationTag(0x66, inner.Len(), out)
	out.Write(inner.Bytes())
	return writeTPKTX224(conn, out.Bytes())
}

func (s *classicRDPServer) sendLicense(conn net.Conn) error {
	licBuf := &bytes.Buffer{}
	core.WriteUInt16LE(0x0080, licBuf) // LICENSE_PKT
	core.WriteUInt16LE(0, licBuf)
	core.WriteUInt8(lic.ERROR_ALERT, licBuf)
	core.WriteUInt8(0x02, licBuf)
	core.WriteUInt16LE(16, licBuf)
	core.WriteUInt32LE(lic.STATUS_VALID_CLIENT, licBuf)
	core.WriteUInt32LE(lic.ST_NO_TRANSITION, licBuf)
	return writeMCSGlobal(conn, licBuf.Bytes())
}

func (s *classicRDPServer) sendDemandActive(conn net.Conn) error {
	da := &pdu.DemandActivePDU{
		SharedId:               0x103EA,
		LengthSourceDescriptor: 1,
		SourceDescriptor:       []byte{0},
		CapabilitySets: []pdu.Capability{
			&pdu.GeneralCapability{ProtocolVersion: 0x0200},
		},
	}
	payload := da.Serialize()
	return writeMCSGlobal(conn, encodeSharePDU(0x11, 1002, payload))
}

func (s *classicRDPServer) sendServerFinalize(conn net.Conn, authed bool) error {
	// 与真实 Windows 连接序列一致：Synchronize → Cooperate → Granted → FontMap
	// →（正确密码才发）SAVE_SESSION_INFO。FontMap 只表示会话建立，不是爆破成功。
	syncPDU := encodeDataPDU(0x1f, []byte{0x01, 0x00, 0xea, 0x03}, 1002, 0x103EA)
	if err := writeMCSGlobal(conn, syncPDU); err != nil {
		return err
	}
	coop := encodeDataPDU(0x14, []byte{0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 1002, 0x103EA)
	if err := writeMCSGlobal(conn, coop); err != nil {
		return err
	}
	granted := encodeDataPDU(0x14, []byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 1002, 0x103EA)
	if err := writeMCSGlobal(conn, granted); err != nil {
		return err
	}
	font := encodeDataPDU(0x28, []byte{0, 0, 0, 0, 3, 0, 4, 0}, 1002, 0x103EA)
	if err := writeMCSGlobal(conn, font); err != nil {
		return err
	}
	if !authed {
		// 真机 XP 错用户会画登录失败对话框（ncrack LOGON_MESSAGE_FAILED_XP）。
		fail := []byte{0x17, 0x00, 0x18, 0x06, 0x10, 0x06, 0x1a, 0x09, 0x1b, 0x05, 0x1a, 0x06, 0x1c, 0x05, 0x10, 0x04, 0x1d, 0x06}
		return writeMCSGlobal(conn, encodeDataPDU(0x02, fail, 1002, 0x103EA))
	}
	if s.successWithoutLogon {
		// 真机 XP 成功路径：FontMap 后只推位图，没有 0x26。
		bmp := make([]byte, 32)
		bmp[0], bmp[1] = 0x01, 0x00
		return writeMCSGlobal(conn, encodeDataPDU(0x02, bmp, 1002, 0x103EA))
	}
	logon := encodeLogonV1("Administrator", "", 1)
	return writeMCSGlobal(conn, encodeDataPDU(0x26, logon, 1002, 0x103EA))
}

func encodeLogonV1(user, domain string, session uint32) []byte {
	b := &bytes.Buffer{}
	core.WriteUInt32LE(0, b) // INFOTYPE_LOGON
	du := padUnicode(domain, 52)
	uu := padUnicode(user, 512)
	core.WriteUInt32LE(uint32(len(utf16.Encode([]rune(domain)))*2), b)
	b.Write(du)
	core.WriteUInt32LE(uint32(len(utf16.Encode([]rune(user)))*2), b)
	b.Write(uu)
	core.WriteUInt32LE(session, b)
	return b.Bytes()
}

func padUnicode(s string, n int) []byte {
	u := utf16.Encode([]rune(s))
	out := make([]byte, n)
	for i, c := range u {
		if i*2+1 >= n {
			break
		}
		binary.LittleEndian.PutUint16(out[i*2:], c)
	}
	return out
}

func encodeSharePDU(pduType, userId uint16, payload []byte) []byte {
	b := &bytes.Buffer{}
	core.WriteUInt16LE(uint16(6+len(payload)), b)
	core.WriteUInt16LE(pduType, b)
	core.WriteUInt16LE(userId, b)
	b.Write(payload)
	return b.Bytes()
}

func encodeDataPDU(type2 byte, payload []byte, userId uint16, shareId uint32) []byte {
	inner := &bytes.Buffer{}
	core.WriteUInt32LE(shareId, inner)
	core.WriteUInt8(0, inner)
	core.WriteUInt8(1, inner) // STREAM_LOW
	core.WriteUInt16LE(uint16(12+len(payload)), inner)
	core.WriteUInt8(type2, inner)
	core.WriteUInt8(0, inner)
	core.WriteUInt16LE(0, inner)
	inner.Write(payload)
	return encodeSharePDU(0x17, userId, inner.Bytes())
}

func gccBlock(t uint16, data []byte) []byte {
	l := 4 + len(data)
	out := make([]byte, l)
	binary.LittleEndian.PutUint16(out[0:], t)
	binary.LittleEndian.PutUint16(out[2:], uint16(l))
	copy(out[4:], data)
	return out
}

func tpktWrap(body []byte) []byte {
	n := 4 + len(body)
	return append([]byte{0x03, 0x00, byte(n >> 8), byte(n)}, body...)
}

func writeTPKTX224(conn net.Conn, mcs []byte) error {
	body := append([]byte{0x02, 0xf0, 0x80}, mcs...)
	pkt := tpktWrap(body)
	if os.Getenv("YAK_RDP_CLASSIC_DEBUG") != "" {
		n := len(pkt)
		if n > 24 {
			n = 24
		}
		log.Printf("classic mock send %d %s", len(pkt), hex.EncodeToString(pkt[:n]))
	}
	_, err := conn.Write(pkt)
	return err
}

func writeMCSGlobal(conn net.Conn, data []byte) error {
	buf := &bytes.Buffer{}
	buf.WriteByte(26 << 2) // SEND_DATA_INDICATION
	per.WriteInteger16(1, buf)
	per.WriteInteger16(1003, buf)
	core.WriteUInt8(0x70, buf)
	per.WriteLength(len(data), buf)
	buf.Write(data)
	return writeTPKTX224(conn, buf.Bytes())
}

func readTPKT(conn net.Conn) ([]byte, error) {
	h := make([]byte, 4)
	if _, err := io.ReadFull(conn, h); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint16(h[2:4]))
	if n < 4 {
		return nil, io.ErrUnexpectedEOF
	}
	body := make([]byte, n-4)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, err
	}
	return body, nil
}

func stripX224Data(body []byte) []byte {
	if len(body) >= 3 && body[1] == 0xf0 {
		return body[3:]
	}
	return body
}

func parseClientInfoFromMCS(payload []byte) (user, pass string) {
	// MCS header + optional 0x70 length + security header + info
	r := bytes.NewReader(payload)
	_, _ = core.ReadUInt8(r) // opcode
	_, _ = per.ReadInteger16(r)
	_, _ = per.ReadInteger16(r)
	_, _ = core.ReadUInt8(r)
	ln, _ := per.ReadLength(r)
	data, err := core.ReadBytes(int(ln), r)
	if err != nil || len(data) < 20 {
		return "", ""
	}
	flag := binary.LittleEndian.Uint16(data[0:2])
	if flag&0x0040 == 0 { // not INFO_PKT
		return "", ""
	}
	info := data[4:]
	if len(info) < 18 {
		return "", ""
	}
	cbDomain := int(binary.LittleEndian.Uint16(info[8:10]))
	cbUser := int(binary.LittleEndian.Uint16(info[10:12]))
	cbPass := int(binary.LittleEndian.Uint16(info[12:14]))
	off := 18
	if off+cbDomain+2+cbUser+2+cbPass+2 > len(info) {
		return "", ""
	}
	off += cbDomain + 2
	user = strings.TrimRight(core.UnicodeDecode(info[off:off+cbUser+2]), "\x00")
	off += cbUser + 2
	pass = strings.TrimRight(core.UnicodeDecode(info[off:off+cbPass+2]), "\x00")
	return user, pass
}

func TestClassicRDPBruteSuccessAndFail(t *testing.T) {
	srv := startClassicRDPServer(t, map[string]string{"administrator": "RdpPass123!"})
	hit := func(u, p string) *BruteItemResult {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return rdpAuth.BrutePass(&BruteItem{
			Type: "rdp", Target: srv.addr(), Username: u, Password: p, Context: ctx,
		})
	}

	ok := hit("Administrator", "RdpPass123!")
	if !ok.Ok || !ok.Finished {
		t.Fatalf("correct XP-style creds: want ok+finished got ok=%v finished=%v", ok.Ok, ok.Finished)
	}
	bad := hit("Administrator", "WrongPass!")
	if bad.Ok {
		t.Fatal("wrong password must not be ok")
	}
	if bad.Finished {
		t.Fatal("wrong password must not finish the target")
	}
	unk := hit("no-such-user", "x")
	if unk.Ok {
		t.Fatal("unknown user must not be ok")
	}
	if unk.Finished {
		t.Fatal("unknown user must not finish the target")
	}
}

func TestClassicRDPDictHunt(t *testing.T) {
	srv := startClassicRDPServer(t, map[string]string{"administrator": "CorrectHorse!"})
	util, err := NewMultiTargetBruteUtilEx(
		WithBruteCallback(rdpAuth.BrutePass),
		WithOkToStop(true),
		WithTargetsConcurrent(1),
		WithTargetTasksConcurrent(1),
	)
	if err != nil {
		t.Fatal(err)
	}
	var found *BruteItemResult
	err = util.StreamBruteContext(context.Background(), "rdp",
		[]string{srv.addr()},
		[]string{"guest", "Administrator"},
		[]string{"123456", "CorrectHorse!"},
		func(res *BruteItemResult) {
			if res.Ok {
				cp := *res
				found = &cp
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	if found == nil || found.Username != "Administrator" || found.Password != "CorrectHorse!" {
		t.Fatalf("dict hunt missed, found=%+v", found)
	}
}
