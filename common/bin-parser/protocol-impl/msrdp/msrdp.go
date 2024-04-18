package msrdp

import (
	"bytes"
	"crypto/md5"
	"crypto/rc4"
	"crypto/rsa"
	"crypto/tls"
	"encoding/asn1"
	"encoding/binary"
	"errors"
	"github.com/huin/asn1ber"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/protocol-impl"
	utils2 "github.com/yaklang/yaklang/common/bin-parser/utils"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"net"
	"time"
)

type RDPClient struct {
	rawTlsConn *tls.Conn
	conn       net.Conn
	Host       string
	Port       int
	security   GssSecurity
}

func NewRDPClient(addr string) (*RDPClient, error) {
	host, port, err := utils.ParseStringToHostPort(addr)
	if err != nil {
		return nil, err
	}
	return &RDPClient{
		Host: host,
		Port: port,
	}, nil
}
func (r *RDPClient) Login(domain, user, password string) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.Errorf("login to %s:%d failed: %v", r.Host, r.Port, e)
		}
	}()
	protocol, err := r.Connect()
	if err != nil {
		return err
	}

	isSet := func(flag uint32) bool {
		return protocol&flag == flag
	}
	if isSet(PROTOCOL_HYBRID_EX) { // CredSSP 认证的基础上加了个 `Early User Authorization Result PDU` 也就是认证结果的确认
		_, err := r.CsspAuthenticate(domain, user, password)
		if err != nil {
			return err
		}
		bs := make([]byte, 1024)
		n, err := r.Read(bs)
		if err != nil {
			return err
		}
		packet := protocol_impl.NewTpktPacket(bs[:n])
		var AUTHZ_SUCCESS uint32 = 0x00000000
		//var AUTHZ_ACCESS_DENIED uint32 = 0x00000005
		result := binary.LittleEndian.Uint32(packet.TPDU)
		if result == AUTHZ_SUCCESS {
			return nil
		} else {
			return errors.New("access denied")
		}
	} else if isSet(PROTOCOL_HYBRID) { // CredSSP 认证
		_, err := r.CsspAuthenticate(domain, user, password)
		if err != nil {
			return err
		}
	}
	if isSet(PROTOCOL_RDP) { // 桌面登录
	}
	if isSet(PROTOCOL_SSL) { // 使用SSL通信的桌面登录

	}
	if isSet(PROTOCOL_RDSTLS) { // PROTOCOL_RDP增强
	}
	if isSet(PROTOCOL_RDSAAD) { // PROTOCOL_RDSTLS增强

	}
	return utils.Errorf("unsupported protocol: %d", protocol)
}
func (r *RDPClient) StartTLS() error {
	tlsConn := utils.NewDefaultTLSClient(r.conn)
	r.rawTlsConn = tlsConn
	r.conn = tlsConn
	return nil
}
func (r *RDPClient) Read(d []byte) (int, error) {
	return r.conn.Read(d)
}
func (r *RDPClient) Write(d []byte) (int, error) {
	return r.conn.Write(d)
}
func (r *RDPClient) CsspAuthenticate(domain, user, password string) (bool, error) {
	// use tls to connect
	err := r.StartTLS()
	if err != nil {
		return false, err
	}
	// send negotiate message
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
	_, err = r.Write(payload)
	if err != nil {
		return false, err
	}
	// receive challenge message
	bs := make([]byte, 1024)
	n, err := r.Read(bs)
	if err != nil {
		return false, err
	}
	treq := &TSRequest{}
	_, err = asn1.Unmarshal(bs[:n], treq)
	if err != nil {
		return false, err
	}
	challengeMsg, err := protocol_impl.ParseChallengeMessage(treq.NegoTokens[0].Data)
	if err != nil {
		return false, err
	}
	// calc nt and lm (v2)
	nt := protocol_impl.NTOWFv2(password, user, domain)
	lm := protocol_impl.LMOWFv2(password, user, domain)
	clientChallenge := []byte(utils.RandStringBytes(8))
	serverInfo := challengeMsg.TargetInfoFields.Value()
	pairs := protocol_impl.ParseAVPAIRs(serverInfo)
	var serverName, timestamp []byte
	_ = serverName
	for _, pair := range pairs {
		if pair.AvId == 0x0007 {
			timestamp = pair.Value
		}
	}
	// calc net ntlm hash and session base key
	netNt, netLm, sessionBaseKey := protocol_impl.NetNTLMv2(nt, lm, challengeMsg.ServerChallenge[:], clientChallenge, timestamp, serverInfo)

	// client rand a session key, use session base key xor it to send to server
	exportedSessionKey := []byte(utils.RandStringBytes(16))
	encryptedRandomSessionKey := make([]byte, len(exportedSessionKey))
	rc, _ := rc4.NewCipher(sessionBaseKey)
	rc.XORKeyStream(encryptedRandomSessionKey, exportedSessionKey)

	// generate auth message
	authMsg := protocol_impl.NewAuthenticationMessage()
	authMsg.Signature = signature
	authMsg.MessageType = 3
	authMsg.LmChallengeResponseFields = authMsg.NewField(netLm)
	authMsg.NtChallengeResponseFields = authMsg.NewField(netNt)
	authMsg.DomainNameFields = authMsg.NewField(protocol_impl.UnicodeEncode(domain))
	authMsg.UserNameFields = authMsg.NewField(protocol_impl.UnicodeEncode(user))
	authMsg.WorkstationFields = authMsg.NewField(protocol_impl.UnicodeEncode(""))
	authMsg.EncryptedRandomSessionKeyFields = authMsg.NewField(encryptedRandomSessionKey)
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
	micBuf := bytes.Buffer{}
	micBuf.Write(negPayload)
	micBuf.Write(challengePayload)
	micBuf.Write(authPayload)
	mic := codec.HmacMD5(sessionBaseKey, micBuf.Bytes())[:16]
	for i := 0; i < 16; i++ {
		authMsg.MIC[i] = mic[i]
	}
	authPayload, err = authMsg.Marshal()
	if err != nil {
		return false, err
	}

	pub := r.rawTlsConn.ConnectionState().PeerCertificates[0].PublicKey.(*rsa.PublicKey)
	certContent, err := asn1ber.Marshal(*pub)

	if err != nil {
		return false, err
	}

	// gen session encrypt and decrypt util by client session key
	security := NewNTLMv2Security(exportedSessionKey)
	certContent = security.GssEncrypt(certContent)
	tsReq = TSRequest{
		Version:    2,
		NegoTokens: []NegoToken{{Data: authPayload}},
		PubKeyAuth: certContent,
	}
	payload, err = asn1.Marshal(tsReq)
	if err != nil {
		return false, err
	}
	r.Write(payload)
	// receive pub key
	resp := make([]byte, 1024)
	n, err = r.Read(resp)
	if err != nil {
		return false, err
	}
	serverKeyHash := resp[:n]
	_ = serverKeyHash
	passwordCredsPaylaod, err := asn1.Marshal(TSPasswordCreds{
		DomainName: protocol_impl.UnicodeEncode(domain),
		UserName:   protocol_impl.UnicodeEncode(user),
		Password:   protocol_impl.UnicodeEncode(password),
	})
	if err != nil {
		return false, err
	}
	credsPayload, err := asn1.Marshal(TSCredentials{
		CredType:    1,
		Credentials: passwordCredsPaylaod,
	})
	if err != nil {
		return false, err
	}
	credsPayloadEnced := security.GssEncrypt(credsPayload)
	tsReq = TSRequest{
		Version:  2,
		AuthInfo: credsPayloadEnced,
	}
	payload, err = asn1.Marshal(tsReq)
	if err != nil {
		return false, err
	}
	// send creds
	r.Write(payload)
	return true, nil
}
func (r *RDPClient) BuildDataByProtocol(data map[string]any, protocol string) ([]byte, error) {
	node, err := parser.GenerateBinary(data, "application-layer.msrdp", protocol)
	if err != nil {
		return nil, err
	}
	d := utils2.NodeToBytes(node)
	return d, nil
}
func (r *RDPClient) ParseProtocol(reader io.Reader, protocol string) (map[string]any, error) {
	node, err := parser.ParseBinary(reader, "application-layer.msrdp", protocol)
	if err != nil {
		return nil, utils.Errorf("parse protocol `%s` from reader failed: %v", protocol, err)
	}
	d := utils2.NodeToData(node)
	if d != nil {
		if v, ok := d.(map[string]any); ok {
			return v, nil
		}
	}
	return nil, errors.New("invalid data")
}
func (r *RDPClient) Connect() (protocol uint32, e error) {
	defer func() {
		if err := recover(); err != nil {
			e = utils.Errorf("send connect request to %s:%d failed: %v", r.Host, r.Port, err)
		}
	}()
	conn, err := netx.DialTimeout(3*time.Second, utils.HostPort(r.Host, r.Port))
	if err != nil {
		return 0, err
	}
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	r.conn = conn
	negotiationMsg, err := r.BuildDataByProtocol(map[string]any{
		"Type":     TYPE_RDP_NEG_REQ,
		"Flag":     0,
		"Length":   0x0008,
		"Protocol": PROTOCOL_RDP | PROTOCOL_SSL | PROTOCOL_HYBRID | PROTOCOL_HYBRID_EX,
	}, "Negotiation")
	if err != nil {
		return 0, err
	}

	connReq, err := r.BuildDataByProtocol(map[string]any{
		"Length":       6 + len(negotiationMsg),
		"Flag":         TPDU_CONNECTION_REQUEST,
		"Destination":  0,
		"Source":       0,
		"Class":        0,
		"VariableData": negotiationMsg,
	}, "X224")
	if err != nil {
		return 0, err
	}

	tkptPacket := protocol_impl.NewTpktPacket(connReq)
	req, err := tkptPacket.Marshal()
	if err != nil {
		return 0, err
	}
	conn.Write(req)
	pkg, err := protocol_impl.ParseTpkt(conn)
	if err != nil {
		return 0, err
	}
	rspX224, err := r.ParseProtocol(bytes.NewReader(pkg.TPDU), "X224")
	if err != nil {
		return 0, err
	}
	rspNeg, err := r.ParseProtocol(bytes.NewReader(rspX224["VariableData"].([]byte)), "Negotiation")
	if err != nil {
		return 0, err
	}
	protocol = rspNeg["Protocol"].(uint32)
	return
}

