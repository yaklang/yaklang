package protocol_impl

import (
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	utils2 "github.com/yaklang/yaklang/common/bin-parser/utils"
	"io"
)

type NegotiateMessage struct {
	Signature         [8]byte
	MessageType       uint32
	NegotiateFlags    uint32
	DomainNameFields  Field
	WorkstationFields Field
	Version           Version
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
}

func (n *NegotiateMessage) Marshal() ([]byte, error) {
	return GenSubProtocol(n, "NegotiateMessage")
}
func (n *ChallengeMessage) Marshal() ([]byte, error) {
	return GenSubProtocol(n, "ChallengeMessage")
}
func (n *AuthenticationMessage) Marshal() ([]byte, error) {
	return GenSubProtocol(n, "AuthenticationMessage")
}
func ParseSubProtocol(reader io.Reader, key string) (any, error) {
	node, err := parser.ParseBinary(reader, "application-layer.ntlm", key)
	if err != nil {
		return nil, err
	}
	d := utils2.NodeToData(node)
	return d, nil
}
func GenSubProtocol(data any, key string) ([]byte, error) {
	node, err := parser.GenerateBinary(data, "application-layer.ntlm", key)
	if err != nil {
		return nil, err
	}
	d := utils2.NodeToBytes(node)
	return d, nil
}
