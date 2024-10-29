package netx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"github.com/yaklang/yaklang/common/consts"
	"net"
	"time"

	"github.com/davecgh/go-spew/spew"
	utls "github.com/refraction-networking/utls"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

type HandshakeConn interface {
	net.Conn
	HandshakeContext(ctx context.Context) error
}

var (
	// tls Conn
	_ HandshakeConn = (*tls.Conn)(nil)
	// utls Conn
	_ HandshakeConn = (*utls.UConn)(nil)
	// gmtls Conn
	_ HandshakeConn = (*gmtls.Conn)(nil)
)

func UpgradeToTLSConnection(conn net.Conn, sni string, i any, spec *utls.ClientHelloSpec) (net.Conn, error) {
	return UpgradeToTLSConnectionWithTimeout(conn, sni, i, 10*time.Second, spec)
}

func UpgradeToTLSConnectionWithTimeout(conn net.Conn, sni string, i any, timeout time.Duration, spec *utls.ClientHelloSpec, tlsNextProto ...string) (net.Conn, error) {
	var handshakeConn HandshakeConn
	minVer, maxVer := consts.GetGlobalTLSVersion()
	if i == nil {
		i = &tls.Config{
			ServerName:         sni,
			MinVersion:         minVer,
			MaxVersion:         maxVer,
			InsecureSkipVerify: true,
			Renegotiation:      tls.RenegotiateFreelyAsClient,
		}
	}
	var (
		config      any
		gmtlsConfig *gmtls.Config
		tlsConfig   *tls.Config
		utlsConfig  *utls.Config
	)
	overrideNextProtos := len(tlsNextProto) > 0
	// i is a *tls.Config or *gmtls.Config
	switch ret := i.(type) {
	case *tls.Config:
		tlsConfig = ret
		config = tlsConfig
	case *gmtls.Config:
		gmtlsConfig = ret
		config = gmtlsConfig
	case *gmtls.GMSupport:

		gmtlsConfig = &gmtls.Config{
			GMSupport:          ret,
			ServerName:         sni,
			MinVersion:         minVer, // nolint[:staticcheck]
			MaxVersion:         maxVer,
			InsecureSkipVerify: true,
		}
		config = gmtlsConfig
	default:
		return nil, utils.Errorf("invalid tlsConfig type %T", i)
	}
	isCustomClientHello := spec != nil

	if tlsConfig != nil {
		if overrideNextProtos {
			tlsConfig.NextProtos = tlsNextProto
		}
		tlsConfig.Renegotiation = tls.RenegotiateFreelyAsClient

		if isCustomClientHello {
			utlsConfig = &utls.Config{
				ServerName:         sni,
				MinVersion:         minVer,
				MaxVersion:         maxVer,
				InsecureSkipVerify: true,
				Renegotiation:      utls.RenegotiateFreelyAsClient,
				NextProtos:         tlsConfig.NextProtos,
			}
			config = utlsConfig
		}

		err := LoadCertificatesConfig(config)
		if err != nil {
			log.Warnf("LoadCertificatesConfig(tlsConfig) error: %s", err)
		}

		if isCustomClientHello {
			spec := *spec

			uConn := utls.UClient(conn, utlsConfig, utls.HelloCustom)
			// if tlsNextProtos not contains h2, but spec contains, remove it
			if !lo.Contains(tlsNextProto, "h2") {
				for i, ext := range spec.Extensions {
					if _, ok := ext.(*utls.ALPNExtension); !ok {
						continue
					}
					old := spec.Extensions[i].(*utls.ALPNExtension).AlpnProtocols
					if !lo.Contains(old, "h2") {
						break
					}

					// force set ALPN
					spec.Extensions[i] = &utls.ALPNExtension{
						AlpnProtocols: tlsNextProto,
					}
					break
				}
			}

			err = uConn.ApplyPreset(&spec)
			if err != nil {
				return nil, utils.Wrap(err, "uConn.ApplyPreset error")
			}

			handshakeConn = uConn
		} else {
			handshakeConn = tls.Client(conn, tlsConfig)
		}
	} else if gmtlsConfig != nil {
		if overrideNextProtos {
			gmtlsConfig.NextProtos = tlsNextProto
		}
		gmtlsConfig.Renegotiation = gmtls.RenegotiateFreelyAsClient
		handshakeConn = gmtls.Client(conn, gmtlsConfig)
	}
	if handshakeConn == nil {
		return nil, utils.Errorf("invalid tlsConfig type %T", i)
	}

	err := handshakeConn.HandshakeContext(utils.TimeoutContext(timeout))
	if err != nil {
		conn.Close()
		return nil, err
	}

	return handshakeConn, nil
}

var (
	// presetClientCertificates is a list of certificates that will be used to
	// authenticate to the server if required.
	// load p12/pfx file to presetClientCertificates
	presetClientCertificates  []tls.Certificate
	presetUClientCertificates []utls.Certificate
	clientRootCA              = x509.NewCertPool()
)

func LoadP12Bytes(p12bytes []byte, password string) error {
	cCert, cKey, ca, err := tlsutils.LoadP12ToPEM(p12bytes, password)
	if err != nil {
		return err
	}
	{
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
	}
	{
		client, err := utls.X509KeyPair(cCert, cKey)
		if err != nil {
			return err
		}
		for _, caBytes := range ca {
			if !clientRootCA.AppendCertsFromPEM(caBytes) {
				log.Warn("append certs from pem failed")
				spew.Dump(caBytes)
			}
		}
		presetUClientCertificates = append(presetUClientCertificates, client)
	}

	return nil
}

func LoadCertificatesConfig(i any) error {
	switch ret := i.(type) {
	case *utls.Config:
		if len(ret.Certificates) > 0 {
			certs := make([]utls.Certificate, len(ret.Certificates), len(ret.Certificates)+len(presetClientCertificates))
			copy(certs, ret.Certificates)
			certs = append(certs, presetUClientCertificates...)
			ret.Certificates = certs
			ret.GetClientCertificate = func(info *utls.CertificateRequestInfo) (*utls.Certificate, error) {
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
			// 服务端请求客户端证书时，如果客户端没有配置证书，是否能完成握手取决于服务器的配置
			if len(presetClientCertificates) == 0 {
				return nil
			}
			ret.GetClientCertificate = func(info *utls.CertificateRequestInfo) (*utls.Certificate, error) {
				for _, cert := range presetUClientCertificates {
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
	case *tls.Config:
		if len(ret.Certificates) > 0 {
			certs := make([]tls.Certificate, len(ret.Certificates), len(ret.Certificates)+len(presetClientCertificates))
			copy(certs, ret.Certificates)
			certs = append(certs, presetClientCertificates...)
			ret.Certificates = certs
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
			// 服务端请求客户端证书时，如果客户端没有配置证书，是否能完成握手取决于服务器的配置
			if len(presetClientCertificates) == 0 {
				return nil
			}
			ret.GetClientCertificate = func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
				// 服务端请求客户端证书时，如果客户端没有配置证书，是否能完成握手取决于服务器的配置
				//if len(presetClientCertificates) == 0 {
				//	log.Warn("server request client certificate, but no client certificate configured")
				//	// sendClientCertificate 不允许发送 nil，否则会 panic 所以尝试发送一个空的证书
				//	// 这个解决方案可能会导致服务器拒绝握手，因为它可能会试图验证一个空的证书。
				//	// 如果服务器配置为VerifyClientCertIfGiven，并且它期望如果客户端提供了证书就必须是有效的，那么这个方法可能会失败。
				//	return &tls.Certificate{}, nil
				//}
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
