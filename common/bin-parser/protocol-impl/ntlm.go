package protocol_impl

import (
	"bytes"
	"errors"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	utils2 "github.com/yaklang/yaklang/common/bin-parser/utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"golang.org/x/crypto/md4"
	"golang.org/x/text/encoding/unicode"
	"io"
	"strings"
)

type AVPAIR struct {
	AvId  uint16
	AvLen uint16
	Value []byte
}

func ParseAVPAIRs(data []byte) (res []*AVPAIR) {
	reader := bytes.NewReader(data)
	for {
		v := &AVPAIR{}
		err := ParseSubProtocol(reader, v, "AV_PAIR")
		if err != nil {
			panic(err)
		}
		if v.AvId == 0 {
			return
		}
		res = append(res, v)
	}
}

type NegotiateMessage struct {
	Signature         [8]byte
	MessageType       uint32
	NegotiateFlags    uint32
	DomainNameFields  Field
	WorkstationFields Field
	Version           Version
	payload           *[]byte
}

func NewNegotiateMessage() *NegotiateMessage {
	res := &NegotiateMessage{}
	res.setPayload([]byte{})
	return res
}
func (n *NegotiateMessage) setPayload(payload []byte) {
	n.payload = &payload
	n.DomainNameFields.payload = n.payload
	n.DomainNameFields.base = 40
	n.WorkstationFields.payload = n.payload
	n.WorkstationFields.base = 40
}
func (n *NegotiateMessage) NewField(d []byte) Field {
	payload := *n.payload
	offset := len(payload) + 40
	f := Field{
		BufferOffset: uint32(offset),
		Length:       uint16(len(d)),
		MaxLength:    uint16(len(d)),
		payload:      n.payload,
		base:         40,
	}
	payload = append(payload, d...)
	*n.payload = payload
	return f
}
func ParseNegotiateMessage(data []byte) (*NegotiateMessage, error) {
	res := &NegotiateMessage{}
	reader := bytes.NewReader(data)
	err := ParseSubProtocol(reader, res, "NegotiateMessage")
	if err != nil {
		return nil, err
	}
	res.setPayload(data[40:])
	return res, nil
}

type ChallengeMessage struct {
	Signature        [8]byte
	MessageType      uint32
	TargetNameFields Field
	NegotiateFlags   uint32
	ServerChallenge  [8]byte
	Reserved         [8]byte
	TargetInfoFields Field
	Version          Version
	payload          *[]byte
}

func NewChallengeMessage() *ChallengeMessage {
	res := &ChallengeMessage{}
	res.SetPayload([]byte{})
	return res
}
func (n *ChallengeMessage) SetPayload(payload []byte) {
	n.payload = &payload
	n.TargetNameFields.payload = n.payload
	n.TargetNameFields.base = 56
	n.TargetInfoFields.payload = n.payload
	n.TargetInfoFields.base = 56
}
func (n *ChallengeMessage) NewField(d []byte) Field {
	payload := *n.payload
	offset := len(payload) + 56
	f := Field{
		BufferOffset: uint32(offset),
		Length:       uint16(len(d)),
		MaxLength:    uint16(len(d)),
		payload:      n.payload,
		base:         56,
	}
	payload = append(payload, d...)
	*n.payload = payload
	return f
}
func ParseChallengeMessage(data []byte) (*ChallengeMessage, error) {
	res := &ChallengeMessage{}
	reader := bytes.NewReader(data)
	err := ParseSubProtocol(reader, res, "ChallengeMessage")
	if err != nil {
		return nil, err
	}
	res.SetPayload(data[56:])
	return res, nil
}

type AuthenticationMessage struct {
	Signature                       [8]byte
	MessageType                     uint32
	LmChallengeResponseFields       Field
	NtChallengeResponseFields       Field
	DomainNameFields                Field
	UserNameFields                  Field
	WorkstationFields               Field
	EncryptedRandomSessionKeyFields Field
	NegotiateFlags                  [4]byte
	Version                         Version
	MIC                             [16]byte
	payload                         *[]byte
}

