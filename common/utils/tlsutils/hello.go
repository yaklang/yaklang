package tlsutils

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"yaklang/common/utils"
)

type HandshakeClientHello struct {
	Random             []byte
	Session            []byte
	CipherSuite        []byte
	CompressionMethods []byte
	ExtensionsRaw      []byte
	Extensions         []*HandshakeClientHelloExt

	checkedSNI bool
	_sni       string
	_alpn      []string
}

type HandshakeClientHelloExt struct {
	TypeRaw []byte
	TypeInt uint16
	Length  uint16
	RawData []byte
}

func (h *HandshakeClientHelloExt) IsSNI() (string, bool) {
	if h == nil {
		return "", false
	}

	if h.TypeInt != 0 {
		return "", false
	}

	// SNI 这个很简单，一般来说只有一个（虽然他理论上能容纳多个）
	if len(h.RawData) > 5 {
		return string(h.RawData[5:]), true
	}
	return "", true
}

func (h *HandshakeClientHelloExt) IsALPN() ([]string, bool) {
	if h == nil {
		return nil, false
	}

	if h.TypeInt != 16 {
		return nil, false
	}

	if len(h.RawData) <= 2 {
		return nil, true
	}

	raw := h.RawData[2:]
	rawReader := bytes.NewReader(raw)
	var protos []string
	for {
		data, err := rawReader.ReadByte()
		if err != nil {
			break
		}
		rawProto, err := utils.ReadN(rawReader, int(data))
		if err != nil {
			break
		}
		protos = append(protos, string(rawProto))
	}
	return protos, false
}

func (h *HandshakeClientHello) ALPN() []string {
	if h._alpn != nil {
		return h._alpn
	}
	for _, i := range h.Extensions {
		if alpn, ok := i.IsALPN(); ok {
			h._alpn = alpn
			return alpn
		}
	}
	return nil
}

func (h *HandshakeClientHello) MaybeHttp() bool {
	for _, l := range h.ALPN() {
		switch strings.ToLower(l) {
		case "h2":
			return true
		case "http/1.1":
			return true
		case "http/1.0":
			return true
		case "spdy/3.1":
			return true
		case "http":
			return true
		}
	}
	return false
}

func (h *HandshakeClientHello) SNI() string {
	if h.checkedSNI {
		return h._sni
	}
	for _, i := range h.Extensions {
		if sni, ok := i.IsSNI(); ok {
			h._sni = sni
			h.checkedSNI = true
			break
		}
	}
	return h._sni
}

// ParseClientHello parses a ClientHello message from the given data.
// It returns the parsed message and the number of bytes consumed.
func ParseClientHello(data []byte) (*HandshakeClientHello, error) {
	var helloInfo []byte
	if data[0] != 0x16 {
		if data[0] != 0x01 {
			return nil, utils.Error("not a tls handshake client hello")
		}
		helloInfo = data[0:]
	} else {
		helloInfo = data[5:]
	}

	originLen := len(helloInfo)
	if len(helloInfo) <= 5 {
		return nil, utils.Errorf("tls handshake client hello too short: %d", len(data))
	}

	data = helloInfo[0:]
	if ret := binary.BigEndian.Uint16([]byte{data[2], data[3]}); int(ret)+4 != originLen {
		return nil, utils.Errorf("tls handshake client hello length error: %d", ret)
	}

	hello := &HandshakeClientHello{}

	buf := bytes.NewReader(data)
	handshakeTypeRaw, _ := utils.ReadN(buf, 1) // handle shake type
	if handshakeTypeRaw[0] != 0x01 {
		return nil, utils.Errorf("not a tls handshake client hello: %d", handshakeTypeRaw[0])
	}
	utils.ReadN(buf, 3) // total len

	utils.ReadN(buf, 2)                    // version
	hello.Random, _ = utils.ReadN(buf, 32) // random

	// parse session
	sessionLengthRaw, _ := utils.ReadN(buf, 1)
	sessionLength := int(sessionLengthRaw[0])
	hello.Session, _ = utils.ReadN(buf, sessionLength)

	// parse cipher suites
	cipherSuitesLengthRaw, _ := utils.ReadN(buf, 2)
	cipherSuitesLength := int(binary.BigEndian.Uint16(cipherSuitesLengthRaw))
	hello.CipherSuite, _ = utils.ReadN(buf, cipherSuitesLength)

	// parse compression methods
	compressionMethodsLengthRaw, _ := utils.ReadN(buf, 1)
	compressionMethodsLength := int(compressionMethodsLengthRaw[0])
	hello.CompressionMethods, _ = utils.ReadN(buf, compressionMethodsLength)

	// parse extensions
	extensionsLengthRaw, _ := utils.ReadN(buf, 2)
	extensionsLength := int(binary.BigEndian.Uint16(extensionsLengthRaw))
	hello.ExtensionsRaw, _ = utils.ReadN(buf, extensionsLength)

	var extBuf = bytes.NewBufferString(string(hello.ExtensionsRaw))
	var err error
	for {
		ext := &HandshakeClientHelloExt{}
		ext.RawData, err = utils.ReadN(extBuf, 2) // extension type
		if err == io.EOF {
			break
		}
		ext.TypeInt = binary.BigEndian.Uint16(ext.RawData)
		lenRaw, _ := utils.ReadN(extBuf, 2) // extension length
		ext.Length = binary.BigEndian.Uint16(lenRaw)
		ext.RawData, _ = utils.ReadN(extBuf, int(ext.Length))
		hello.Extensions = append(hello.Extensions, ext)
	}
	return hello, nil
}
