package tls

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"golang.org/x/crypto/cryptobyte"
)

func TestApplicationSettingsExtensionNewEncoding(t *testing.T) {
	extension := &ApplicationSettingsExtensionNew{SupportedProtocols: []string{"h2"}}
	encoded := make([]byte, extension.Len())
	if _, err := extension.Read(encoded); !errors.Is(err, io.EOF) {
		t.Fatalf("extension Read error = %v, want io.EOF", err)
	}
	want := []byte{0x44, 0xcd, 0x00, 0x05, 0x00, 0x03, 0x02, 'h', '2'}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("encoded ALPS 17613 = %x, want %x", encoded, want)
	}
}

func TestApplicationSettingsNewHandshakePlumbing(t *testing.T) {
	serverSettings := []byte{0x00, 0x04, 0x00, 0x60, 0x00, 0x00}
	serverExtensions := new(encryptedExtensionsMsg)
	if !serverExtensions.utlsUnmarshal(utlsExtensionApplicationSettingsNew, cryptobyte.String(serverSettings)) {
		t.Fatal("failed to parse server ALPS 17613")
	}
	if serverExtensions.utls.applicationSettingsCodepoint != utlsExtensionApplicationSettingsNew {
		t.Fatalf("server ALPS codepoint = %d", serverExtensions.utls.applicationSettingsCodepoint)
	}
	if !bytes.Equal(serverExtensions.utls.applicationSettings, serverSettings) {
		t.Fatalf("server ALPS settings = %x", serverExtensions.utls.applicationSettings)
	}

	clientSettings := []byte{0x00, 0x02, 0x00, 0x00, 0x00, 0x00}
	clientExtensions := &utlsClientEncryptedExtensionsMsg{
		applicationSettingsCodepoint: utlsExtensionApplicationSettingsNew,
		applicationSettings:          clientSettings,
	}
	encoded, err := clientExtensions.marshal()
	if err != nil {
		t.Fatal(err)
	}
	decoded := new(utlsClientEncryptedExtensionsMsg)
	if !decoded.unmarshal(encoded) {
		t.Fatalf("failed to parse client EncryptedExtensions: %x", encoded)
	}
	if decoded.applicationSettingsCodepoint != utlsExtensionApplicationSettingsNew {
		t.Fatalf("client ALPS codepoint = %d", decoded.applicationSettingsCodepoint)
	}
	if !bytes.Equal(decoded.applicationSettings, clientSettings) {
		t.Fatalf("client ALPS settings = %x", decoded.applicationSettings)
	}
}
