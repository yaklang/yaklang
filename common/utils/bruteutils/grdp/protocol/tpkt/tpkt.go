package tpkt

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/core"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/emission"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/glog"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/nla"
)

var errNonRDPPeer = errors.New("protocol error: peer does not speak RDP/TPKT")

// take idea from https://github.com/Madnikulin50/gordp

/**
 * Type of tpkt packet
 * Fastpath is use to shortcut RDP stack
 * @see http://msdn.microsoft.com/en-us/library/cc240621.aspx
 * @see http://msdn.microsoft.com/en-us/library/cc240589.aspx
 */
const (
	FASTPATH_ACTION_FASTPATH = 0x0
	FASTPATH_ACTION_X224     = 0x3
)

/**
 * TPKT layer of rdp stack
 */
type TPKT struct {
	emission.Emitter
	Conn             *core.SocketLayer
	ntlm             *nla.NTLMv2
	secFlag          byte
	lastShortLength  int
	fastPathListener core.FastPathListener
	ntlmSec          *nla.NTLMv2Security
}

func New(s *core.SocketLayer, ntlm *nla.NTLMv2) *TPKT {
	t := &TPKT{
		Emitter: *emission.NewEmitter(),
		Conn:    s,
		secFlag: 0,
		ntlm:    ntlm}
	core.StartReadBytes(2, s, t.recvHeader)
	return t
}

func (t *TPKT) StartTLS() error {
	return t.Conn.StartTLS()
}

func isNLAAuthDrop(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "tls: access denied") ||
		strings.Contains(s, "tls: internal error") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "broken pipe")
}

func generateCredSSPNonce() ([]byte, error) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("rdp nla: generate nonce: %w", err)
	}
	return nonce, nil
}

func (t *TPKT) StartNLA() error {
	err := t.StartTLS()
	if err != nil {
		glog.Info("start tls failed", err)
		return fmt.Errorf("rdp tls handshake: %w", err)
	}

	nonce, err := generateCredSSPNonce()
	if err != nil {
		return err
	}

	// Advertise v6 from the first TSRequest (Windows 10/2016+ require v5+
	// after the CVE-2018-0886 Encryption Oracle Remediation policy).
	req := nla.EncodeDERTRequest(nla.DefaultCredSSPVersion,
		[]nla.Message{t.ntlm.GetNegotiateMessage()}, nil, nil, nonce)
	_, err = t.Conn.Write(req)
	if err != nil {
		glog.Info("send NegotiateMessage", err)
		return fmt.Errorf("rdp nla: send negotiate: %w", err)
	}

	tsreq, err := nla.ReadTSRequest(t.Conn)
	if err != nil {
		return fmt.Errorf("rdp nla: read challenge: %w", err)
	}
	glog.Debug("StartNLA Read success")
	return t.recvChallenge(tsreq, nonce)
}

func (t *TPKT) recvChallenge(tsreq *nla.TSRequest, nonce []byte) error {
	glog.Debugf("tsreq:%+v", tsreq)
	if err := tsreq.AuthError(); err != nil {
		return err
	}
	if len(tsreq.NegoTokens) == 0 {
		return fmt.Errorf("rdp nla: no NegoTokens in challenge (server version %d)", tsreq.Version)
	}

	pubkey, err := t.Conn.TlsPubKey()
	if err != nil {
		return fmt.Errorf("rdp nla: tls public key: %w", err)
	}
	glog.Debugf("pubkey=%+v", pubkey)

	effective := nla.EffectiveVersion(nla.DefaultCredSSPVersion, tsreq.Version)
	glog.Debugf("CredSSP version client=%d server=%d effective=%d",
		nla.DefaultCredSSPVersion, tsreq.Version, effective)

	authMsg, ntlmSec := t.ntlm.GetAuthenticateMessage(tsreq.NegoTokens[0].Data)
	if authMsg == nil || ntlmSec == nil {
		return fmt.Errorf("rdp nla: failed to build NTLM Authenticate")
	}
	t.ntlmSec = ntlmSec

	var pubValue []byte
	var sendNonce []byte
	if effective >= nla.CredSSPVersion5 {
		pubValue = nla.ComputePubKeyHash(true, nonce, pubkey)
		sendNonce = nonce
	} else {
		pubValue = pubkey
	}
	encrypted := ntlmSec.GssEncrypt(pubValue)
	req := nla.EncodeDERTRequest(effective, []nla.Message{authMsg}, nil, encrypted, sendNonce)
	_, err = t.Conn.Write(req)
	if err != nil {
		glog.Info("send AuthenticateMessage", err)
		return fmt.Errorf("rdp nla: send authenticate: %w", err)
	}

	next, err := nla.ReadTSRequest(t.Conn)
	if err != nil {
		// Windows 7/2008 and some 2012 boxes just drop the TLS session
		// on a failed NTLM Authenticate instead of sending errorCode.
		if isNLAAuthDrop(err) {
			return fmt.Errorf("rdp nla: authentication failed: %w", err)
		}
		glog.Error("Read:", err)
		return fmt.Errorf("rdp nla: read pubkey: %w", err)
	}
	glog.Debug("recvChallenge Read success")
	return t.recvPubKeyInc(next, effective)
}