func NewAuthenticationMessage() *AuthenticationMessage {
	res := &AuthenticationMessage{}
	res.SetPayload([]byte{})
	return res
}
func (n *AuthenticationMessage) SetPayload(payload []byte) {
	n.payload = &payload
	fields := []*Field{&n.LmChallengeResponseFields, &n.DomainNameFields, &n.UserNameFields, &n.WorkstationFields, &n.NtChallengeResponseFields, &n.EncryptedRandomSessionKeyFields}
	for _, field := range fields {
		field.payload = n.payload
		field.base = 88
	}
}
func (n *AuthenticationMessage) NewField(d []byte) Field {
	payload := *n.payload
	offset := len(payload) + 88
	f := Field{
		BufferOffset: uint32(offset),
		Length:       uint16(len(d)),
		MaxLength:    uint16(len(d)),
		payload:      n.payload,
		base:         88,
	}
	payload = append(payload, d...)
	*n.payload = payload
	return f
}
func ParseAuthenticationMessage(data []byte) (*AuthenticationMessage, error) {
	res := &AuthenticationMessage{}
	reader := bytes.NewReader(data)
	err := ParseSubProtocol(reader, res, "AuthenticationMessage")
	if err != nil {
		return nil, err
	}
	res.SetPayload(data[88:])
	return res, nil
}

type Version struct {
	ProductMajorVersion uint8
	ProductMinorVersion uint8
	ProductBuild        uint16
	Reserved            [3]byte
	NTLMRevisionCurrent uint8
}
type Field struct {
	Length       uint16
	MaxLength    uint16
	BufferOffset uint32
	payload      *[]byte
	base         uint32
}

func (f Field) Value() []byte {
	offset := f.BufferOffset - f.base
	return (*f.payload)[offset : offset+uint32(f.Length)]
}
func (n *NegotiateMessage) Marshal() ([]byte, error) {
	res, err := GenSubProtocol(n, "NegotiateMessage")
	if err != nil {
		return nil, err
	}
	res = append(res, *n.payload...)
	return res, nil
}
func (n *ChallengeMessage) Marshal() ([]byte, error) {
	res, err := GenSubProtocol(n, "ChallengeMessage")
	if err != nil {
		return nil, err
	}
	res = append(res, *n.payload...)
	return res, nil
}
func (n *AuthenticationMessage) Marshal() ([]byte, error) {
	res, err := GenSubProtocol(n, "AuthenticationMessage")
	if err != nil {
		return nil, err
	}
	res = append(res, *n.payload...)
	return res, nil
}
func ParseSubProtocol(reader io.Reader, v any, key string) error {
	node, err := parser.ParseBinary(reader, "application-layer.ntlm", key)
	if err != nil {
		return err
	}
	err = utils2.NodeToStruct(node, v)
	if err != nil {
		return err
	}
	return nil
}
func GenSubProtocol(data any, key string) ([]byte, error) {
	node, err := parser.GenerateBinary(data, "application-layer.ntlm", key)
	if err != nil {
		return nil, err
	}
	d := utils2.NodeToBytes(node)
	return d, nil
}
func UnicodeEncode(p string) []byte {
	res, err := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder().Bytes([]byte(p))
	if err != nil {
		log.Errorf("unicode encode error: %v", err)
	}
	return res
}
func NTOWFv2(Passwd, User, UserDom string) []byte {
	return codec.HmacMD5(md4Encode(Passwd), UnicodeEncode(strings.ToUpper(User)+UserDom))
}
func LMOWFv2(Passwd, User, UserDom string) []byte {
	return NTOWFv2(Passwd, User, UserDom)
}
func NTOWFv1(Passwd, User, UserDom string) []byte {
	return md4Encode(Passwd)
}
func LMOWFv1(Passwd, User, UserDom string) []byte {
	if len(Passwd) < 14 {
		Passwd += string(make([]byte, 14-len(Passwd)))
	}
	data := []byte("KGS!@#$%")
	key1, err := createDESKey([]byte(strings.ToUpper(Passwd)[:7]))
	if err != nil {
		log.Errorf("create des encrypt key error: %v", err)
	}
	key2, err := createDESKey([]byte(strings.ToUpper(Passwd)[7:]))
	if err != nil {
		log.Errorf("create des encrypt key error: %v", err)
	}
	res1 := desEncode(key1, data)
	res2 := desEncode(key2, data)
	return append(res1, res2...)
}
func createDESKey(b []byte) ([]byte, error) {
	if len(b) < 7 {
		return nil, errors.New("need at least 7 bytes")
	}

	key := make([]byte, 8)

	key[0] = b[0]
	key[1] = b[0]<<7 | b[1]>>1
	key[2] = b[1]<<6 | b[2]>>2
	key[3] = b[2]<<5 | b[3]>>3
	key[4] = b[3]<<4 | b[4]>>4
	key[5] = b[4]<<3 | b[5]>>5
	key[6] = b[5]<<2 | b[6]>>6
	key[7] = b[6] << 1

	for i, x := range key {
		key[i] = (x & 0xfe) | ((((x >> 1) ^ (x >> 2) ^ (x >> 3) ^ (x >> 4) ^ (x >> 5) ^ (x >> 6) ^ (x >> 7)) ^ 0x01) & 0x01)
	}

	return key, nil
}

