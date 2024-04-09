package msrdp

import (
	"bytes"
	"crypto/md5"
	"crypto/rc4"
	"crypto/rsa"
	"crypto/tls"
	"encoding/asn1"
	"encoding/binary"
	"github.com/huin/asn1ber"
	"github.com/lunixbochs/struc"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/protocol-impl"
	utils2 "github.com/yaklang/yaklang/common/bin-parser/utils"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"os"
	"time"
)

func Login(host string, port int, domainRaw string, userRaw string, passwordRaw string) (bool, error) {
	domain := toWindowsString(domainRaw)
	user := toWindowsString(userRaw)
	password := toWindowsString(passwordRaw)

	conn, err := netx.DialTimeout(3*time.Second, utils.HostPort(host, port))
	if err != nil {
		return false, err
	}
	rdpNegReq := &Negotiation{TYPE_RDP_NEG_REQ, 0, 0x0008, PROTOCOL_RDP | PROTOCOL_SSL | PROTOCOL_HYBRID | PROTOCOL_RDSAAD}
	buf := &bytes.Buffer{}
	struc.Pack(buf, rdpNegReq)
	connReq := GenX224(TPDU_CONNECTION_REQUEST, buf.Bytes())
	tkptPacket := protocol_impl.NewTpktPacket(connReq)
	req, err := tkptPacket.Marshal()
	if err != nil {
		return false, err
	}
	conn.Write(req)
	pkg, err := protocol_impl.ParseTpkt(conn)
	if err != nil {
		return false, err
	}
	node, err := parser.ParseBinary(bytes.NewReader(pkg.TPDU), "application-layer.msrdp", "X224")
	rspX224 := utils2.NodeToData(node).(map[string]any)
	paylaod := rspX224["VariableData"].([]byte)
	rspNeg := &Negotiation{}
	err = struc.Unpack(bytes.NewReader(paylaod), rspNeg)
	if err != nil {
		return false, err
	}
	if rspNeg.Result&PROTOCOL_HYBRID == PROTOCOL_HYBRID {
		file, err := os.OpenFile("/Users/z3/Downloads/gotlskey.log", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			return false, err
		}
		defer file.Close()
		tlsConfig := &tls.Config{
			KeyLogWriter:       file,
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionSSL30,
			MaxVersion:         tls.VersionTLS13,
			Renegotiation:      tls.RenegotiateFreelyAsClient,
		}
		tlsConn := tls.Client(conn, tlsConfig)
		negoMsg := NewNegotiateMessage()
		negoMsg.NegotiateFlags = NTLMSSP_NEGOTIATE_KEY_EXCH |
			NTLMSSP_NEGOTIATE_128 |
			NTLMSSP_NEGOTIATE_EXTENDED_SESSIONSECURITY |
			NTLMSSP_NEGOTIATE_ALWAYS_SIGN |
			NTLMSSP_NEGOTIATE_NTLM |
			NTLMSSP_NEGOTIATE_SEAL |
			NTLMSSP_NEGOTIATE_SIGN |
			NTLMSSP_REQUEST_TARGET |
			NTLMSSP_NEGOTIATE_UNICODE
		tsReq := TSRequest{
			Version:    2,
			NegoTokens: []NegoToken{{Data: negoMsg.Serialize()}},
		}
		payload, err := asn1.Marshal(tsReq)
		if err != nil {
			return false, err
		}
		_, err = tlsConn.Write(payload)
		if err != nil {
			return false, err
		}
		bs := make([]byte, 1024)
		n, err := tlsConn.Read(bs)
		treq := &TSRequest{}
		_, err = asn1.Unmarshal(bs[:n], treq)
		if err != nil {
			return false, err
		}
		reader := bytes.NewReader(treq.NegoTokens[0].Data)
		challengeMsgData, err := ParseRdpSubProtocol(reader, "Challenge")
		if err != nil {
			return false, err
		}

		n, err = reader.Read(bs)
		if err != nil {
			return false, err
		}
		nt, lm := CalcNTLMHash(string(password), string(user), string(domain))
		block := bs[:n]
		challengeMsgDataMap := challengeMsgData.(map[string]any)
		netNt, netLm, key := CalcNetNTLMHash(challengeMsgDataMap, block, nt, lm)
		builder := NewNTLMFieldBuilder()

		AuMessageMap := map[string]any{
			"Signature":                       [8]byte{'N', 'T', 'L', 'M', 'S', 'S', 'P', 0x00},
			"MessageType":                     3,
			"LmChallengeResponseFields":       builder.WriteField(netLm),
			"NtChallengeResponseFields":       builder.WriteField(netNt),
			"DomainNameFields":                builder.WriteField([]byte(domain)),
			"UserNameFields":                  builder.WriteField([]byte(user)),
			"WorkstationFields":               builder.WriteField([]byte("")),
			"EncryptedRandomSessionKeyFields": builder.WriteField(key),
			"NegotiateFlags":                  challengeMsgDataMap["NegotiateFlags"],
			"Version": map[string]any{
				"ProductMajorVersion": 6,
				"ProductMinorVersion": 0,
				"ProductBuild":        6002,
				"Reserved":            [3]byte{},
				"NTLMRevisionCurrent": 0x0F,
			},
			"MIC": [16]byte{},
		}
		auPayload, err := GenRdpSubProtocol(AuMessageMap, "Authentication")
		if err != nil {
			return false, err
		}
		auPayload = append(auPayload, builder.GetPayload()...)

		micBuf := bytes.Buffer{}
		micBuf.Write(negoMsg.Serialize())
		micBuf.Write(treq.NegoTokens[0].Data)
		micBuf.Write(auPayload)
		mic := codec.HmacMD5(key, micBuf.Bytes())[:16]
		AuMessageMap["MIC"] = mic

		auPayload, err = GenRdpSubProtocol(AuMessageMap, "Authentication")
		if err != nil {
			return false, err
		}
		auPayload = append(auPayload, builder.GetPayload()...)

		pub := tlsConn.ConnectionState().PeerCertificates[0].PublicKey.(*rsa.PublicKey)
		cert, err := asn1ber.Marshal(*pub)
		if err != nil {
			return false, err
		}
		var (
			clientSigning = append([]byte("session key to client-to-server signing key magic constant"), 0x00)
			serverSigning = append([]byte("session key to server-to-client signing key magic constant"), 0x00)
			clientSealing = append([]byte("session key to client-to-server sealing key magic constant"), 0x00)
			serverSealing = append([]byte("session key to server-to-client sealing key magic constant"), 0x00)
		)
		exportedSessionKey := []byte(utils.RandStringBytes(16))
		md := md5.New()
		//ClientSigningKey
		a := append(exportedSessionKey, clientSigning...)
		md.Write(a)
		ClientSigningKey := md.Sum(nil)
		//ServerSigningKey
		md.Reset()
		a = append(exportedSessionKey, serverSigning...)
		md.Write(a)
		//ServerSigningKey := md.Sum(nil)
		//ClientSealingKey
		md.Reset()
		a = append(exportedSessionKey, clientSealing...)
		md.Write(a)
		ClientSealingKey := md.Sum(nil)
		//ServerSealingKey
		md.Reset()
		a = append(exportedSessionKey, serverSealing...)
		md.Write(a)
		//ServerSealingKey := md.Sum(nil)
		encryptRC4, _ := rc4.NewCipher(ClientSealingKey)
		//decryptRC4, _ := rc4.NewCipher(ServerSealingKey)

		p := make([]byte, len(cert))
		encryptRC4.XORKeyStream(p, cert)
		b := &bytes.Buffer{}

		//signature
		bs = make([]byte, 4)
		binary.LittleEndian.PutUint32(bs, 0)
		b.Write(bs)
		b.Write(cert)
		s1 := codec.HmacMD5(ClientSigningKey, b.Bytes())[:8]
		checksum := make([]byte, 8)
		encryptRC4.XORKeyStream(checksum, s1)
		b.Reset()

		bs = make([]byte, 4)
		binary.LittleEndian.PutUint32(bs, 0x00000001)
		b.Write(bs)
		b.Write(checksum)
		bs = make([]byte, 4)
		binary.LittleEndian.PutUint32(bs, 0)
		b.Write(bs)
		//b.Write(cert)
		b.Write(p)

		cert = b.Bytes()
		tsReq = TSRequest{
			Version:    2,
			NegoTokens: []NegoToken{{Data: auPayload}},
			PubKeyAuth: cert,
		}
		payload, err = asn1.Marshal(tsReq)
		if err != nil {
			return false, err
		}
		tlsConn.Write(payload)
		resp := make([]byte, 1024)
		n, err = tlsConn.Read(resp)
		if err != nil {
			return false, err
		}
	}
	//tsReqByte,err := asn1.Marshal(tsReq)
	//if err != nil{
	//	return false, err
	//}

	return false, nil
}
func GenX224(flag byte, payload []byte) []byte {
	x224Crq := []byte{byte(6 + len(payload)), flag, 0, 0, 0, 0, 0}
	buf := bytes.Buffer{}
	buf.Write(x224Crq)
	buf.Write(payload)
	return buf.Bytes()
}
