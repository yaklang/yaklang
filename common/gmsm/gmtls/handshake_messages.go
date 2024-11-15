/*
Copyright Suzhou Tongji Fintech Research Institute 2017 All Rights Reserved.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gmtls

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"golang.org/x/crypto/cryptobyte"
	"strings"
)

// The marshalingFunction type is an adapter to allow the use of ordinary
// functions as cryptobyte.MarshalingValue.
type marshalingFunction func(b *cryptobyte.Builder) error

func (f marshalingFunction) Marshal(b *cryptobyte.Builder) error {
	return f(b)
}

// addBytesWithLength appends a sequence of bytes to the cryptobyte.Builder. If
// the length of the sequence is not the value specified, it produces an error.
func addBytesWithLength(b *cryptobyte.Builder, v []byte, n int) {
	b.AddValue(marshalingFunction(func(b *cryptobyte.Builder) error {
		if len(v) != n {
			return fmt.Errorf("invalid value length: expected %d, got %d", n, len(v))
		}
		b.AddBytes(v)
		return nil
	}))
}

// addUint64 appends a big-endian, 64-bit value to the cryptobyte.Builder.
func addUint64(b *cryptobyte.Builder, v uint64) {
	b.AddUint32(uint32(v >> 32))
	b.AddUint32(uint32(v))
}

// readUint64 decodes a big-endian, 64-bit value into out and advances over it.
// It reports whether the read was successful.
func readUint64(s *cryptobyte.String, out *uint64) bool {
	var hi, lo uint32
	if !s.ReadUint32(&hi) || !s.ReadUint32(&lo) {
		return false
	}
	*out = uint64(hi)<<32 | uint64(lo)
	return true
}

// readUint8LengthPrefixed acts like s.ReadUint8LengthPrefixed, but targets a
// []byte instead of a cryptobyte.String.
func readUint8LengthPrefixed(s *cryptobyte.String, out *[]byte) bool {
	return s.ReadUint8LengthPrefixed((*cryptobyte.String)(out))
}

// readUint16LengthPrefixed acts like s.ReadUint16LengthPrefixed, but targets a
// []byte instead of a cryptobyte.String.
func readUint16LengthPrefixed(s *cryptobyte.String, out *[]byte) bool {
	return s.ReadUint16LengthPrefixed((*cryptobyte.String)(out))
}

// readUint24LengthPrefixed acts like s.ReadUint24LengthPrefixed, but targets a
// []byte instead of a cryptobyte.String.
func readUint24LengthPrefixed(s *cryptobyte.String, out *[]byte) bool {
	return s.ReadUint24LengthPrefixed((*cryptobyte.String)(out))
}

type clientHelloMsg struct {
	raw                              []byte
	vers                             uint16
	random                           []byte
	sessionId                        []byte
	cipherSuites                     []uint16
	compressionMethods               []uint8
	nextProtoNeg                     bool
	serverName                       string
	ocspStapling                     bool
	scts                             bool
	supportedCurves                  []CurveID
	supportedPoints                  []uint8
	ticketSupported                  bool
	sessionTicket                    []uint8
	supportedSignatureAlgorithms     []SignatureScheme
	secureRenegotiation              []byte
	secureRenegotiationSupported     bool
	alpnProtocols                    []string
	supportedSignatureAlgorithmsCert []SignatureScheme
	supportedVersions                []uint16
	cookie                           []byte
	keyShares                        []keyShare
	earlyData                        bool
	pskModes                         []uint8
	pskIdentities                    []pskIdentity
	pskBinders                       [][]byte
}

func (m *clientHelloMsg) getClientVersions() []uint16 {
	clientVersions := m.supportedVersions
	if len(m.supportedVersions) == 0 {
		clientVersions = supportedVersionsFromMax(m.vers)
	}
	return clientVersions
}

// marshalWithoutBinders returns the ClientHello through the
// PreSharedKeyExtension.identities field, according to RFC 8446, Section
// 4.2.11.2. Note that m.pskBinders must be set to slices of the correct length.
func (m *clientHelloMsg) marshalWithoutBinders() []byte {
	bindersLen := 2 // uint16 length prefix
	for _, binder := range m.pskBinders {
		bindersLen += 1 // uint8 length prefix
		bindersLen += len(binder)
	}

	fullMessage := m.marshal()
	return fullMessage[:len(fullMessage)-bindersLen]
}

// updateBinders updates the m.pskBinders field, if necessary updating the
// cached marshaled representation. The supplied binders must have the same
// length as the current m.pskBinders.
func (m *clientHelloMsg) updateBinders(pskBinders [][]byte) error {
	if len(pskBinders) != len(m.pskBinders) {
		return errors.New("tls: internal error: pskBinders length mismatch")
	}
	for i := range m.pskBinders {
		if len(pskBinders[i]) != len(m.pskBinders[i]) {
			return errors.New("tls: internal error: pskBinders length mismatch")
		}
	}
	m.pskBinders = pskBinders
	if m.raw != nil {
		helloBytes := m.marshalWithoutBinders()
		lenWithoutBinders := len(helloBytes)
		b := cryptobyte.NewFixedBuilder(m.raw[:lenWithoutBinders])
		b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
			for _, binder := range m.pskBinders {
				b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
					b.AddBytes(binder)
				})
			}
		})
		if out, err := b.Bytes(); err != nil || len(out) != len(m.raw) {
			return errors.New("tls: internal error: failed to update binders")
		}
	}

	return nil
}

func (m *clientHelloMsg) marshal() []byte {
	if m.raw != nil {
		return m.raw
	}

	var exts cryptobyte.Builder
	if len(m.serverName) > 0 {
		// RFC 6066, Section 3
		exts.AddUint16(extensionServerName)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
				exts.AddUint8(0) // name_type = host_name
				exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
					exts.AddBytes([]byte(m.serverName))
				})
			})
		})
	}
	if m.ocspStapling {
		// RFC 4366, Section 3.6
		exts.AddUint16(extensionStatusRequest)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint8(1)  // status_type = ocsp
			exts.AddUint16(0) // empty responder_id_list
			exts.AddUint16(0) // empty request_extensions
		})
	}
	if len(m.supportedCurves) > 0 {
		// RFC 4492, sections 5.1.1 and RFC 8446, Section 4.2.7
		exts.AddUint16(extensionSupportedCurves)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
				for _, curve := range m.supportedCurves {
					exts.AddUint16(uint16(curve))
				}
			})
		})
	}
	if len(m.supportedPoints) > 0 {
		// RFC 4492, Section 5.1.2
		exts.AddUint16(extensionSupportedPoints)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint8LengthPrefixed(func(exts *cryptobyte.Builder) {
				exts.AddBytes(m.supportedPoints)
			})
		})
	}
	if m.ticketSupported {
		// RFC 5077, Section 3.2
		exts.AddUint16(extensionSessionTicket)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddBytes(m.sessionTicket)
		})
	}
	if len(m.supportedSignatureAlgorithms) > 0 {
		// RFC 5246, Section 7.4.1.4.1
		exts.AddUint16(extensionSignatureAlgorithms)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
				for _, sigAlgo := range m.supportedSignatureAlgorithms {
					exts.AddUint16(uint16(sigAlgo))
				}
			})
		})
	}
	if len(m.supportedSignatureAlgorithmsCert) > 0 {
		// RFC 8446, Section 4.2.3
		exts.AddUint16(extensionSignatureAlgorithmsCert)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
				for _, sigAlgo := range m.supportedSignatureAlgorithmsCert {
					exts.AddUint16(uint16(sigAlgo))
				}
			})
		})
	}
	if m.secureRenegotiationSupported {
		// RFC 5746, Section 3.2
		exts.AddUint16(extensionRenegotiationInfo)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint8LengthPrefixed(func(exts *cryptobyte.Builder) {
				exts.AddBytes(m.secureRenegotiation)
			})
		})
	}
	if len(m.alpnProtocols) > 0 {
		// RFC 7301, Section 3.1
		exts.AddUint16(extensionALPN)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
				for _, proto := range m.alpnProtocols {
					exts.AddUint8LengthPrefixed(func(exts *cryptobyte.Builder) {
						exts.AddBytes([]byte(proto))
					})
				}
			})
		})
	}
	if m.scts {
		// RFC 6962, Section 3.3.1
		exts.AddUint16(extensionSCT)
		exts.AddUint16(0) // empty extension_data
	}
	if len(m.supportedVersions) > 0 {
		// RFC 8446, Section 4.2.1
		exts.AddUint16(extensionSupportedVersions)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint8LengthPrefixed(func(exts *cryptobyte.Builder) {
				for _, vers := range m.supportedVersions {
					exts.AddUint16(vers)
				}
			})
		})
	}
	if len(m.cookie) > 0 {
		// RFC 8446, Section 4.2.2
		exts.AddUint16(extensionCookie)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
				exts.AddBytes(m.cookie)
			})
		})
	}
	if len(m.keyShares) > 0 {
		// RFC 8446, Section 4.2.8
		exts.AddUint16(extensionKeyShare)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
				for _, ks := range m.keyShares {
					exts.AddUint16(uint16(ks.group))
					exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
						exts.AddBytes(ks.data)
					})
				}
			})
		})
	}
	if m.earlyData {
		// RFC 8446, Section 4.2.10
		exts.AddUint16(extensionEarlyData)
		exts.AddUint16(0) // empty extension_data
	}
	if len(m.pskModes) > 0 {
		// RFC 8446, Section 4.2.9
		exts.AddUint16(extensionPSKModes)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint8LengthPrefixed(func(exts *cryptobyte.Builder) {
				exts.AddBytes(m.pskModes)
			})
		})
	}
	if len(m.pskIdentities) > 0 { // pre_shared_key must be the last extension
		// RFC 8446, Section 4.2.11
		exts.AddUint16(extensionPreSharedKey)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
				for _, psk := range m.pskIdentities {
					exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
						exts.AddBytes(psk.label)
					})
					exts.AddUint32(psk.obfuscatedTicketAge)
				}
			})
			exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
				for _, binder := range m.pskBinders {
					exts.AddUint8LengthPrefixed(func(exts *cryptobyte.Builder) {
						exts.AddBytes(binder)
					})
				}
			})
		})
	}
	extBytes, err := exts.Bytes()
	if err != nil {
		log.Errorf("gmtls: failed to marshal ClientHello extensions: %v", err)
		return nil
	}

	var b cryptobyte.Builder
	b.AddUint8(typeClientHello)
	b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
		b.AddUint16(m.vers)
		addBytesWithLength(b, m.random, 32)
		b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
			b.AddBytes(m.sessionId)
		})
		b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
			for _, suite := range m.cipherSuites {
				b.AddUint16(suite)
			}
		})
		b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
			b.AddBytes(m.compressionMethods)
		})

		if len(extBytes) > 0 {
			b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
				b.AddBytes(extBytes)
			})
		}
	})

	m.raw, err = b.Bytes()
	if err != nil {
		log.Errorf("gmtls: failed to marshal ClientHello: %v", err)
	}
	return m.raw
}

func (m *clientHelloMsg) unmarshal(data []byte) bool {
	*m = clientHelloMsg{raw: data}
	s := cryptobyte.String(data)

	if !s.Skip(4) || // message type and uint24 length field
		!s.ReadUint16(&m.vers) || !s.ReadBytes(&m.random, 32) ||
		!readUint8LengthPrefixed(&s, &m.sessionId) {
		return false
	}

	var cipherSuites cryptobyte.String
	if !s.ReadUint16LengthPrefixed(&cipherSuites) {
		return false
	}
	m.cipherSuites = []uint16{}
	m.secureRenegotiationSupported = false
	for !cipherSuites.Empty() {
		var suite uint16
		if !cipherSuites.ReadUint16(&suite) {
			return false
		}
		if suite == scsvRenegotiation {
			m.secureRenegotiationSupported = true
		}
		m.cipherSuites = append(m.cipherSuites, suite)
	}

	if !readUint8LengthPrefixed(&s, &m.compressionMethods) {
		return false
	}

	if s.Empty() {
		// ClientHello is optionally followed by extension data
		return true
	}

	var extensions cryptobyte.String
	if !s.ReadUint16LengthPrefixed(&extensions) || !s.Empty() {
		return false
	}

	seenExts := make(map[uint16]bool)
	for !extensions.Empty() {
		var extension uint16
		var extData cryptobyte.String
		if !extensions.ReadUint16(&extension) ||
			!extensions.ReadUint16LengthPrefixed(&extData) {
			return false
		}

		if seenExts[extension] {
			return false
		}
		seenExts[extension] = true

		switch extension {
		case extensionServerName:
			// RFC 6066, Section 3
			var nameList cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&nameList) || nameList.Empty() {
				return false
			}
			for !nameList.Empty() {
				var nameType uint8
				var serverName cryptobyte.String
				if !nameList.ReadUint8(&nameType) ||
					!nameList.ReadUint16LengthPrefixed(&serverName) ||
					serverName.Empty() {
					return false
				}
				if nameType != 0 {
					continue
				}
				if len(m.serverName) != 0 {
					// Multiple names of the same name_type are prohibited.
					return false
				}
				m.serverName = string(serverName)
				// An SNI value may not include a trailing dot.
				if strings.HasSuffix(m.serverName, ".") {
					return false
				}
			}
		case extensionStatusRequest:
			// RFC 4366, Section 3.6
			var statusType uint8
			var ignored cryptobyte.String
			if !extData.ReadUint8(&statusType) ||
				!extData.ReadUint16LengthPrefixed(&ignored) ||
				!extData.ReadUint16LengthPrefixed(&ignored) {
				return false
			}
			m.ocspStapling = statusType == statusTypeOCSP
		case extensionSupportedCurves:
			// RFC 4492, sections 5.1.1 and RFC 8446, Section 4.2.7
			var curves cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&curves) || curves.Empty() {
				return false
			}
			for !curves.Empty() {
				var curve uint16
				if !curves.ReadUint16(&curve) {
					return false
				}
				m.supportedCurves = append(m.supportedCurves, CurveID(curve))
			}
		case extensionSupportedPoints:
			// RFC 4492, Section 5.1.2
			if !readUint8LengthPrefixed(&extData, &m.supportedPoints) ||
				len(m.supportedPoints) == 0 {
				return false
			}
		case extensionSessionTicket:
			// RFC 5077, Section 3.2
			m.ticketSupported = true
			extData.ReadBytes(&m.sessionTicket, len(extData))
		case extensionSignatureAlgorithms:
			// RFC 5246, Section 7.4.1.4.1
			var sigAndAlgs cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&sigAndAlgs) || sigAndAlgs.Empty() {
				return false
			}
			for !sigAndAlgs.Empty() {
				var sigAndAlg uint16
				if !sigAndAlgs.ReadUint16(&sigAndAlg) {
					return false
				}
				m.supportedSignatureAlgorithms = append(
					m.supportedSignatureAlgorithms, SignatureScheme(sigAndAlg))
			}
		case extensionSignatureAlgorithmsCert:
			// RFC 8446, Section 4.2.3
			var sigAndAlgs cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&sigAndAlgs) || sigAndAlgs.Empty() {
				return false
			}
			for !sigAndAlgs.Empty() {
				var sigAndAlg uint16
				if !sigAndAlgs.ReadUint16(&sigAndAlg) {
					return false
				}
				m.supportedSignatureAlgorithmsCert = append(
					m.supportedSignatureAlgorithmsCert, SignatureScheme(sigAndAlg))
			}
		case extensionNextProtoNeg:
			var protoNeg uint16
			if !extData.ReadUint16(&protoNeg) {
				return false
			}
			m.nextProtoNeg = protoNeg == 0
		case extensionRenegotiationInfo:
			// RFC 5746, Section 3.2
			if !readUint8LengthPrefixed(&extData, &m.secureRenegotiation) {
				return false
			}
			m.secureRenegotiationSupported = true
		case extensionALPN:
			// RFC 7301, Section 3.1
			var protoList cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&protoList) || protoList.Empty() {
				return false
			}
			for !protoList.Empty() {
				var proto cryptobyte.String
				if !protoList.ReadUint8LengthPrefixed(&proto) || proto.Empty() {
					return false
				}
				m.alpnProtocols = append(m.alpnProtocols, string(proto))
			}
		case extensionSCT:
			// RFC 6962, Section 3.3.1
			m.scts = true
		case extensionSupportedVersions:
			// RFC 8446, Section 4.2.1
			var versList cryptobyte.String
			if !extData.ReadUint8LengthPrefixed(&versList) || versList.Empty() {
				return false
			}
			for !versList.Empty() {
				var vers uint16
				if !versList.ReadUint16(&vers) {
					return false
				}
				m.supportedVersions = append(m.supportedVersions, vers)
			}
		case extensionCookie:
			// RFC 8446, Section 4.2.2
			if !readUint16LengthPrefixed(&extData, &m.cookie) ||
				len(m.cookie) == 0 {
				return false
			}
		case extensionKeyShare:
			// RFC 8446, Section 4.2.8
			var clientShares cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&clientShares) {
				return false
			}
			for !clientShares.Empty() {
				var ks keyShare
				if !clientShares.ReadUint16((*uint16)(&ks.group)) ||
					!readUint16LengthPrefixed(&clientShares, &ks.data) ||
					len(ks.data) == 0 {
					return false
				}
				m.keyShares = append(m.keyShares, ks)
			}
		case extensionEarlyData:
			// RFC 8446, Section 4.2.10
			m.earlyData = true
		case extensionPSKModes:
			// RFC 8446, Section 4.2.9
			if !readUint8LengthPrefixed(&extData, &m.pskModes) {
				return false
			}
		case extensionPreSharedKey:
			// RFC 8446, Section 4.2.11
			if !extensions.Empty() {
				return false // pre_shared_key must be the last extension
			}
			var identities cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&identities) || identities.Empty() {
				return false
			}
			for !identities.Empty() {
				var psk pskIdentity
				if !readUint16LengthPrefixed(&identities, &psk.label) ||
					!identities.ReadUint32(&psk.obfuscatedTicketAge) ||
					len(psk.label) == 0 {
					return false
				}
				m.pskIdentities = append(m.pskIdentities, psk)
			}
			var binders cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&binders) || binders.Empty() {
				return false
			}
			for !binders.Empty() {
				var binder []byte
				if !readUint8LengthPrefixed(&binders, &binder) ||
					len(binder) == 0 {
					return false
				}
				m.pskBinders = append(m.pskBinders, binder)
			}
		default:
			// Ignore unknown extensions.
			continue
		}

		if !extData.Empty() {
			return false
		}
	}

	return true
}

func (m *clientHelloMsg) equal(i interface{}) bool {
	m1, ok := i.(*clientHelloMsg)
	if !ok {
		return false
	}

	return bytes.Equal(m.raw, m1.raw) &&
		m.vers == m1.vers &&
		bytes.Equal(m.random, m1.random) &&
		bytes.Equal(m.sessionId, m1.sessionId) &&
		eqUint16s(m.cipherSuites, m1.cipherSuites) &&
		bytes.Equal(m.compressionMethods, m1.compressionMethods) &&
		m.nextProtoNeg == m1.nextProtoNeg &&
		m.serverName == m1.serverName &&
		m.ocspStapling == m1.ocspStapling &&
		m.scts == m1.scts &&
		eqCurveIDs(m.supportedCurves, m1.supportedCurves) &&
		bytes.Equal(m.supportedPoints, m1.supportedPoints) &&
		m.ticketSupported == m1.ticketSupported &&
		bytes.Equal(m.sessionTicket, m1.sessionTicket) &&
		eqSignatureAlgorithms(m.supportedSignatureAlgorithms, m1.supportedSignatureAlgorithms) &&
		m.secureRenegotiationSupported == m1.secureRenegotiationSupported &&
		bytes.Equal(m.secureRenegotiation, m1.secureRenegotiation) &&
		eqStrings(m.alpnProtocols, m1.alpnProtocols)
}

//type clientHelloMsg struct {
//	raw                          []byte
//	vers                         uint16
//	random                       []byte
//	sessionId                    []byte
//	cipherSuites                 []uint16
//	compressionMethods           []uint8
//	nextProtoNeg                 bool
//	serverName                   string
//	ocspStapling                 bool
//	scts                         bool
//	supportedCurves              []CurveID
//	supportedPoints              []uint8
//	ticketSupported              bool
//	sessionTicket                []uint8
//	supportedSignatureAlgorithms []SignatureScheme
//	secureRenegotiation          []byte
//	secureRenegotiationSupported bool
//	alpnProtocols                []string
//}
//
//func (m *clientHelloMsg) marshal() []byte {
//	if m.raw != nil {
//		return m.raw
//	}
//
//	length := 2 + 32 + 1 + len(m.sessionId) + 2 + len(m.cipherSuites)*2 + 1 + len(m.compressionMethods)
//	numExtensions := 0
//	extensionsLength := 0
//	if m.nextProtoNeg {
//		numExtensions++
//	}
//	if m.ocspStapling {
//		extensionsLength += 1 + 2 + 2
//		numExtensions++
//	}
//	if len(m.serverName) > 0 {
//		extensionsLength += 5 + len(m.serverName)
//		numExtensions++
//	}
//	if len(m.supportedCurves) > 0 {
//		extensionsLength += 2 + 2*len(m.supportedCurves)
//		numExtensions++
//	}
//	if len(m.supportedPoints) > 0 {
//		extensionsLength += 1 + len(m.supportedPoints)
//		numExtensions++
//	}
//	if m.ticketSupported {
//		extensionsLength += len(m.sessionTicket)
//		numExtensions++
//	}
//	if len(m.supportedSignatureAlgorithms) > 0 {
//		extensionsLength += 2 + 2*len(m.supportedSignatureAlgorithms)
//		numExtensions++
//	}
//	if m.secureRenegotiationSupported {
//		extensionsLength += 1 + len(m.secureRenegotiation)
//		numExtensions++
//	}
//	if len(m.alpnProtocols) > 0 {
//		extensionsLength += 2
//		for _, s := range m.alpnProtocols {
//			if l := len(s); l == 0 || l > 255 {
//				panic("invalid ALPN protocol")
//			}
//			extensionsLength++
//			extensionsLength += len(s)
//		}
//		numExtensions++
//	}
//	if m.scts {
//		numExtensions++
//	}
//	if numExtensions > 0 {
//		extensionsLength += 4 * numExtensions
//		length += 2 + extensionsLength
//	}
//
//	x := make([]byte, 4+length)
//	x[0] = typeClientHello
//	x[1] = uint8(length >> 16)
//	x[2] = uint8(length >> 8)
//	x[3] = uint8(length)
//	x[4] = uint8(m.vers >> 8)
//	x[5] = uint8(m.vers)
//	copy(x[6:38], m.random)
//	x[38] = uint8(len(m.sessionId))
//	copy(x[39:39+len(m.sessionId)], m.sessionId)
//	y := x[39+len(m.sessionId):]
//	y[0] = uint8(len(m.cipherSuites) >> 7)
//	y[1] = uint8(len(m.cipherSuites) << 1)
//	for i, suite := range m.cipherSuites {
//		y[2+i*2] = uint8(suite >> 8)
//		y[3+i*2] = uint8(suite)
//	}
//	z := y[2+len(m.cipherSuites)*2:]
//	z[0] = uint8(len(m.compressionMethods))
//	copy(z[1:], m.compressionMethods)
//
//	z = z[1+len(m.compressionMethods):]
//	if numExtensions > 0 {
//		z[0] = byte(extensionsLength >> 8)
//		z[1] = byte(extensionsLength)
//		z = z[2:]
//	}
//	if m.nextProtoNeg {
//		z[0] = byte(extensionNextProtoNeg >> 8)
//		z[1] = byte(extensionNextProtoNeg & 0xff)
//		// The length is always 0
//		z = z[4:]
//	}
//	if len(m.serverName) > 0 {
//		z[0] = byte(extensionServerName >> 8)
//		z[1] = byte(extensionServerName & 0xff)
//		l := len(m.serverName) + 5
//		z[2] = byte(l >> 8)
//		z[3] = byte(l)
//		z = z[4:]
//
//		// RFC 3546, section 3.1
//		//
//		// struct {
//		//     NameType name_type;
//		//     select (name_type) {
//		//         case host_name: HostName;
//		//     } name;
//		// } ServerName;
//		//
//		// enum {
//		//     host_name(0), (255)
//		// } NameType;
//		//
//		// opaque HostName<1..2^16-1>;
//		//
//		// struct {
//		//     ServerName server_name_list<1..2^16-1>
//		// } ServerNameList;
//
//		z[0] = byte((len(m.serverName) + 3) >> 8)
//		z[1] = byte(len(m.serverName) + 3)
//		z[3] = byte(len(m.serverName) >> 8)
//		z[4] = byte(len(m.serverName))
//		copy(z[5:], []byte(m.serverName))
//		z = z[l:]
//	}
//	if m.ocspStapling {
//		// RFC 4366, section 3.6
//		z[0] = byte(extensionStatusRequest >> 8)
//		z[1] = byte(extensionStatusRequest)
//		z[2] = 0
//		z[3] = 5
//		z[4] = 1 // OCSP type
//		// Two zero valued uint16s for the two lengths.
//		z = z[9:]
//	}
//	if len(m.supportedCurves) > 0 {
//		// http://tools.ietf.org/html/rfc4492#section-5.5.1
//		z[0] = byte(extensionSupportedCurves >> 8)
//		z[1] = byte(extensionSupportedCurves)
//		l := 2 + 2*len(m.supportedCurves)
//		z[2] = byte(l >> 8)
//		z[3] = byte(l)
//		l -= 2
//		z[4] = byte(l >> 8)
//		z[5] = byte(l)
//		z = z[6:]
//		for _, curve := range m.supportedCurves {
//			z[0] = byte(curve >> 8)
//			z[1] = byte(curve)
//			z = z[2:]
//		}
//	}
//	if len(m.supportedPoints) > 0 {
//		// http://tools.ietf.org/html/rfc4492#section-5.5.2
//		z[0] = byte(extensionSupportedPoints >> 8)
//		z[1] = byte(extensionSupportedPoints)
//		l := 1 + len(m.supportedPoints)
//		z[2] = byte(l >> 8)
//		z[3] = byte(l)
//		l--
//		z[4] = byte(l)
//		z = z[5:]
//		for _, pointFormat := range m.supportedPoints {
//			z[0] = pointFormat
//			z = z[1:]
//		}
//	}
//	if m.ticketSupported {
//		// http://tools.ietf.org/html/rfc5077#section-3.2
//		z[0] = byte(extensionSessionTicket >> 8)
//		z[1] = byte(extensionSessionTicket)
//		l := len(m.sessionTicket)
//		z[2] = byte(l >> 8)
//		z[3] = byte(l)
//		z = z[4:]
//		copy(z, m.sessionTicket)
//		z = z[len(m.sessionTicket):]
//	}
//	if len(m.supportedSignatureAlgorithms) > 0 {
//		// https://tools.ietf.org/html/rfc5246#section-7.4.1.4.1
//		z[0] = byte(extensionSignatureAlgorithms >> 8)
//		z[1] = byte(extensionSignatureAlgorithms)
//		l := 2 + 2*len(m.supportedSignatureAlgorithms)
//		z[2] = byte(l >> 8)
//		z[3] = byte(l)
//		z = z[4:]
//
//		l -= 2
//		z[0] = byte(l >> 8)
//		z[1] = byte(l)
//		z = z[2:]
//		for _, sigAlgo := range m.supportedSignatureAlgorithms {
//			z[0] = byte(sigAlgo >> 8)
//			z[1] = byte(sigAlgo)
//			z = z[2:]
//		}
//	}
//	if m.secureRenegotiationSupported {
//		z[0] = byte(extensionRenegotiationInfo >> 8)
//		z[1] = byte(extensionRenegotiationInfo & 0xff)
//		z[2] = 0
//		z[3] = byte(len(m.secureRenegotiation) + 1)
//		z[4] = byte(len(m.secureRenegotiation))
//		z = z[5:]
//		copy(z, m.secureRenegotiation)
//		z = z[len(m.secureRenegotiation):]
//	}
//	if len(m.alpnProtocols) > 0 {
//		z[0] = byte(extensionALPN >> 8)
//		z[1] = byte(extensionALPN & 0xff)
//		lengths := z[2:]
//		z = z[6:]
//
//		stringsLength := 0
//		for _, s := range m.alpnProtocols {
//			l := len(s)
//			z[0] = byte(l)
//			copy(z[1:], s)
//			z = z[1+l:]
//			stringsLength += 1 + l
//		}
//
//		lengths[2] = byte(stringsLength >> 8)
//		lengths[3] = byte(stringsLength)
//		stringsLength += 2
//		lengths[0] = byte(stringsLength >> 8)
//		lengths[1] = byte(stringsLength)
//	}
//	if m.scts {
//		// https://tools.ietf.org/html/rfc6962#section-3.3.1
//		z[0] = byte(extensionSCT >> 8)
//		z[1] = byte(extensionSCT)
//		// zero uint16 for the zero-length extension_data
//		z = z[4:]
//	}
//
//	m.raw = x
//
//	return x
//}
//
//func (m *clientHelloMsg) unmarshal(data []byte) bool {
//	if len(data) < 42 {
//		return false
//	}
//	m.raw = data
//	m.vers = uint16(data[4])<<8 | uint16(data[5])
//	m.random = data[6:38]
//	sessionIdLen := int(data[38])
//	if sessionIdLen > 32 || len(data) < 39+sessionIdLen {
//		return false
//	}
//	m.sessionId = data[39 : 39+sessionIdLen]
//	data = data[39+sessionIdLen:]
//	if len(data) < 2 {
//		return false
//	}
//	// cipherSuiteLen is the number of bytes of cipher suite numbers. Since
//	// they are uint16s, the number must be even.
//	cipherSuiteLen := int(data[0])<<8 | int(data[1])
//	if cipherSuiteLen%2 == 1 || len(data) < 2+cipherSuiteLen {
//		return false
//	}
//	numCipherSuites := cipherSuiteLen / 2
//	m.cipherSuites = make([]uint16, numCipherSuites)
//	for i := 0; i < numCipherSuites; i++ {
//		m.cipherSuites[i] = uint16(data[2+2*i])<<8 | uint16(data[3+2*i])
//		if m.cipherSuites[i] == scsvRenegotiation {
//			m.secureRenegotiationSupported = true
//		}
//	}
//	data = data[2+cipherSuiteLen:]
//	if len(data) < 1 {
//		return false
//	}
//	compressionMethodsLen := int(data[0])
//	if len(data) < 1+compressionMethodsLen {
//		return false
//	}
//	m.compressionMethods = data[1 : 1+compressionMethodsLen]
//
//	data = data[1+compressionMethodsLen:]
//
//	m.nextProtoNeg = false
//	m.serverName = ""
//	m.ocspStapling = false
//	m.ticketSupported = false
//	m.sessionTicket = nil
//	m.supportedSignatureAlgorithms = nil
//	m.alpnProtocols = nil
//	m.scts = false
//
//	if len(data) == 0 {
//		// ClientHello is optionally followed by extension data
//		return true
//	}
//	if len(data) < 2 {
//		return false
//	}
//
//	extensionsLength := int(data[0])<<8 | int(data[1])
//	data = data[2:]
//	if extensionsLength != len(data) {
//		return false
//	}
//
//	for len(data) != 0 {
//		if len(data) < 4 {
//			return false
//		}
//		extension := uint16(data[0])<<8 | uint16(data[1])
//		length := int(data[2])<<8 | int(data[3])
//		data = data[4:]
//		if len(data) < length {
//			return false
//		}
//
//		switch extension {
//		case extensionServerName:
//			d := data[:length]
//			if len(d) < 2 {
//				return false
//			}
//			namesLen := int(d[0])<<8 | int(d[1])
//			d = d[2:]
//			if len(d) != namesLen {
//				return false
//			}
//			for len(d) > 0 {
//				if len(d) < 3 {
//					return false
//				}
//				nameType := d[0]
//				nameLen := int(d[1])<<8 | int(d[2])
//				d = d[3:]
//				if len(d) < nameLen {
//					return false
//				}
//				if nameType == 0 {
//					m.serverName = string(d[:nameLen])
//					break
//				}
//				d = d[nameLen:]
//			}
//		case extensionNextProtoNeg:
//			if length > 0 {
//				return false
//			}
//			m.nextProtoNeg = true
//		case extensionStatusRequest:
//			m.ocspStapling = length > 0 && data[0] == statusTypeOCSP
//		case extensionSupportedCurves:
//			// http://tools.ietf.org/html/rfc4492#section-5.5.1
//			if length < 2 {
//				return false
//			}
//			l := int(data[0])<<8 | int(data[1])
//			if l%2 == 1 || length != l+2 {
//				return false
//			}
//			numCurves := l / 2
//			m.supportedCurves = make([]CurveID, numCurves)
//			d := data[2:]
//			for i := 0; i < numCurves; i++ {
//				m.supportedCurves[i] = CurveID(d[0])<<8 | CurveID(d[1])
//				d = d[2:]
//			}
//		case extensionSupportedPoints:
//			// http://tools.ietf.org/html/rfc4492#section-5.5.2
//			if length < 1 {
//				return false
//			}
//			l := int(data[0])
//			if length != l+1 {
//				return false
//			}
//			m.supportedPoints = make([]uint8, l)
//			copy(m.supportedPoints, data[1:])
//		case extensionSessionTicket:
//			// http://tools.ietf.org/html/rfc5077#section-3.2
//			m.ticketSupported = true
//			m.sessionTicket = data[:length]
//		case extensionSignatureAlgorithms:
//			// https://tools.ietf.org/html/rfc5246#section-7.4.1.4.1
//			if length < 2 || length&1 != 0 {
//				return false
//			}
//			l := int(data[0])<<8 | int(data[1])
//			if l != length-2 {
//				return false
//			}
//			n := l / 2
//			d := data[2:]
//			m.supportedSignatureAlgorithms = make([]SignatureScheme, n)
//			for i := range m.supportedSignatureAlgorithms {
//				m.supportedSignatureAlgorithms[i] = SignatureScheme(d[0])<<8 | SignatureScheme(d[1])
//				d = d[2:]
//
//			}
//		case extensionRenegotiationInfo:
//			if length == 0 {
//				return false
//			}
//			d := data[:length]
//			l := int(d[0])
//			d = d[1:]
//			if l != len(d) {
//				return false
//			}
//
//			m.secureRenegotiation = d
//			m.secureRenegotiationSupported = true
//		case extensionALPN:
//			if length < 2 {
//				return false
//			}
//			l := int(data[0])<<8 | int(data[1])
//			if l != length-2 {
//				return false
//			}
//			d := data[2:length]
//			for len(d) != 0 {
//				stringLen := int(d[0])
//				d = d[1:]
//				if stringLen == 0 || stringLen > len(d) {
//					return false
//				}
//				m.alpnProtocols = append(m.alpnProtocols, string(d[:stringLen]))
//				d = d[stringLen:]
//			}
//		case extensionSCT:
//			m.scts = true
//			if length != 0 {
//				return false
//			}
//		}
//		data = data[length:]
//	}
//
//	return true
//}

type serverHelloMsg struct {
	raw                          []byte
	vers                         uint16
	random                       []byte
	sessionId                    []byte
	cipherSuite                  uint16
	compressionMethod            uint8
	ocspStapling                 bool
	ticketSupported              bool
	secureRenegotiation          []byte
	secureRenegotiationSupported bool
	alpnProtocol                 string
	scts                         [][]byte
	supportedVersion             uint16
	serverShare                  keyShare

	selectedIdentityPresent bool
	selectedIdentity        uint16
	supportedPoints         []uint8

	// HelloRetryRequest extensions
	cookie        []byte
	selectedGroup CurveID

	nextProtoNeg bool
	nextProtos   []string
}

func (m *serverHelloMsg) getServerVersion() uint16 {
	peerVersion := m.vers
	if m.supportedVersion != 0 {
		peerVersion = m.supportedVersion
	}
	return peerVersion
}

func (m *serverHelloMsg) marshal() []byte {
	if m.raw != nil {
		return m.raw
	}

	var exts cryptobyte.Builder
	if m.ocspStapling {
		exts.AddUint16(extensionStatusRequest)
		exts.AddUint16(0) // empty extension_data
	}
	if m.ticketSupported {
		exts.AddUint16(extensionSessionTicket)
		exts.AddUint16(0) // empty extension_data
	}
	if m.secureRenegotiationSupported {
		exts.AddUint16(extensionRenegotiationInfo)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint8LengthPrefixed(func(exts *cryptobyte.Builder) {
				exts.AddBytes(m.secureRenegotiation)
			})
		})
	}
	if len(m.alpnProtocol) > 0 {
		exts.AddUint16(extensionALPN)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
				exts.AddUint8LengthPrefixed(func(exts *cryptobyte.Builder) {
					exts.AddBytes([]byte(m.alpnProtocol))
				})
			})
		})
	}
	if len(m.scts) > 0 {
		exts.AddUint16(extensionSCT)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
				for _, sct := range m.scts {
					exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
						exts.AddBytes(sct)
					})
				}
			})
		})
	}
	if m.supportedVersion != 0 {
		exts.AddUint16(extensionSupportedVersions)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16(m.supportedVersion)
		})
	}

	if m.nextProtoNeg || len(m.nextProtos) > 0 {
		exts.AddUint16(extensionNextProtoNeg)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			for _, proto := range m.nextProtos {
				exts.AddUint8LengthPrefixed(func(exts *cryptobyte.Builder) {
					exts.AddBytes([]byte(proto))
				})
			}
		})
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16(m.supportedVersion)
		})
	}

	if m.serverShare.group != 0 {
		exts.AddUint16(extensionKeyShare)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16(uint16(m.serverShare.group))
			exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
				exts.AddBytes(m.serverShare.data)
			})
		})
	}
	if m.selectedIdentityPresent {
		exts.AddUint16(extensionPreSharedKey)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16(m.selectedIdentity)
		})
	}

	if len(m.cookie) > 0 {
		exts.AddUint16(extensionCookie)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
				exts.AddBytes(m.cookie)
			})
		})
	}
	if m.selectedGroup != 0 {
		exts.AddUint16(extensionKeyShare)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint16(uint16(m.selectedGroup))
		})
	}
	if len(m.supportedPoints) > 0 {
		exts.AddUint16(extensionSupportedPoints)
		exts.AddUint16LengthPrefixed(func(exts *cryptobyte.Builder) {
			exts.AddUint8LengthPrefixed(func(exts *cryptobyte.Builder) {
				exts.AddBytes(m.supportedPoints)
			})
		})
	}

	extBytes, err := exts.Bytes()
	if err != nil {
		log.Errorf("gmtls: failed to marshal ServerHello extensions: %v", err)
		return nil
	}

	var b cryptobyte.Builder
	b.AddUint8(typeServerHello)
	b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
		b.AddUint16(m.vers)
		addBytesWithLength(b, m.random, 32)
		b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
			b.AddBytes(m.sessionId)
		})
		b.AddUint16(m.cipherSuite)
		b.AddUint8(m.compressionMethod)

		if len(extBytes) > 0 {
			b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
				b.AddBytes(extBytes)
			})
		}
	})

	m.raw, err = b.Bytes()
	if err != nil {
		log.Errorf("gmtls: failed to marshal ServerHello: %v", err)
	}
	return m.raw
}

func (m *serverHelloMsg) unmarshal(data []byte) bool {
	*m = serverHelloMsg{raw: data}
	s := cryptobyte.String(data)

	if !s.Skip(4) || // message type and uint24 length field
		!s.ReadUint16(&m.vers) || !s.ReadBytes(&m.random, 32) ||
		!readUint8LengthPrefixed(&s, &m.sessionId) ||
		!s.ReadUint16(&m.cipherSuite) ||
		!s.ReadUint8(&m.compressionMethod) {
		return false
	}

	if s.Empty() {
		// ServerHello is optionally followed by extension data
		return true
	}

	var extensions cryptobyte.String
	if !s.ReadUint16LengthPrefixed(&extensions) || !s.Empty() {
		return false
	}

	seenExts := make(map[uint16]bool)
	for !extensions.Empty() {
		var extension uint16
		var extData cryptobyte.String
		if !extensions.ReadUint16(&extension) ||
			!extensions.ReadUint16LengthPrefixed(&extData) {
			return false
		}

		if seenExts[extension] {
			return false
		}
		seenExts[extension] = true

		switch extension {
		case extensionStatusRequest:
			m.ocspStapling = true
		case extensionSessionTicket:
			m.ticketSupported = true
		case extensionRenegotiationInfo:
			if !readUint8LengthPrefixed(&extData, &m.secureRenegotiation) {
				return false
			}
			m.secureRenegotiationSupported = true
		case extensionALPN:
			var protoList cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&protoList) || protoList.Empty() {
				return false
			}
			var proto cryptobyte.String
			if !protoList.ReadUint8LengthPrefixed(&proto) ||
				proto.Empty() || !protoList.Empty() {
				return false
			}
			m.alpnProtocol = string(proto)
		case extensionSCT:
			var sctList cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&sctList) || sctList.Empty() {
				return false
			}
			for !sctList.Empty() {
				var sct []byte
				if !readUint16LengthPrefixed(&sctList, &sct) ||
					len(sct) == 0 {
					return false
				}
				m.scts = append(m.scts, sct)
			}
		case extensionSupportedVersions:
			if !extData.ReadUint16(&m.supportedVersion) {
				return false
			}
		case extensionCookie:
			if !readUint16LengthPrefixed(&extData, &m.cookie) ||
				len(m.cookie) == 0 {
				return false
			}
		case extensionKeyShare:
			// This extension has different formats in SH and HRR, accept either
			// and let the handshake logic decide. See RFC 8446, Section 4.2.8.
			if len(extData) == 2 {
				if !extData.ReadUint16((*uint16)(&m.selectedGroup)) {
					return false
				}
			} else {
				if !extData.ReadUint16((*uint16)(&m.serverShare.group)) ||
					!readUint16LengthPrefixed(&extData, &m.serverShare.data) {
					return false
				}
			}
		case extensionPreSharedKey:
			m.selectedIdentityPresent = true
			if !extData.ReadUint16(&m.selectedIdentity) {
				return false
			}
		case extensionSupportedPoints:
			// RFC 4492, Section 5.1.2
			if !readUint8LengthPrefixed(&extData, &m.supportedPoints) ||
				len(m.supportedPoints) == 0 {
				return false
			}
		case extensionNextProtoNeg:

		default:
			// Ignore unknown extensions.
			continue
		}

		if !extData.Empty() {
			return false
		}
	}

	return true
}

func (m *serverHelloMsg) equal(i interface{}) bool {
	m1, ok := i.(*serverHelloMsg)
	if !ok {
		return false
	}

	if len(m.scts) != len(m1.scts) {
		return false
	}
	for i, sct := range m.scts {
		if !bytes.Equal(sct, m1.scts[i]) {
			return false
		}
	}

	return bytes.Equal(m.raw, m1.raw) &&
		m.vers == m1.vers &&
		bytes.Equal(m.random, m1.random) &&
		bytes.Equal(m.sessionId, m1.sessionId) &&
		m.cipherSuite == m1.cipherSuite &&
		m.compressionMethod == m1.compressionMethod &&
		m.nextProtoNeg == m1.nextProtoNeg &&
		eqStrings(m.nextProtos, m1.nextProtos) &&
		m.ocspStapling == m1.ocspStapling &&
		m.ticketSupported == m1.ticketSupported &&
		m.secureRenegotiationSupported == m1.secureRenegotiationSupported &&
		bytes.Equal(m.secureRenegotiation, m1.secureRenegotiation) &&
		m.alpnProtocol == m1.alpnProtocol
}

//func (m *serverHelloMsg) marshal() []byte {
//	if m.raw != nil {
//		return m.raw
//	}
//
//	length := 38 + len(m.sessionId)
//	numExtensions := 0
//	extensionsLength := 0
//
//	nextProtoLen := 0
//	if m.nextProtoNeg {
//		numExtensions++
//		for _, v := range m.nextProtos {
//			nextProtoLen += len(v)
//		}
//		nextProtoLen += len(m.nextProtos)
//		extensionsLength += nextProtoLen
//	}
//	if m.ocspStapling {
//		numExtensions++
//	}
//	if m.ticketSupported {
//		numExtensions++
//	}
//	if m.secureRenegotiationSupported {
//		extensionsLength += 1 + len(m.secureRenegotiation)
//		numExtensions++
//	}
//	if alpnLen := len(m.alpnProtocol); alpnLen > 0 {
//		if alpnLen >= 256 {
//			panic("invalid ALPN protocol")
//		}
//		extensionsLength += 2 + 1 + alpnLen
//		numExtensions++
//	}
//	sctLen := 0
//	if len(m.scts) > 0 {
//		for _, sct := range m.scts {
//			sctLen += len(sct) + 2
//		}
//		extensionsLength += 2 + sctLen
//		numExtensions++
//	}
//
//	if numExtensions > 0 {
//		extensionsLength += 4 * numExtensions
//		length += 2 + extensionsLength
//	}
//
//	x := make([]byte, 4+length)
//	x[0] = typeServerHello
//	x[1] = uint8(length >> 16)
//	x[2] = uint8(length >> 8)
//	x[3] = uint8(length)
//	x[4] = uint8(m.vers >> 8)
//	x[5] = uint8(m.vers)
//	copy(x[6:38], m.random)
//	x[38] = uint8(len(m.sessionId))
//	copy(x[39:39+len(m.sessionId)], m.sessionId)
//	z := x[39+len(m.sessionId):]
//	z[0] = uint8(m.cipherSuite >> 8)
//	z[1] = uint8(m.cipherSuite)
//	z[2] = m.compressionMethod
//
//	z = z[3:]
//	if numExtensions > 0 {
//		z[0] = byte(extensionsLength >> 8)
//		z[1] = byte(extensionsLength)
//		z = z[2:]
//	}
//	if m.nextProtoNeg {
//		z[0] = byte(extensionNextProtoNeg >> 8)
//		z[1] = byte(extensionNextProtoNeg & 0xff)
//		z[2] = byte(nextProtoLen >> 8)
//		z[3] = byte(nextProtoLen)
//		z = z[4:]
//
//		for _, v := range m.nextProtos {
//			l := len(v)
//			if l > 255 {
//				l = 255
//			}
//			z[0] = byte(l)
//			copy(z[1:], []byte(v[0:l]))
//			z = z[1+l:]
//		}
//	}
//	if m.ocspStapling {
//		z[0] = byte(extensionStatusRequest >> 8)
//		z[1] = byte(extensionStatusRequest)
//		z = z[4:]
//	}
//	if m.ticketSupported {
//		z[0] = byte(extensionSessionTicket >> 8)
//		z[1] = byte(extensionSessionTicket)
//		z = z[4:]
//	}
//	if m.secureRenegotiationSupported {
//		z[0] = byte(extensionRenegotiationInfo >> 8)
//		z[1] = byte(extensionRenegotiationInfo & 0xff)
//		z[2] = 0
//		z[3] = byte(len(m.secureRenegotiation) + 1)
//		z[4] = byte(len(m.secureRenegotiation))
//		z = z[5:]
//		copy(z, m.secureRenegotiation)
//		z = z[len(m.secureRenegotiation):]
//	}
//	if alpnLen := len(m.alpnProtocol); alpnLen > 0 {
//		z[0] = byte(extensionALPN >> 8)
//		z[1] = byte(extensionALPN & 0xff)
//		l := 2 + 1 + alpnLen
//		z[2] = byte(l >> 8)
//		z[3] = byte(l)
//		l -= 2
//		z[4] = byte(l >> 8)
//		z[5] = byte(l)
//		l -= 1
//		z[6] = byte(l)
//		copy(z[7:], []byte(m.alpnProtocol))
//		z = z[7+alpnLen:]
//	}
//	if sctLen > 0 {
//		z[0] = byte(extensionSCT >> 8)
//		z[1] = byte(extensionSCT)
//		l := sctLen + 2
//		z[2] = byte(l >> 8)
//		z[3] = byte(l)
//		z[4] = byte(sctLen >> 8)
//		z[5] = byte(sctLen)
//
//		z = z[6:]
//		for _, sct := range m.scts {
//			z[0] = byte(len(sct) >> 8)
//			z[1] = byte(len(sct))
//			copy(z[2:], sct)
//			z = z[len(sct)+2:]
//		}
//	}
//
//	m.raw = x
//
//	return x
//}
//
//func (m *serverHelloMsg) unmarshal(data []byte) bool {
//	if len(data) < 42 {
//		return false
//	}
//	m.raw = data
//	m.vers = uint16(data[4])<<8 | uint16(data[5])
//	m.random = data[6:38]
//	sessionIdLen := int(data[38])
//	if sessionIdLen > 32 || len(data) < 39+sessionIdLen {
//		return false
//	}
//	m.sessionId = data[39 : 39+sessionIdLen]
//	data = data[39+sessionIdLen:]
//	if len(data) < 3 {
//		return false
//	}
//	m.cipherSuite = uint16(data[0])<<8 | uint16(data[1])
//	m.compressionMethod = data[2]
//	data = data[3:]
//
//	m.nextProtoNeg = false
//	m.nextProtos = nil
//	m.ocspStapling = false
//	m.scts = nil
//	m.ticketSupported = false
//	m.alpnProtocol = ""
//
//	if len(data) == 0 {
//		// ServerHello is optionally followed by extension data
//		return true
//	}
//	if len(data) < 2 {
//		return false
//	}
//
//	extensionsLength := int(data[0])<<8 | int(data[1])
//	data = data[2:]
//	if len(data) != extensionsLength {
//		return false
//	}
//
//	for len(data) != 0 {
//		if len(data) < 4 {
//			return false
//		}
//		extension := uint16(data[0])<<8 | uint16(data[1])
//		length := int(data[2])<<8 | int(data[3])
//		data = data[4:]
//		if len(data) < length {
//			return false
//		}
//
//		switch extension {
//		case extensionNextProtoNeg:
//			m.nextProtoNeg = true
//			d := data[:length]
//			for len(d) > 0 {
//				l := int(d[0])
//				d = d[1:]
//				if l == 0 || l > len(d) {
//					return false
//				}
//				m.nextProtos = append(m.nextProtos, string(d[:l]))
//				d = d[l:]
//			}
//		case extensionStatusRequest:
//			if length > 0 {
//				return false
//			}
//			m.ocspStapling = true
//		case extensionSessionTicket:
//			if length > 0 {
//				return false
//			}
//			m.ticketSupported = true
//		case extensionRenegotiationInfo:
//			if length == 0 {
//				return false
//			}
//			d := data[:length]
//			l := int(d[0])
//			d = d[1:]
//			if l != len(d) {
//				return false
//			}
//
//			m.secureRenegotiation = d
//			m.secureRenegotiationSupported = true
//		case extensionALPN:
//			d := data[:length]
//			if len(d) < 3 {
//				return false
//			}
//			l := int(d[0])<<8 | int(d[1])
//			if l != len(d)-2 {
//				return false
//			}
//			d = d[2:]
//			l = int(d[0])
//			if l != len(d)-1 {
//				return false
//			}
//			d = d[1:]
//			if len(d) == 0 {
//				// ALPN protocols must not be empty.
//				return false
//			}
//			m.alpnProtocol = string(d)
//		case extensionSCT:
//			d := data[:length]
//
//			if len(d) < 2 {
//				return false
//			}
//			l := int(d[0])<<8 | int(d[1])
//			d = d[2:]
//			if len(d) != l || l == 0 {
//				return false
//			}
//
//			m.scts = make([][]byte, 0, 3)
//			for len(d) != 0 {
//				if len(d) < 2 {
//					return false
//				}
//				sctLen := int(d[0])<<8 | int(d[1])
//				d = d[2:]
//				if sctLen == 0 || len(d) < sctLen {
//					return false
//				}
//				m.scts = append(m.scts, d[:sctLen])
//				d = d[sctLen:]
//			}
//		}
//		data = data[length:]
//	}
//
//	return true
//}

type encryptedExtensionsMsg struct {
	raw          []byte
	alpnProtocol string
}

func (m *encryptedExtensionsMsg) marshal() []byte {
	if m.raw != nil {
		return m.raw
	}

	var b cryptobyte.Builder
	b.AddUint8(typeEncryptedExtensions)
	b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
		b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
			if len(m.alpnProtocol) > 0 {
				b.AddUint16(extensionALPN)
				b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
					b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
						b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
							b.AddBytes([]byte(m.alpnProtocol))
						})
					})
				})
			}
		})
	})

	var err error
	m.raw, err = b.Bytes()
	if err != nil {
		log.Errorf("encrypted extensions message marshal failed: %v", err)
	}
	return m.raw
}

func (m *encryptedExtensionsMsg) unmarshal(data []byte) bool {
	*m = encryptedExtensionsMsg{raw: data}
	s := cryptobyte.String(data)

	var extensions cryptobyte.String
	if !s.Skip(4) || // message type and uint24 length field
		!s.ReadUint16LengthPrefixed(&extensions) || !s.Empty() {
		return false
	}

	for !extensions.Empty() {
		var extension uint16
		var extData cryptobyte.String
		if !extensions.ReadUint16(&extension) ||
			!extensions.ReadUint16LengthPrefixed(&extData) {
			return false
		}

		switch extension {
		case extensionALPN:
			var protoList cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&protoList) || protoList.Empty() {
				return false
			}
			var proto cryptobyte.String
			if !protoList.ReadUint8LengthPrefixed(&proto) ||
				proto.Empty() || !protoList.Empty() {
				return false
			}
			m.alpnProtocol = string(proto)
		default:
			// Ignore unknown extensions.
			continue
		}

		if !extData.Empty() {
			return false
		}
	}

	return true
}

type endOfEarlyDataMsg struct{}

func (m *endOfEarlyDataMsg) marshal() []byte {
	x := make([]byte, 4)
	x[0] = typeEndOfEarlyData
	return x
}

func (m *endOfEarlyDataMsg) unmarshal(data []byte) bool {
	return len(data) == 4
}

type keyUpdateMsg struct {
	raw             []byte
	updateRequested bool
}

func (m *keyUpdateMsg) marshal() []byte {
	if m.raw != nil {
		return m.raw
	}

	var b cryptobyte.Builder
	b.AddUint8(typeKeyUpdate)
	b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
		if m.updateRequested {
			b.AddUint8(1)
		} else {
			b.AddUint8(0)
		}
	})

	var err error
	m.raw, err = b.Bytes()
	if err != nil {
		log.Errorf("key Udpate message marshal failed: %v", err)
	}
	return m.raw
}

func (m *keyUpdateMsg) unmarshal(data []byte) bool {
	m.raw = data
	s := cryptobyte.String(data)

	var updateRequested uint8
	if !s.Skip(4) || // message type and uint24 length field
		!s.ReadUint8(&updateRequested) || !s.Empty() {
		return false
	}
	switch updateRequested {
	case 0:
		m.updateRequested = false
	case 1:
		m.updateRequested = true
	default:
		return false
	}
	return true
}

type newSessionTicketMsgTLS13 struct {
	raw          []byte
	lifetime     uint32
	ageAdd       uint32
	nonce        []byte
	label        []byte
	maxEarlyData uint32
}

func (m *newSessionTicketMsgTLS13) marshal() []byte {
	if m.raw != nil {
		return m.raw
	}

	var b cryptobyte.Builder
	b.AddUint8(typeNewSessionTicket)
	b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
		b.AddUint32(m.lifetime)
		b.AddUint32(m.ageAdd)
		b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
			b.AddBytes(m.nonce)
		})
		b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
			b.AddBytes(m.label)
		})

		b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
			if m.maxEarlyData > 0 {
				b.AddUint16(extensionEarlyData)
				b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
					b.AddUint32(m.maxEarlyData)
				})
			}
		})
	})

	var err error
	m.raw, err = b.Bytes()
	if err != nil {
		log.Errorf("new session ticket message marshal failed: %v", err)
	}
	return m.raw
}

func (m *newSessionTicketMsgTLS13) unmarshal(data []byte) bool {
	*m = newSessionTicketMsgTLS13{raw: data}
	s := cryptobyte.String(data)

	var extensions cryptobyte.String
	if !s.Skip(4) || // message type and uint24 length field
		!s.ReadUint32(&m.lifetime) ||
		!s.ReadUint32(&m.ageAdd) ||
		!readUint8LengthPrefixed(&s, &m.nonce) ||
		!readUint16LengthPrefixed(&s, &m.label) ||
		!s.ReadUint16LengthPrefixed(&extensions) ||
		!s.Empty() {
		return false
	}

	for !extensions.Empty() {
		var extension uint16
		var extData cryptobyte.String
		if !extensions.ReadUint16(&extension) ||
			!extensions.ReadUint16LengthPrefixed(&extData) {
			return false
		}

		switch extension {
		case extensionEarlyData:
			if !extData.ReadUint32(&m.maxEarlyData) {
				return false
			}
		default:
			// Ignore unknown extensions.
			continue
		}

		if !extData.Empty() {
			return false
		}
	}

	return true
}

type certificateRequestMsgTLS13 struct {
	raw                              []byte
	ocspStapling                     bool
	scts                             bool
	supportedSignatureAlgorithms     []SignatureScheme
	supportedSignatureAlgorithmsCert []SignatureScheme
	certificateAuthorities           [][]byte
}

func (m *certificateRequestMsgTLS13) marshal() ([]byte, error) {
	if m.raw != nil {
		return m.raw, nil
	}

	var b cryptobyte.Builder
	b.AddUint8(typeCertificateRequest)
	b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
		// certificate_request_context (SHALL be zero length unless used for
		// post-handshake authentication)
		b.AddUint8(0)

		b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
			if m.ocspStapling {
				b.AddUint16(extensionStatusRequest)
				b.AddUint16(0) // empty extension_data
			}
			if m.scts {
				// RFC 8446, Section 4.4.2.1 makes no mention of
				// signed_certificate_timestamp in CertificateRequest, but
				// "Extensions in the Certificate message from the client MUST
				// correspond to extensions in the CertificateRequest message
				// from the server." and it appears in the table in Section 4.2.
				b.AddUint16(extensionSCT)
				b.AddUint16(0) // empty extension_data
			}
			if len(m.supportedSignatureAlgorithms) > 0 {
				b.AddUint16(extensionSignatureAlgorithms)
				b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
					b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
						for _, sigAlgo := range m.supportedSignatureAlgorithms {
							b.AddUint16(uint16(sigAlgo))
						}
					})
				})
			}
			if len(m.supportedSignatureAlgorithmsCert) > 0 {
				b.AddUint16(extensionSignatureAlgorithmsCert)
				b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
					b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
						for _, sigAlgo := range m.supportedSignatureAlgorithmsCert {
							b.AddUint16(uint16(sigAlgo))
						}
					})
				})
			}
			if len(m.certificateAuthorities) > 0 {
				b.AddUint16(extensionCertificateAuthorities)
				b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
					b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
						for _, ca := range m.certificateAuthorities {
							b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
								b.AddBytes(ca)
							})
						}
					})
				})
			}
		})
	})

	var err error
	m.raw, err = b.Bytes()
	return m.raw, err
}

func (m *certificateRequestMsgTLS13) unmarshal(data []byte) bool {
	*m = certificateRequestMsgTLS13{raw: data}
	s := cryptobyte.String(data)

	var context, extensions cryptobyte.String
	if !s.Skip(4) || // message type and uint24 length field
		!s.ReadUint8LengthPrefixed(&context) || !context.Empty() ||
		!s.ReadUint16LengthPrefixed(&extensions) ||
		!s.Empty() {
		return false
	}

	for !extensions.Empty() {
		var extension uint16
		var extData cryptobyte.String
		if !extensions.ReadUint16(&extension) ||
			!extensions.ReadUint16LengthPrefixed(&extData) {
			return false
		}

		switch extension {
		case extensionStatusRequest:
			m.ocspStapling = true
		case extensionSCT:
			m.scts = true
		case extensionSignatureAlgorithms:
			var sigAndAlgs cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&sigAndAlgs) || sigAndAlgs.Empty() {
				return false
			}
			for !sigAndAlgs.Empty() {
				var sigAndAlg uint16
				if !sigAndAlgs.ReadUint16(&sigAndAlg) {
					return false
				}
				m.supportedSignatureAlgorithms = append(
					m.supportedSignatureAlgorithms, SignatureScheme(sigAndAlg))
			}
		case extensionSignatureAlgorithmsCert:
			var sigAndAlgs cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&sigAndAlgs) || sigAndAlgs.Empty() {
				return false
			}
			for !sigAndAlgs.Empty() {
				var sigAndAlg uint16
				if !sigAndAlgs.ReadUint16(&sigAndAlg) {
					return false
				}
				m.supportedSignatureAlgorithmsCert = append(
					m.supportedSignatureAlgorithmsCert, SignatureScheme(sigAndAlg))
			}
		case extensionCertificateAuthorities:
			var auths cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&auths) || auths.Empty() {
				return false
			}
			for !auths.Empty() {
				var ca []byte
				if !readUint16LengthPrefixed(&auths, &ca) || len(ca) == 0 {
					return false
				}
				m.certificateAuthorities = append(m.certificateAuthorities, ca)
			}
		default:
			// Ignore unknown extensions.
			continue
		}

		if !extData.Empty() {
			return false
		}
	}

	return true
}

type certificateMsg struct {
	raw          []byte
	certificates [][]byte
}

func (m *certificateMsg) equal(i interface{}) bool {
	m1, ok := i.(*certificateMsg)
	if !ok {
		return false
	}

	return bytes.Equal(m.raw, m1.raw) &&
		eqByteSlices(m.certificates, m1.certificates)
}

func (m *certificateMsg) marshal() (x []byte) {
	if m.raw != nil {
		return m.raw
	}

	var i int
	for _, slice := range m.certificates {
		i += len(slice)
	}

	length := 3 + 3*len(m.certificates) + i
	x = make([]byte, 4+length)
	x[0] = typeCertificate
	x[1] = uint8(length >> 16)
	x[2] = uint8(length >> 8)
	x[3] = uint8(length)

	certificateOctets := length - 3
	x[4] = uint8(certificateOctets >> 16)
	x[5] = uint8(certificateOctets >> 8)
	x[6] = uint8(certificateOctets)

	y := x[7:]
	for _, slice := range m.certificates {
		y[0] = uint8(len(slice) >> 16)
		y[1] = uint8(len(slice) >> 8)
		y[2] = uint8(len(slice))
		copy(y[3:], slice)
		y = y[3+len(slice):]
	}

	m.raw = x
	return
}

func (m *certificateMsg) unmarshal(data []byte) bool {
	if len(data) < 7 {
		return false
	}

	m.raw = data
	certsLen := uint32(data[4])<<16 | uint32(data[5])<<8 | uint32(data[6])
	if uint32(len(data)) != certsLen+7 {
		return false
	}

	numCerts := 0
	d := data[7:]
	for certsLen > 0 {
		if len(d) < 4 {
			return false
		}
		certLen := uint32(d[0])<<16 | uint32(d[1])<<8 | uint32(d[2])
		if uint32(len(d)) < 3+certLen {
			return false
		}
		d = d[3+certLen:]
		certsLen -= 3 + certLen
		numCerts++
	}

	m.certificates = make([][]byte, numCerts)
	d = data[7:]
	for i := 0; i < numCerts; i++ {
		certLen := uint32(d[0])<<16 | uint32(d[1])<<8 | uint32(d[2])
		m.certificates[i] = d[3 : 3+certLen]
		d = d[3+certLen:]
	}

	return true
}

type certificateMsgTLS13 struct {
	raw          []byte
	certificate  Certificate
	ocspStapling bool
	scts         bool
}

func (m *certificateMsgTLS13) marshal() []byte {
	if m.raw != nil {
		return m.raw
	}

	var b cryptobyte.Builder
	b.AddUint8(typeCertificate)
	b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
		b.AddUint8(0) // certificate_request_context

		certificate := m.certificate
		if !m.ocspStapling {
			certificate.OCSPStaple = nil
		}
		if !m.scts {
			certificate.SignedCertificateTimestamps = nil
		}
		marshalCertificate(b, certificate)
	})

	var err error
	m.raw, err = b.Bytes()
	if err != nil {
		log.Errorf("certificate tls13 message marshal failed: %v", err)
	}
	return m.raw
}

func marshalCertificate(b *cryptobyte.Builder, certificate Certificate) {
	b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
		for i, cert := range certificate.Certificate {
			b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
				b.AddBytes(cert)
			})
			b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
				if i > 0 {
					// This library only supports OCSP and SCT for leaf certificates.
					return
				}
				if certificate.OCSPStaple != nil {
					b.AddUint16(extensionStatusRequest)
					b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
						b.AddUint8(statusTypeOCSP)
						b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
							b.AddBytes(certificate.OCSPStaple)
						})
					})
				}
				if certificate.SignedCertificateTimestamps != nil {
					b.AddUint16(extensionSCT)
					b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
						b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
							for _, sct := range certificate.SignedCertificateTimestamps {
								b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
									b.AddBytes(sct)
								})
							}
						})
					})
				}
			})
		}
	})
}

func (m *certificateMsgTLS13) unmarshal(data []byte) bool {
	*m = certificateMsgTLS13{raw: data}
	s := cryptobyte.String(data)

	var context cryptobyte.String
	if !s.Skip(4) || // message type and uint24 length field
		!s.ReadUint8LengthPrefixed(&context) || !context.Empty() ||
		!unmarshalCertificate(&s, &m.certificate) ||
		!s.Empty() {
		return false
	}

	m.scts = m.certificate.SignedCertificateTimestamps != nil
	m.ocspStapling = m.certificate.OCSPStaple != nil

	return true
}

func unmarshalCertificate(s *cryptobyte.String, certificate *Certificate) bool {
	var certList cryptobyte.String
	if !s.ReadUint24LengthPrefixed(&certList) {
		return false
	}
	for !certList.Empty() {
		var cert []byte
		var extensions cryptobyte.String
		if !readUint24LengthPrefixed(&certList, &cert) ||
			!certList.ReadUint16LengthPrefixed(&extensions) {
			return false
		}
		certificate.Certificate = append(certificate.Certificate, cert)
		for !extensions.Empty() {
			var extension uint16
			var extData cryptobyte.String
			if !extensions.ReadUint16(&extension) ||
				!extensions.ReadUint16LengthPrefixed(&extData) {
				return false
			}
			if len(certificate.Certificate) > 1 {
				// This library only supports OCSP and SCT for leaf certificates.
				continue
			}

			switch extension {
			case extensionStatusRequest:
				var statusType uint8
				if !extData.ReadUint8(&statusType) || statusType != statusTypeOCSP ||
					!readUint24LengthPrefixed(&extData, &certificate.OCSPStaple) ||
					len(certificate.OCSPStaple) == 0 {
					return false
				}
			case extensionSCT:
				var sctList cryptobyte.String
				if !extData.ReadUint16LengthPrefixed(&sctList) || sctList.Empty() {
					return false
				}
				for !sctList.Empty() {
					var sct []byte
					if !readUint16LengthPrefixed(&sctList, &sct) ||
						len(sct) == 0 {
						return false
					}
					certificate.SignedCertificateTimestamps = append(
						certificate.SignedCertificateTimestamps, sct)
				}
			default:
				// Ignore unknown extensions.
				continue
			}

			if !extData.Empty() {
				return false
			}
		}
	}
	return true
}

type serverKeyExchangeMsg struct {
	raw []byte
	key []byte
}

func (m *serverKeyExchangeMsg) equal(i interface{}) bool {
	m1, ok := i.(*serverKeyExchangeMsg)
	if !ok {
		return false
	}

	return bytes.Equal(m.raw, m1.raw) &&
		bytes.Equal(m.key, m1.key)
}

func (m *serverKeyExchangeMsg) marshal() []byte {
	if m.raw != nil {
		return m.raw
	}
	length := len(m.key)
	x := make([]byte, length+4)
	x[0] = typeServerKeyExchange
	x[1] = uint8(length >> 16)
	x[2] = uint8(length >> 8)
	x[3] = uint8(length)
	copy(x[4:], m.key)

	m.raw = x
	return x
}

func (m *serverKeyExchangeMsg) unmarshal(data []byte) bool {
	m.raw = data
	if len(data) < 4 {
		return false
	}
	m.key = data[4:]
	return true
}

type certificateStatusMsg struct {
	raw        []byte
	statusType uint8
	response   []byte
}

func (m *certificateStatusMsg) equal(i interface{}) bool {
	m1, ok := i.(*certificateStatusMsg)
	if !ok {
		return false
	}

	return bytes.Equal(m.raw, m1.raw) &&
		m.statusType == m1.statusType &&
		bytes.Equal(m.response, m1.response)
}

func (m *certificateStatusMsg) marshal() []byte {
	if m.raw != nil {
		return m.raw
	}

	var x []byte
	if m.statusType == statusTypeOCSP {
		x = make([]byte, 4+4+len(m.response))
		x[0] = typeCertificateStatus
		l := len(m.response) + 4
		x[1] = byte(l >> 16)
		x[2] = byte(l >> 8)
		x[3] = byte(l)
		x[4] = statusTypeOCSP

		l -= 4
		x[5] = byte(l >> 16)
		x[6] = byte(l >> 8)
		x[7] = byte(l)
		copy(x[8:], m.response)
	} else {
		x = []byte{typeCertificateStatus, 0, 0, 1, m.statusType}
	}

	m.raw = x
	return x
}

func (m *certificateStatusMsg) unmarshal(data []byte) bool {
	m.raw = data
	if len(data) < 5 {
		return false
	}
	m.statusType = data[4]

	m.response = nil
	if m.statusType == statusTypeOCSP {
		if len(data) < 8 {
			return false
		}
		respLen := uint32(data[5])<<16 | uint32(data[6])<<8 | uint32(data[7])
		if uint32(len(data)) != 4+4+respLen {
			return false
		}
		m.response = data[8:]
	}
	return true
}

type serverHelloDoneMsg struct{}

func (m *serverHelloDoneMsg) equal(i interface{}) bool {
	_, ok := i.(*serverHelloDoneMsg)
	return ok
}

func (m *serverHelloDoneMsg) marshal() []byte {
	x := make([]byte, 4)
	x[0] = typeServerHelloDone
	return x
}

func (m *serverHelloDoneMsg) unmarshal(data []byte) bool {
	return len(data) == 4
}

type clientKeyExchangeMsg struct {
	raw        []byte
	ciphertext []byte
}

func (m *clientKeyExchangeMsg) equal(i interface{}) bool {
	m1, ok := i.(*clientKeyExchangeMsg)
	if !ok {
		return false
	}

	return bytes.Equal(m.raw, m1.raw) &&
		bytes.Equal(m.ciphertext, m1.ciphertext)
}

func (m *clientKeyExchangeMsg) marshal() []byte {
	if m.raw != nil {
		return m.raw
	}
	length := len(m.ciphertext)
	x := make([]byte, length+4)
	x[0] = typeClientKeyExchange
	x[1] = uint8(length >> 16)
	x[2] = uint8(length >> 8)
	x[3] = uint8(length)
	copy(x[4:], m.ciphertext)

	m.raw = x
	return x
}

func (m *clientKeyExchangeMsg) unmarshal(data []byte) bool {
	m.raw = data
	if len(data) < 4 {
		return false
	}
	l := int(data[1])<<16 | int(data[2])<<8 | int(data[3])
	if l != len(data)-4 {
		return false
	}
	m.ciphertext = data[4:]
	return true
}

type finishedMsg struct {
	raw        []byte
	verifyData []byte
}

func (m *finishedMsg) equal(i interface{}) bool {
	m1, ok := i.(*finishedMsg)
	if !ok {
		return false
	}

	return bytes.Equal(m.raw, m1.raw) &&
		bytes.Equal(m.verifyData, m1.verifyData)
}

func (m *finishedMsg) marshal() (x []byte) {
	if m.raw != nil {
		return m.raw
	}

	x = make([]byte, 4+len(m.verifyData))
	x[0] = typeFinished
	x[3] = byte(len(m.verifyData))
	copy(x[4:], m.verifyData)
	m.raw = x
	return
}

func (m *finishedMsg) unmarshal(data []byte) bool {
	m.raw = data
	if len(data) < 4 {
		return false
	}
	m.verifyData = data[4:]
	return true
}

type nextProtoMsg struct {
	raw   []byte
	proto string
}

func (m *nextProtoMsg) equal(i interface{}) bool {
	m1, ok := i.(*nextProtoMsg)
	if !ok {
		return false
	}

	return bytes.Equal(m.raw, m1.raw) &&
		m.proto == m1.proto
}

func (m *nextProtoMsg) marshal() []byte {
	if m.raw != nil {
		return m.raw
	}
	l := len(m.proto)
	if l > 255 {
		l = 255
	}

	padding := 32 - (l+2)%32
	length := l + padding + 2
	x := make([]byte, length+4)
	x[0] = typeNextProtocol
	x[1] = uint8(length >> 16)
	x[2] = uint8(length >> 8)
	x[3] = uint8(length)

	y := x[4:]
	y[0] = byte(l)
	copy(y[1:], []byte(m.proto[0:l]))
	y = y[1+l:]
	y[0] = byte(padding)

	m.raw = x

	return x
}

func (m *nextProtoMsg) unmarshal(data []byte) bool {
	m.raw = data

	if len(data) < 5 {
		return false
	}
	data = data[4:]
	protoLen := int(data[0])
	data = data[1:]
	if len(data) < protoLen {
		return false
	}
	m.proto = string(data[0:protoLen])
	data = data[protoLen:]

	if len(data) < 1 {
		return false
	}
	paddingLen := int(data[0])
	data = data[1:]
	if len(data) != paddingLen {
		return false
	}

	return true
}

type certificateRequestMsg struct {
	raw []byte
	// hasSignatureAndHash indicates whether this message includes a list
	// of signature and hash functions. This change was introduced with TLS
	// 1.2.
	hasSignatureAndHash bool

	certificateTypes             []byte
	supportedSignatureAlgorithms []SignatureScheme
	certificateAuthorities       [][]byte
}

func (m *certificateRequestMsg) equal(i interface{}) bool {
	m1, ok := i.(*certificateRequestMsg)
	if !ok {
		return false
	}

	return bytes.Equal(m.raw, m1.raw) &&
		bytes.Equal(m.certificateTypes, m1.certificateTypes) &&
		eqByteSlices(m.certificateAuthorities, m1.certificateAuthorities) &&
		eqSignatureAlgorithms(m.supportedSignatureAlgorithms, m1.supportedSignatureAlgorithms)
}

func (m *certificateRequestMsg) marshal() (x []byte) {
	if m.raw != nil {
		return m.raw
	}

	// See http://tools.ietf.org/html/rfc4346#section-7.4.4
	length := 1 + len(m.certificateTypes) + 2
	casLength := 0
	for _, ca := range m.certificateAuthorities {
		casLength += 2 + len(ca)
	}
	length += casLength

	if m.hasSignatureAndHash {
		length += 2 + 2*len(m.supportedSignatureAlgorithms)
	}

	x = make([]byte, 4+length)
	x[0] = typeCertificateRequest
	x[1] = uint8(length >> 16)
	x[2] = uint8(length >> 8)
	x[3] = uint8(length)

	x[4] = uint8(len(m.certificateTypes))

	copy(x[5:], m.certificateTypes)
	y := x[5+len(m.certificateTypes):]

	if m.hasSignatureAndHash {
		n := len(m.supportedSignatureAlgorithms) * 2
		y[0] = uint8(n >> 8)
		y[1] = uint8(n)
		y = y[2:]
		for _, sigAlgo := range m.supportedSignatureAlgorithms {
			y[0] = uint8(sigAlgo >> 8)
			y[1] = uint8(sigAlgo)
			y = y[2:]
		}
	}

	y[0] = uint8(casLength >> 8)
	y[1] = uint8(casLength)
	y = y[2:]
	for _, ca := range m.certificateAuthorities {
		y[0] = uint8(len(ca) >> 8)
		y[1] = uint8(len(ca))
		y = y[2:]
		copy(y, ca)
		y = y[len(ca):]
	}

	m.raw = x
	return
}

func (m *certificateRequestMsg) unmarshal(data []byte) bool {
	m.raw = data

	if len(data) < 5 {
		return false
	}

	length := uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	if uint32(len(data))-4 != length {
		return false
	}

	numCertTypes := int(data[4])
	data = data[5:]
	if numCertTypes == 0 || len(data) <= numCertTypes {
		return false
	}

	m.certificateTypes = make([]byte, numCertTypes)
	if copy(m.certificateTypes, data) != numCertTypes {
		return false
	}

	data = data[numCertTypes:]

	if m.hasSignatureAndHash {
		if len(data) < 2 {
			return false
		}
		sigAndHashLen := uint16(data[0])<<8 | uint16(data[1])
		data = data[2:]
		if sigAndHashLen&1 != 0 {
			return false
		}
		if len(data) < int(sigAndHashLen) {
			return false
		}
		numSigAlgos := sigAndHashLen / 2
		m.supportedSignatureAlgorithms = make([]SignatureScheme, numSigAlgos)
		for i := range m.supportedSignatureAlgorithms {
			m.supportedSignatureAlgorithms[i] = SignatureScheme(data[0])<<8 | SignatureScheme(data[1])
			data = data[2:]
		}
	}

	if len(data) < 2 {
		return false
	}
	casLength := uint16(data[0])<<8 | uint16(data[1])
	data = data[2:]
	if len(data) < int(casLength) {
		return false
	}
	cas := make([]byte, casLength)
	copy(cas, data)
	data = data[casLength:]

	m.certificateAuthorities = nil
	for len(cas) > 0 {
		if len(cas) < 2 {
			return false
		}
		caLen := uint16(cas[0])<<8 | uint16(cas[1])
		cas = cas[2:]

		if len(cas) < int(caLen) {
			return false
		}

		m.certificateAuthorities = append(m.certificateAuthorities, cas[:caLen])
		cas = cas[caLen:]
	}

	return len(data) == 0
}

type certificateVerifyMsg struct {
	raw                 []byte
	hasSignatureAndHash bool
	signatureAlgorithm  SignatureScheme
	signature           []byte
}

func (m *certificateVerifyMsg) equal(i interface{}) bool {
	m1, ok := i.(*certificateVerifyMsg)
	if !ok {
		return false
	}

	return bytes.Equal(m.raw, m1.raw) &&
		m.hasSignatureAndHash == m1.hasSignatureAndHash &&
		m.signatureAlgorithm == m1.signatureAlgorithm &&
		bytes.Equal(m.signature, m1.signature)
}

func (m *certificateVerifyMsg) marshal() (x []byte) {
	if m.raw != nil {
		return m.raw
	}

	// See http://tools.ietf.org/html/rfc4346#section-7.4.8
	siglength := len(m.signature)
	length := 2 + siglength
	if m.hasSignatureAndHash {
		length += 2
	}
	x = make([]byte, 4+length)
	x[0] = typeCertificateVerify
	x[1] = uint8(length >> 16)
	x[2] = uint8(length >> 8)
	x[3] = uint8(length)
	y := x[4:]
	if m.hasSignatureAndHash {
		y[0] = uint8(m.signatureAlgorithm >> 8)
		y[1] = uint8(m.signatureAlgorithm)
		y = y[2:]
	}
	y[0] = uint8(siglength >> 8)
	y[1] = uint8(siglength)
	copy(y[2:], m.signature)

	m.raw = x

	return
}

func (m *certificateVerifyMsg) unmarshal(data []byte) bool {
	m.raw = data

	if len(data) < 6 {
		return false
	}

	length := uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	if uint32(len(data))-4 != length {
		return false
	}

	data = data[4:]
	if m.hasSignatureAndHash {
		m.signatureAlgorithm = SignatureScheme(data[0])<<8 | SignatureScheme(data[1])
		data = data[2:]
	}

	if len(data) < 2 {
		return false
	}
	siglength := int(data[0])<<8 + int(data[1])
	data = data[2:]
	if len(data) != siglength {
		return false
	}

	m.signature = data

	return true
}

type newSessionTicketMsg struct {
	raw    []byte
	ticket []byte
}

func (m *newSessionTicketMsg) equal(i interface{}) bool {
	m1, ok := i.(*newSessionTicketMsg)
	if !ok {
		return false
	}

	return bytes.Equal(m.raw, m1.raw) &&
		bytes.Equal(m.ticket, m1.ticket)
}

func (m *newSessionTicketMsg) marshal() (x []byte) {
	if m.raw != nil {
		return m.raw
	}

	// See http://tools.ietf.org/html/rfc5077#section-3.3
	ticketLen := len(m.ticket)
	length := 2 + 4 + ticketLen
	x = make([]byte, 4+length)
	x[0] = typeNewSessionTicket
	x[1] = uint8(length >> 16)
	x[2] = uint8(length >> 8)
	x[3] = uint8(length)
	x[8] = uint8(ticketLen >> 8)
	x[9] = uint8(ticketLen)
	copy(x[10:], m.ticket)

	m.raw = x

	return
}

func (m *newSessionTicketMsg) unmarshal(data []byte) bool {
	m.raw = data

	if len(data) < 10 {
		return false
	}

	length := uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	if uint32(len(data))-4 != length {
		return false
	}

	ticketLen := int(data[8])<<8 + int(data[9])
	if len(data)-10 != ticketLen {
		return false
	}

	m.ticket = data[10:]

	return true
}

type helloRequestMsg struct {
}

func (*helloRequestMsg) marshal() []byte {
	return []byte{typeHelloRequest, 0, 0, 0}
}

func (*helloRequestMsg) unmarshal(data []byte) bool {
	return len(data) == 4
}

type transcriptHash interface {
	Write([]byte) (int, error)
}

// transcriptMsg is a helper used to marshal and hash messages which typically
// are not written to the wire, and as such aren't hashed during Conn.writeRecord.
func transcriptMsg(msg handshakeMessage, h transcriptHash) error {
	data := msg.marshal()
	h.Write(data)
	return nil
}

func eqUint16s(x, y []uint16) bool {
	if len(x) != len(y) {
		return false
	}
	for i, v := range x {
		if y[i] != v {
			return false
		}
	}
	return true
}

func eqCurveIDs(x, y []CurveID) bool {
	if len(x) != len(y) {
		return false
	}
	for i, v := range x {
		if y[i] != v {
			return false
		}
	}
	return true
}

func eqStrings(x, y []string) bool {
	if len(x) != len(y) {
		return false
	}
	for i, v := range x {
		if y[i] != v {
			return false
		}
	}
	return true
}

func eqByteSlices(x, y [][]byte) bool {
	if len(x) != len(y) {
		return false
	}
	for i, v := range x {
		if !bytes.Equal(v, y[i]) {
			return false
		}
	}
	return true
}

func eqSignatureAlgorithms(x, y []SignatureScheme) bool {
	if len(x) != len(y) {
		return false
	}
	for i, v := range x {
		if v != y[i] {
			return false
		}
	}
	return true
}
