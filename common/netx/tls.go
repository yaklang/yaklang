package netx

import (
	"crypto/tls"
	"crypto/x509"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"net"
	"time"
)

func UpgradeToTLSConnection(conn net.Conn, sni string, i any) (net.Conn, error) {
	return UpgradeToTLSConnectionWithTimeout(conn, sni, i, 10*time.Second)
}

func UpgradeToTLSConnectionWithTimeout(conn net.Conn, sni string, i any, timeout time.Duration) (net.Conn, error) {
	if i == nil {
		i = &tls.Config{
			ServerName:         sni,
			MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
			MaxVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true,
			Renegotiation:      tls.RenegotiateFreelyAsClient,
		}
	}
	var gmtlsConfig *gmtls.Config
	var tlsConfig *tls.Config
	// i is a *tls.Config or *gmtls.Config
	switch ret := i.(type) {
	case *tls.Config:
		ret.Renegotiation = tls.RenegotiateFreelyAsClient
		tlsConfig = ret
	case *gmtls.Config:
		gmtlsConfig = ret
	case *gmtls.GMSupport:
		gmtlsConfig = &gmtls.Config{
			GMSupport:          ret,
			ServerName:         sni,
			MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
			MaxVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true,
		}
	default:
		return nil, utils.Errorf("invalid tlsConfig type %T", i)
	}

	if tlsConfig != nil {
		tlsConfig.Renegotiation = tls.RenegotiateFreelyAsClient
		err := LoadCertificatesConfig(tlsConfig)
		if err != nil {
			log.Warnf("LoadCertificatesConfig(tlsConfig) error: %s", err)
		}
		var sConn = tls.Client(conn, tlsConfig)
		err = sConn.HandshakeContext(utils.TimeoutContext(timeout))
		if err != nil {
			conn.Close()
			return nil, err
		}
		return sConn, nil
	} else if gmtlsConfig != nil {
		gmtlsConfig.Renegotiation = gmtls.RenegotiateFreelyAsClient
		var sConn = gmtls.Client(conn, gmtlsConfig)
		err := sConn.HandshakeContext(utils.TimeoutContext(timeout))
		if err != nil {
			conn.Close()
			return nil, err
		}
		return sConn, nil
	} else {
		return nil, utils.Errorf("invalid tlsConfig type %T", i)
	}
}

var (
	// presetClientCertificates is a list of certificates that will be used to
	// authenticate to the server if required.
	// load p12/pfx file to presetClientCertificates
	presetClientCertificates []tls.Certificate
	clientRootCA             = x509.NewCertPool()
)

func LoadP12Bytes(p12bytes []byte, password string) error {
	cCert, cKey, ca, err := tlsutils.LoadP12ToPEM(p12bytes, password)
	if err != nil {
		return err
	}
	client, err := tls.X509KeyPair(cCert, cKey)
	if err != nil {
		return err
	}
	for _, caBytes := range ca {
		if !clientRootCA.AppendCertsFromPEM(caBytes) {
			log.Warn("append certs from pem failed")
			spew.Dump(caBytes)
		}
	}
	presetClientCertificates = append(presetClientCertificates, client)
	return nil
}

func LoadCertificatesConfig(i any) error {
	switch ret := i.(type) {
	case *tls.Config:
		if len(ret.Certificates) > 0 {
			certs := make([]tls.Certificate, len(ret.Certificates), len(ret.Certificates)+len(presetClientCertificates))
			copy(certs, ret.Certificates)
			certs = append(certs, presetClientCertificates...)
			ret.Certificates = make([]tls.Certificate, 0)
			ret.GetClientCertificate = func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
				for _, cert := range certs {
					err := info.SupportsCertificate(&cert)
					if err != nil {
						continue
					}
					return &cert, nil
				}
				return nil, utils.Errorf("all [%v] certificates are tested, no one is supported for %v", len(certs), info.Version)
			}
		} else {
			ret.GetClientCertificate = func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
				for _, cert := range presetClientCertificates {
					err := info.SupportsCertificate(&cert)
					if err != nil {
						continue
					}
					return &cert, nil
				}
				return nil, utils.Errorf("all [%v] certificates are tested, no one is supported for %v", len(presetClientCertificates), info.Version)
			}
		}
		return nil
	case *gmtls.Config:
		return nil
	default:
		log.Warnf("invalid tlsConfig type %T", i)
		return nil
	}
}