// desEncode Indicates the encryption of an 8-byte data item D with the 7-byte key K using the Data Encryption Standard (DES) algorithm in Electronic Codebook (ECB) mode
func desEncode(k, d []byte) []byte {
	res, err := codec.DESECBEnc(k, d)
	if err != nil {
		log.Errorf("desc encrypt error: %v", err)
	}
	return res
}
func md4Encode(d string) []byte {
	en := md4.New()
	en.Write(UnicodeEncode(d))
	return en.Sum(nil)
}

// desl Indicates the encryption of an 8-byte data item D with the 16-byte key K using the Data Encryption Standard Long (DESL) algorithm
func desl(k, d []byte) []byte {
	res := bytes.Buffer{}
	res.Write(desEncode(k[:7], d))
	res.Write(desEncode(k[7:14], d))
	res.Write(desEncode(append(k[14:16], make([]byte, 5)...), d))
	return res.Bytes()
}

var NoLMResponseNTLMv1 bool

func NetNTLMv2(nt []byte, lm []byte, serverChallenge []byte, clientChallenge []byte, time []byte, ServerName []byte) ([]byte, []byte, []byte) {
	buf := bytes.Buffer{}
	buf.Write([]byte{0x01, 0x01})
	buf.Write(make([]byte, 6))
	buf.Write(time)
	buf.Write(clientChallenge)
	buf.Write(make([]byte, 4))
	buf.Write(ServerName)
	buf.Write(make([]byte, 4))
	temp := buf.Bytes()
	nTProofStr := codec.HmacMD5(nt, append(serverChallenge, temp...))
	netNt := append(nTProofStr, temp...)
	netLm := append(codec.HmacMD5(lm, append(serverChallenge, clientChallenge...)), clientChallenge...)
	sessionBaseKey := codec.HmacMD5(nt, nTProofStr)
	return netNt, netLm, sessionBaseKey
}
func NetNTLMv1(nt []byte, lm []byte, serverChallenge []byte, clientChallenge []byte, Time []byte, ServerName []byte) ([]byte, []byte, []byte) {
	if false {
		netNt := desl(nt, []byte(codec.Md5(append(serverChallenge, clientChallenge...))[:8]))
		netLm := append(clientChallenge, make([]byte, 16)...)
		return netNt, netLm, md4Encode(string(netNt))
	} else {
		netNt := desl(nt, serverChallenge)
		var netLm []byte
		if NoLMResponseNTLMv1 {
			netLm = netNt
		} else {
			netLm = desl(lm, serverChallenge)
		}
		return netNt, netLm, md4Encode(string(netNt))
	}
}