type GssSecurity interface {
	GssEncrypt([]byte) []byte
	GssDecrypt([]byte) []byte
}
type NTLMv2Security struct {
	clientSealing    *rc4.Cipher
	serverSealing    *rc4.Cipher
	clientSigningKey []byte
	serverSigningKey []byte
	SeqNum           uint32
}

func (n *NTLMv2Security) GssEncrypt(data []byte) []byte {
	defer func() {
		n.SeqNum++
	}()
	p := make([]byte, len(data))
	n.clientSealing.XORKeyStream(p, data)
	b := &bytes.Buffer{}

	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, n.SeqNum)
	b.Write(bs)
	b.Write(data)
	s1 := codec.HmacMD5(n.clientSigningKey, b.Bytes())[:8]
	checksum := make([]byte, 8)
	n.clientSealing.XORKeyStream(checksum, s1)
	b.Reset()

	bs = make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, 0x00000001)
	b.Write(bs)
	b.Write(checksum)
	bs = make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, n.SeqNum)
	b.Write(bs)
	//b.Write(cert)
	b.Write(p)
	return b.Bytes()
}
func (n *NTLMv2Security) GssDecrypt(data []byte) []byte {
	res := make([]byte, len(data))
	n.serverSealing.XORKeyStream(res, data)
	return res
}
func NewNTLMv2Security(sessionKey []byte) *NTLMv2Security {
	var (
		clientSigning = append([]byte("session key to client-to-server signing key magic constant"), 0x00)
		serverSigning = append([]byte("session key to server-to-client signing key magic constant"), 0x00)
		clientSealing = append([]byte("session key to client-to-server sealing key magic constant"), 0x00)
		serverSealing = append([]byte("session key to server-to-client sealing key magic constant"), 0x00)
	)
	md5Enc := func(i any) []byte {
		res := md5.Sum(utils.InterfaceToBytes(i))
		return res[:]
	}
	clientEnc, _ := rc4.NewCipher(md5Enc(append(sessionKey, clientSealing...)))
	serverEnc, _ := rc4.NewCipher(md5Enc(append(sessionKey, serverSealing...)))
	sec := &NTLMv2Security{
		clientSigningKey: md5Enc(append(sessionKey, clientSigning...)),
		serverSigningKey: md5Enc(append(sessionKey, serverSigning...)),
		clientSealing:    clientEnc,
		serverSealing:    serverEnc,
	}
	return sec
}
