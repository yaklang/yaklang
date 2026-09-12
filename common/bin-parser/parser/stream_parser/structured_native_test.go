package stream_parser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStructuredNativeCompletePrograms(t *testing.T) {
	const source = "err = parseMQTTFields(4)\nif err != nil { panic(err) }\n"
	require.NotNil(t, StructuredDecoderForProgram(source))
	for _, bad := range []string{
		source + "panic(1)", "panic(1)\n" + source,
		"err = parseMQTTFields(level)\nif err != nil { panic(err) }\n",
		"err = parseMQTTFields(3+1)\nif err != nil { panic(err) }\n",
		"err = arbitraryNative(4)\nif err != nil { panic(err) }\n",
		"err = parseMQTTFields(5)\nif err != nil { panic(err) }\n",
	} {
		require.Nil(t, StructuredDecoderForProgram(bad))
	}
	decoder := StructuredDecoderForProgram(source)
	for _, wire := range [][]byte{nil, make([]byte, (1<<20)+1)} {
		value, info, err := decoder(wire)
		require.Error(t, err)
		require.Nil(t, value)
		require.Nil(t, info)
	}
}

func TestStructuredNativeTextProfiles(t *testing.T) {
	for _, family := range []struct {
		name     string
		fixtures map[string]string
		decode   func([]byte, string) ([]tlsCertificateField, map[string]any, error)
	}{
		{"parseIMAPFields", imapFieldsTestFixtures(), decodeIMAPFields},
		{"parsePOP3Fields", pop3TestFixtures(), decodePOP3Fields},
	} {
		for profile, message := range family.fixtures {
			t.Run(family.name+"/"+profile, func(t *testing.T) {
				wire := []byte(message)
				fields, metadata, err := family.decode(wire, profile)
				require.NoError(t, err)
				node := positionedResultNode(wire, 0)
				require.NoError(t, buildExactByteFieldTree(node, fields, metadata, 0, uint64(len(wire)*8), profile, "big"))
				want := NodeToMap(node)
				decoder := StructuredDecoderForProgram(fmt.Sprintf("err = %s(%q)\nif err != nil { panic(err) }\n", family.name, profile))
				require.NotNil(t, decoder)
				got, info, err := decoder(wire)
				require.NoError(t, err)
				require.Equal(t, want, got)
				require.Equal(t, metadata, info)
				clear(wire)
				require.Equal(t, want, got)
			})
		}
	}
}

func TestStructuredNativeLDAPOwnership(t *testing.T) {
	decoder := StructuredDecoderForProgram("err = parseLDAPFields(\"bind-request\")\nif err != nil { panic(err) }\n")
	for _, wire := range ldapFieldsTestFixtures() {
		original := bytes.Clone(wire)
		fields, metadata, err := decodeLDAPFields(original, "bind-request")
		require.NoError(t, err)
		node := positionedResultNode(original, 0)
		require.NoError(t, buildExactByteFieldTree(node, fields, metadata, 0, uint64(len(wire)*8), "ldap", "big"))
		got, info, err := decoder(wire)
		require.NoError(t, err)
		clear(wire)
		require.Equal(t, NodeToMap(node), got)
		require.Equal(t, metadata, info)
	}
}

func TestStructuredNativeTLSAndSMB(t *testing.T) {
	for _, tc := range []struct {
		name, endian string
		wire         []byte
		decode       byteFieldsDecoder
	}{
		{"parseTLSCertificateHandshake", "big", tlsCertificateTestMessage(), decodeTLSCertificateHandshake},
		{"parseTLSCertificateHandshake", "big", tlsCertificateTestMessage([]byte{0xff}, []byte{0x30, 0}), decodeTLSCertificateHandshake},
		{"parseSMB3TransformFields", "little", smb3TransformTestWire(), decodeSMB3TransformFields},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := bytes.Clone(tc.wire)
			fields, metadata, err := tc.decode(tc.wire)
			require.NoError(t, err)
			node := positionedResultNode(tc.wire, 0)
			require.NoError(t, buildExactByteFieldTree(node, fields, metadata, 0, uint64(len(wire)*8), tc.name, tc.endian))
			decoder := StructuredDecoderForProgram("err = " + tc.name + "()\nif err != nil { panic(err) }\n")
			require.NotNil(t, decoder)
			value, info, err := decoder(wire)
			require.NoError(t, err)
			clear(wire)
			require.Equal(t, NodeToMap(node), value)
			require.Equal(t, metadata, info)
		})
	}
}
