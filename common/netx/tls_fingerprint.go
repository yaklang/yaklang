package netx

import (
	"bytes"
	"crypto/ecdh"
	"fmt"
	"sort"

	"github.com/cloudflare/circl/kem/hybrid"
	utls "github.com/refraction-networking/utls"
)

const (
	TLSFingerprintChrome120 = "chrome-120"
	TLSFingerprintChrome151 = "chrome-151"

	DefaultTLSFingerprint = TLSFingerprintChrome151

	x25519MLKEM768 utls.CurveID = 4588

	signatureMLDSA44 utls.SignatureScheme = 0x0904
	signatureMLDSA65 utls.SignatureScheme = 0x0905
	signatureMLDSA87 utls.SignatureScheme = 0x0906
)

// ClientHelloProfile describes a built-in TLS fingerprint. A profile creates a
// fresh preset for every connection so ephemeral key shares are never reused.
type ClientHelloProfile struct {
	id          string
	chromeHTTP2 bool
	newPreset   func() (*clientHelloPreset, error)
}

type clientHelloPreset struct {
	spec       *utls.ClientHelloSpec
	applyState func(*utls.UConn) error
}

var clientHelloProfiles = map[string]*ClientHelloProfile{
	TLSFingerprintChrome120: {
		id:          TLSFingerprintChrome120,
		chromeHTTP2: true,
		newPreset:   newChrome120Preset,
	},
	TLSFingerprintChrome151: {
		id:          TLSFingerprintChrome151,
		chromeHTTP2: true,
		newPreset:   newChrome151Preset,
	},
}

func (p *ClientHelloProfile) ID() string {
	if p == nil {
		return ""
	}
	return p.id
}

func (p *ClientHelloProfile) UsesChromeHTTP2() bool {
	return p != nil && p.chromeHTTP2
}

func (p *ClientHelloProfile) newClientHelloPreset() (*clientHelloPreset, error) {
	if p == nil || p.newPreset == nil {
		return nil, fmt.Errorf("invalid TLS fingerprint profile")
	}
	return p.newPreset()
}

func GetClientHelloProfile(name string) (*ClientHelloProfile, error) {
	profile, ok := clientHelloProfiles[name]
	if !ok {
		return nil, fmt.Errorf("unknown TLS fingerprint profile %q (available: %v)", name, AvailableClientHelloProfiles())
	}
	return profile, nil
}

func AvailableClientHelloProfiles() []string {
	profiles := make([]string, 0, len(clientHelloProfiles))
	for name := range clientHelloProfiles {
		profiles = append(profiles, name)
	}
	sort.Strings(profiles)
	return profiles
}

func newChrome120Preset() (*clientHelloPreset, error) {
	spec, err := utls.UTLSIdToSpec(utls.HelloChrome_120)
	if err != nil {
		return nil, err
	}
	fixChromeGREASEECHCipherSuite(&spec)
	return &clientHelloPreset{spec: &spec}, nil
}

func newChrome151Preset() (*clientHelloPreset, error) {
	scheme := hybrid.X25519MLKEM768()
	publicKey, privateKey, err := scheme.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate X25519MLKEM768 key pair: %w", err)
	}
	publicShare, err := publicKey.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal X25519MLKEM768 public key: %w", err)
	}
	privateShare, err := privateKey.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal X25519MLKEM768 private key: %w", err)
	}
	if len(publicShare) < 32 || len(privateShare) < 32 {
		return nil, fmt.Errorf("invalid X25519MLKEM768 key sizes: public=%d private=%d", len(publicShare), len(privateShare))
	}

	// CIRCL serializes the hybrid key as ML-KEM followed by X25519. Reuse the
	// same X25519 ephemeral key in the standalone fallback share, as Chrome does.
	x25519Private, err := ecdh.X25519().NewPrivateKey(privateShare[len(privateShare)-32:])
	if err != nil {
		return nil, fmt.Errorf("restore X25519 private key: %w", err)
	}
	x25519Public := x25519Private.PublicKey()
	if !bytes.Equal(publicShare[len(publicShare)-32:], x25519Public.Bytes()) {
		return nil, fmt.Errorf("X25519 public key mismatch in hybrid key pair")
	}

	spec, err := utls.UTLSIdToSpec(utls.HelloChrome_120)
	if err != nil {
		return nil, err
	}
	for i, extension := range spec.Extensions {
		switch ext := extension.(type) {
		case *utls.SupportedCurvesExtension:
			ext.Curves = []utls.CurveID{
				utls.GREASE_PLACEHOLDER,
				x25519MLKEM768,
				utls.X25519,
				utls.CurveP256,
				utls.CurveP384,
			}
		case *utls.KeyShareExtension:
			ext.KeyShares = []utls.KeyShare{
				{Group: utls.GREASE_PLACEHOLDER, Data: []byte{0}},
				{Group: x25519MLKEM768, Data: publicShare},
				{Group: utls.X25519, Data: x25519Public.Bytes()},
			}
		case *utls.SignatureAlgorithmsExtension:
			ext.SupportedSignatureAlgorithms = append([]utls.SignatureScheme{
				signatureMLDSA44,
				signatureMLDSA65,
				signatureMLDSA87,
			}, ext.SupportedSignatureAlgorithms...)
		case *utls.ApplicationSettingsExtension:
			// ALPS moved from the old experimental codepoint 17513 to 17613.
			spec.Extensions[i] = &utls.ApplicationSettingsExtensionNew{
				SupportedProtocols: append([]string(nil), ext.SupportedProtocols...),
			}
		}
	}
	fixChromeGREASEECHCipherSuite(&spec)

	return &clientHelloPreset{
		spec: &spec,
		applyState: func(uConn *utls.UConn) error {
			params := uConn.HandshakeState.State13.KeySharesParams
			if params == nil {
				return fmt.Errorf("uTLS did not initialize TLS 1.3 key share state")
			}
			uConn.HandshakeState.State13.KEMKey = &utls.KemPrivateKey{
				SecretKey: privateKey,
				CurveID:   x25519MLKEM768,
			}
			params.AddEcdheKeypair(utls.X25519, x25519Private, x25519Public)
			return nil
		},
	}, nil
}

// uTLS before v1.8.1 randomly mixed AES-preferring outer ClientHello ciphers
// with ChaCha GREASE ECH. Pinning the ECH candidate to AES keeps the profile
// internally consistent with the AES-preferring Chrome preset.
func fixChromeGREASEECHCipherSuite(spec *utls.ClientHelloSpec) {
	for _, extension := range spec.Extensions {
		if ech, ok := extension.(*utls.GREASEEncryptedClientHelloExtension); ok {
			ech.CandidateCipherSuites = []utls.HPKESymmetricCipherSuite{{
				KdfId:  0x0001, // HKDF-SHA256
				AeadId: 0x0001, // AES-128-GCM
			}}
		}
	}
}