func (t *TPKT) recvPubKeyInc(tsreq *nla.TSRequest, effective int) error {
	if err := tsreq.AuthError(); err != nil {
		return err
	}
	glog.Debug("PubKeyAuth:", tsreq.PubKeyAuth)

	domain, username, password := t.ntlm.GetEncodedCredentials()
	credentials := nla.EncodeDERTCredentials(domain, username, password)
	authInfo := t.ntlmSec.GssEncrypt(credentials)
	req := nla.EncodeDERTRequest(effective, nil, authInfo, nil, nil)
	_, err := t.Conn.Write(req)
	if err != nil {
		glog.Info("send AuthenticateMessage", err)
		return fmt.Errorf("rdp nla: send authInfo: %w", err)
	}
	return nil
}

func (t *TPKT) Read(b []byte) (n int, err error) {
	return t.Conn.Read(b)
}

func (t *TPKT) Write(data []byte) (n int, err error) {
	buff := &bytes.Buffer{}
	core.WriteUInt8(FASTPATH_ACTION_X224, buff)
	core.WriteUInt8(0, buff)
	core.WriteUInt16BE(uint16(len(data)+4), buff)
	buff.Write(data)
	glog.Debug("tpkt Write", hex.EncodeToString(buff.Bytes()))
	return t.Conn.Write(buff.Bytes())
}

func (t *TPKT) Close() error {
	return t.Conn.Close()
}

func (t *TPKT) SetFastPathListener(f core.FastPathListener) {
	t.fastPathListener = f
}

func (t *TPKT) SendFastPath(secFlag byte, data []byte) (n int, err error) {
	buff := &bytes.Buffer{}
	core.WriteUInt8(FASTPATH_ACTION_FASTPATH|((secFlag&0x3)<<6), buff)
	core.WriteUInt16BE(uint16(len(data)+3)|0x8000, buff)
	buff.Write(data)
	glog.Debug("TPTK SendFastPath", hex.EncodeToString(buff.Bytes()))
	return t.Conn.Write(buff.Bytes())
}

func (t *TPKT) recvHeader(s []byte, err error) {
	glog.Debug("tpkt recvHeader", hex.EncodeToString(s), err)
	if err != nil {
		t.Emit("error", err)
		return
	}
	r := bytes.NewReader(s)
	version, _ := core.ReadUInt8(r)
	if version == FASTPATH_ACTION_X224 {
		glog.Debug("tptk recvHeader FASTPATH_ACTION_X224, wait for recvExtendedHeader")
		core.StartReadBytes(2, t.Conn, t.recvExtendedHeader)
	} else {
		t.secFlag = (version >> 6) & 0x3
		length, _ := core.ReadUInt8(r)
		t.lastShortLength = int(length)
		if t.lastShortLength&0x80 != 0 {
			core.StartReadBytes(1, t.Conn, t.recvExtendedFastPathHeader)
		} else {
			// fastpath 长度来自对端首字节；<2 说明对端不是 RDP（畸形
			// banner），按协议错误处理而非 makeslice panic。
			if t.lastShortLength < 2 {
				t.Emit("error", errNonRDPPeer)
				return
			}
			core.StartReadBytes(t.lastShortLength-2, t.Conn, t.recvFastPath)
		}
	}
}

func (t *TPKT) recvExtendedHeader(s []byte, err error) {
	glog.Debug("tpkt recvExtendedHeader", hex.EncodeToString(s), err)
	if err != nil {
		return
	}
	r := bytes.NewReader(s)
	size, _ := core.ReadUint16BE(r)
	glog.Debug("tpkt wait recvData:", size)
	if size < 4 {
		t.Emit("error", errNonRDPPeer)
		return
	}
	core.StartReadBytes(int(size-4), t.Conn, t.recvData)
}

func (t *TPKT) recvData(s []byte, err error) {
	glog.Debug("tpkt recvData", hex.EncodeToString(s), err)
	if err != nil {
		return
	}
	t.Emit("data", s)
	core.StartReadBytes(2, t.Conn, t.recvHeader)
}

func (t *TPKT) recvExtendedFastPathHeader(s []byte, err error) {
	glog.Debug("tpkt recvExtendedFastPathHeader", hex.EncodeToString(s))
	r := bytes.NewReader(s)
	rightPart, err := core.ReadUInt8(r)
	if err != nil {
		glog.Error("TPTK recvExtendedFastPathHeader", err)
		return
	}

	leftPart := t.lastShortLength & ^0x80
	packetSize := (leftPart << 8) + int(rightPart)
	core.StartReadBytes(packetSize-3, t.Conn, t.recvFastPath)
}

func (t *TPKT) recvFastPath(s []byte, err error) {
	glog.Debug("tpkt recvFastPath")
	if err != nil {
		return
	}

	t.fastPathListener.RecvFastPath(t.secFlag, s)
	core.StartReadBytes(2, t.Conn, t.recvHeader)
}
