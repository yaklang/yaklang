package gmtls

import (
	"errors"
)

func UnmarshalClientHello(data []byte) (*ClientHelloInfo, error) {
	clientHello := new(clientHelloMsg)
	if !clientHello.unmarshal(data) {
		return nil, errors.New("failed to unmarshal ClientHelloInfo")
	}

	if len(clientHello.supportedVersions) <= 0 {
		clientHello.supportedVersions = []uint16{clientHello.vers}
	}
	return &ClientHelloInfo{
		CipherSuites:      clientHello.cipherSuites,
		ServerName:        clientHello.serverName,
		SupportedCurves:   clientHello.supportedCurves,
		SupportedPoints:   clientHello.supportedPoints,
		SignatureSchemes:  clientHello.supportedSignatureAlgorithms,
		SupportedProtos:   clientHello.alpnProtocols,
		SupportedVersions: clientHello.supportedVersions,
	}, nil

}
