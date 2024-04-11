package msrdp

import (
	"bytes"
	"crypto/md5"
	"crypto/rc4"
	"crypto/rsa"
	"crypto/tls"
	"encoding/asn1"
	"encoding/binary"
	"fmt"
	"github.com/huin/asn1ber"
	"github.com/lunixbochs/struc"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/protocol-impl"
	utils2 "github.com/yaklang/yaklang/common/bin-parser/utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"os"
	"time"
)

func Login(host string, port int, password string, user string, domain string) (bool, error) {
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
		cert, err := tls.LoadX509KeyPair("/Users/z3/yakit-projects/yak-mitm-ca.crt", "/Users/z3/yakit-projects/yak-mitm-ca.key")
		if err != nil {
			log.Fatal(err)
		}
		tlsConfig := &tls.Config{
			Certificates:       []tls.Certificate{cert},
			KeyLogWriter:       file,
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionSSL30,
			MaxVersion:         tls.VersionTLS13,
			Renegotiation:      tls.RenegotiateFreelyAsClient,
		}
		tlsConn := tls.Client(conn, tlsConfig)
		negoMsg := protocol_impl.NewNegotiateMessage()
		signature := [8]byte{'N', 'T', 'L', 'M', 'S', 'S', 'P', 0x00}
		negoMsg.Signature = signature
		negoMsg.MessageType = 0x00000001
		negoMsg.NegotiateFlags = NTLMSSP_NEGOTIATE_KEY_EXCH |
			NTLMSSP_NEGOTIATE_128 |
			NTLMSSP_NEGOTIATE_EXTENDED_SESSIONSECURITY |
			NTLMSSP_NEGOTIATE_ALWAYS_SIGN |
			NTLMSSP_NEGOTIATE_NTLM |
			NTLMSSP_NEGOTIATE_SEAL |
			NTLMSSP_NEGOTIATE_SIGN |
			NTLMSSP_REQUEST_TARGET |
			NTLMSSP_NEGOTIATE_UNICODE
		negPayload, err := negoMsg.Marshal()
		if err != nil {
			return false, err
		}
		tsReq := TSRequest{
			Version:    2,
			NegoTokens: []NegoToken{{Data: negPayload}},
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
		//challengeData, _ := codec.DecodeHex("4e544c4d53535000020000001e001e003800000035828a6261b162cdad9087fc000000000000000098009800560000000a0039380000000f69005a003700770034006e00310069006f0075006d003600340035005a0002001e0069005a003700770034006e00310069006f0075006d003600340035005a0001001e0069005a003700770034006e00310069006f0075006d003600340035005a0004001e0069005a003700770034006e00310069006f0075006d003600340035005a0003001e0069005a003700770034006e00310069006f0075006d003600340035005a0007000800d9f5b97edd8bda0100000000")
		//challengeMsg, err := protocol_impl.ParseChallengeMessage(challengeData)
		//challengeDataBytes, _ := challengeMsg.Marshal()
		//println(codec.EncodeToHex(challengeDataBytes))
		challengeMsg, err := protocol_impl.ParseChallengeMessage(treq.NegoTokens[0].Data)
		if err != nil {
			return false, err
		}
		nt := protocol_impl.NTOWFv2(password, user, domain)
		lm := protocol_impl.LMOWFv2(password, user, domain)
		//clientChallenge := []byte(utils.RandStringBytes(8))
		clientChallenge := []byte("12345678")
		serverInfo := challengeMsg.TargetInfoFields.Value()
		pairs := protocol_impl.ParseAVPAIRs(serverInfo)
		var serverName, timestamp []byte
		_ = serverName
		for _, pair := range pairs {
			if pair.AvId == 0x0007 {
				timestamp = pair.Value
			}
		}
		//clientChallenge, _ = codec.DecodeHex("594d646174713673")
		//serverChallenge, _ := codec.DecodeHex("ee1ac04dce23620f")
		netNt, netLm, sessionBaseKey := protocol_impl.NetNTLMv2(nt, lm, challengeMsg.ServerChallenge[:], clientChallenge, timestamp, serverInfo)
		//netNt, netLm, sessionBaseKey := protocol_impl.NetNTLMv2(nt, lm, serverChallenge, clientChallenge, timestamp, serverInfo)
		exportedSessionKey := []byte("1234567812345678")
		EncryptedRandomSessionKey := make([]byte, len(exportedSessionKey))
		rc, _ := rc4.NewCipher(sessionBaseKey)
		rc.XORKeyStream(EncryptedRandomSessionKey, exportedSessionKey)

		authMsg := protocol_impl.NewAuthenticationMessage()
		authMsg.Signature = signature
		authMsg.MessageType = 3
		authMsg.LmChallengeResponseFields = authMsg.NewField(netLm)
		authMsg.NtChallengeResponseFields = authMsg.NewField(netNt)
		authMsg.DomainNameFields = authMsg.NewField(protocol_impl.UnicodeEncode(domain))
		authMsg.UserNameFields = authMsg.NewField(protocol_impl.UnicodeEncode(user))
		authMsg.WorkstationFields = authMsg.NewField(protocol_impl.UnicodeEncode(""))
		authMsg.EncryptedRandomSessionKeyFields = authMsg.NewField(EncryptedRandomSessionKey)
		bsFlag := make([]byte, 4)
		binary.LittleEndian.PutUint32(bsFlag, challengeMsg.NegotiateFlags)
		for i := 0; i < len(authMsg.NegotiateFlags); i++ {
			authMsg.NegotiateFlags[i] = bsFlag[i]
		}
		authMsg.Version = protocol_impl.Version{
			ProductMajorVersion: 6,
			ProductMinorVersion: 0,
			ProductBuild:        6002,
			Reserved:            [3]byte{},
			NTLMRevisionCurrent: 0x0F,
		}
		challengePayload, err := challengeMsg.Marshal()
		if err != nil {
			return false, err
		}
		authPayload, err := authMsg.Marshal()
		if err != nil {
			return false, err
		}
		fmt.Printf("au1: %s", codec.EncodeToHex(authPayload))
		micBuf := bytes.Buffer{}
		micBuf.Write(negPayload)
		fmt.Printf("rawNt: %s\n", codec.EncodeToHex(nt))
		fmt.Printf("rawLm: %s\n", codec.EncodeToHex(lm))
		fmt.Printf("timestamp: %s\n", codec.EncodeToHex(timestamp))
		fmt.Printf("clientChallenge: %s\n", codec.EncodeToHex(clientChallenge))
		fmt.Printf("serverChallenge: %s\n", codec.EncodeToHex(challengeMsg.ServerChallenge[:]))
		fmt.Printf("nt: %s\n", codec.EncodeToHex(netNt))
		fmt.Printf("lm: %s\n", codec.EncodeToHex(netLm))
		fmt.Printf("key: %s\n", codec.EncodeToHex(sessionBaseKey))
		fmt.Printf("serverInfo: %s\n", codec.EncodeToHex(serverInfo))

		fmt.Printf("negPayload: %s\n", codec.EncodeToHex(negPayload))
		fmt.Printf("challenge: %s\n", codec.EncodeToHex(challengePayload))
		fmt.Printf("authPayload: %s\n", codec.EncodeToHex(authPayload))
		fmt.Printf("key: %s\n", codec.EncodeToHex(sessionBaseKey))

		micBuf.Write(challengePayload)
		micBuf.Write(authPayload)
		mic := codec.HmacMD5(sessionBaseKey, micBuf.Bytes())[:16]
		for i := 0; i < 16; i++ {
			authMsg.MIC[i] = mic[i]
		}
		fmt.Printf("mic: %s\n", codec.EncodeToHex(authMsg.MIC[:]))
		authPayload, err = authMsg.Marshal()
		if err != nil {
			return false, err
		}

		pub := tlsConn.ConnectionState().PeerCertificates[0].PublicKey.(*rsa.PublicKey)
		certContent, err := asn1ber.Marshal(*pub)

		if err != nil {
			return false, err
		}
		var (
			clientSigning = append([]byte("session key to client-to-server signing key magic constant"), 0x00)
			//serverSigning = append([]byte("session key to server-to-client signing key magic constant"), 0x00)
			clientSealing = append([]byte("session key to client-to-server sealing key magic constant"), 0x00)
			//serverSealing = append([]byte("session key to server-to-client sealing key magic constant"), 0x00)
		)

		//exportedSessionKey := []byte(utils.RandStringBytes(16))

		md5Enc := func(i any) []byte {
			res := md5.Sum(utils.InterfaceToBytes(i))
			return res[:]
		}
		ClientSigningKey := md5Enc(append(exportedSessionKey, clientSigning...))
		ClientSealingKey := md5Enc(append(exportedSessionKey, clientSealing...))
		//ServerSigningKey := md5Enc(append(exportedSessionKey, serverSigning...))
		//ServerSealingKey := md5Enc(append(exportedSessionKey, serverSealing...))

		encryptRC4, _ := rc4.NewCipher(ClientSealingKey)
		//decryptRC4, _ := rc4.NewCipher(ServerSealingKey)

		p := make([]byte, len(certContent))
		encryptRC4.XORKeyStream(p, certContent)
		b := &bytes.Buffer{}

		//signature
		bs = make([]byte, 4)
		binary.LittleEndian.PutUint32(bs, 0)
		b.Write(bs)
		b.Write(certContent)
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

		certContent = b.Bytes()
		fmt.Printf("certEncrypt: %s\n", codec.EncodeToHex(certContent))
		tsReq = TSRequest{
			Version:    2,
			NegoTokens: []NegoToken{{Data: authPayload}},
			PubKeyAuth: certContent,
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
		return true, nil
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
