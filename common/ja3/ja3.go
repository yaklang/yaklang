package ja3

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	tls "github.com/refraction-networking/utls"

	"github.com/yaklang/yaklang/common/netx"
)

func ParseTLSVersion(version string) *TLSVersion {
	var versionNum uint16
	_, err := fmt.Sscan(version, &versionNum)
	if err != nil {
		return &TLSVersion{
			Version:     uint16(versionNum),
			VersionName: "Unknown",
		}
	}
	switch versionNum {
	case VersionTLS10:
		return &TLSVersion{
			Version:     VersionTLS10,
			VersionName: "VersionTLS10",
		}
	case VersionTLS11:
		return &TLSVersion{
			Version:     VersionTLS11,
			VersionName: "VersionTLS11",
		}
	case VersionTLS12:
		return &TLSVersion{
			Version:     VersionTLS12,
			VersionName: "VersionTLS12",
		}
	case VersionTLS13:
		return &TLSVersion{
			Version:     VersionTLS13,
			VersionName: "VersionTLS13",
		}
	case VersionSSL30:
		return &TLSVersion{
			Version:     VersionSSL30,
			VersionName: "VersionSSL30",
		}
	}
	return &TLSVersion{
		Version:     uint16(versionNum),
		VersionName: "Unknown",
	}
}

func ParseCipherSuites(suite string) []*CipherSuite {
	suites := strings.Split(suite, "-")
	var cipherSuites []*CipherSuite
	for _, val := range suites {
		cipherSuites = append(cipherSuites, GetCipherSuiteByID(val))
	}
	return cipherSuites
}

func ParseExtensionsTypes(extension string) []*ExtensionsType {
	extensions := strings.Split(extension, "-")
	var extensionsTypes []*ExtensionsType
	for _, val := range extensions {
		extensionsTypes = append(extensionsTypes, GetExtensionByType(val))
	}
	return extensionsTypes
}

func ParseEllipticCurves(curve string) []*EllipticCurve {
	curves := strings.Split(curve, "-")
	var ellipticCurves []*EllipticCurve
	for _, val := range curves {
		ellipticCurves = append(ellipticCurves, GetEllipticCurvesByID(val))
	}
	return ellipticCurves
}

func ParseEllipticCurvePointFormats(pointFormat string) []*EllipticCurvePointFormat {
	points := strings.Split(pointFormat, "-")
	var ellipticCurvePointFormats []*EllipticCurvePointFormat
	for _, val := range points {
		ellipticCurvePointFormats = append(ellipticCurvePointFormats, GetEllipticCurvePointFormatByID(val))
	}
	return ellipticCurvePointFormats
}

// ParseJA3ToClientHelloSpec 将 JA3 全字符串转换为 uTLS 的 ClientHelloSpec，用于模拟特定 TLS 客户端指纹
// 参数:
//   - str: JA3 全字符串，形如 "771,4865-4866-4867,0-23-65281,29-23-24,0"
//
// 返回值:
//   - 构造出的 *tls.ClientHelloSpec，可用于自定义 TLS 握手
//   - 错误信息，解析失败时非 nil
//
// Example:
// ```
// ja3str = "771,4865-4866-4867,0-23-65281,29-23-24,0"
// spec, err = ja3.ParseJA3ToClientHelloSpec(ja3str)
// assert err == nil, "should build client hello spec"
// assert spec != nil, "spec should not be nil"
// ```
func ParseJA3ToClientHelloSpec(str string) (*tls.ClientHelloSpec, error) {
	var (
		extensions string
		info       tls.ClientHelloInfo
		spec       tls.ClientHelloSpec
	)
	for i, field := range strings.SplitN(str, ",", 5) {
		switch i {
		case 0:
			// TLSVersMin is the record version, TLSVersMax is the handshake
			// version
			_, err := fmt.Sscan(field, &spec.TLSVersMax)
			if err != nil {
				return nil, err
			}
		case 1:
			// build CipherSuites
			for _, cipherKey := range strings.Split(field, "-") {
				var cipher uint16
				_, err := fmt.Sscan(cipherKey, &cipher)
				if err != nil {
					return nil, err
				}
				spec.CipherSuites = append(spec.CipherSuites, cipher)
			}
		case 2:
			extensions = field
		case 3:
			for _, curveKey := range strings.Split(field, "-") {
				var curve tls.CurveID
				_, err := fmt.Sscan(curveKey, &curve)
				if err != nil {
					return nil, err
				}
				info.SupportedCurves = append(info.SupportedCurves, curve)
			}
		case 4:
			for _, pointKey := range strings.Split(field, "-") {
				var point uint8
				_, err := fmt.Sscan(pointKey, &point)
				if err != nil {
					return nil, err
				}
				info.SupportedPoints = append(info.SupportedPoints, point)
			}
		}
	}
	// build extenions list
	for _, extKey := range strings.Split(extensions, "-") {
		var ext tls.TLSExtension
		switch extKey {
		case "0":
			// Android API 24
			ext = &tls.SNIExtension{}
		case "5":
			// Android API 26
			ext = &tls.StatusRequestExtension{}
		case "10":
			ext = &tls.SupportedCurvesExtension{info.SupportedCurves}
		case "11":
			ext = &tls.SupportedPointsExtension{info.SupportedPoints}
		case "13":
			ext = &tls.SignatureAlgorithmsExtension{
				SupportedSignatureAlgorithms: []tls.SignatureScheme{
					// Android API 24
					tls.ECDSAWithP256AndSHA256,
					// httpbin.org
					tls.PKCS1WithSHA256,
				},
			}
		case "16":
			ext = &tls.ALPNExtension{
				AlpnProtocols: []string{
					// Android API 24
					"http/1.1",
				},
			}
		case "23":
			// Android API 24
			ext = &tls.UtlsExtendedMasterSecretExtension{}
		case "43":
			// Android API 29
			ext = &tls.SupportedVersionsExtension{
				Versions: []uint16{tls.VersionTLS12},
			}
		case "45":
			// Android API 29
			ext = &tls.PSKKeyExchangeModesExtension{
				Modes: []uint8{tls.PskModeDHE},
			}
		case "65281":
			// Android API 24
			ext = &tls.RenegotiationInfoExtension{}
		default:
			var id uint16
			_, err := fmt.Sscan(extKey, &id)
			if err != nil {
				return nil, err
			}
			ext = &tls.GenericExtension{Id: id}
		}
		spec.Extensions = append(spec.Extensions, ext)
	}
	// uTLS does not support 0x0 as min version
	spec.TLSVersMin = tls.VersionTLS10
	return &spec, nil
}

// GetTransportByClientHelloSpec 根据给定的 ClientHelloSpec 构造一个使用该 TLS 指纹的 http.Transport
// 配合 ParseJA3ToClientHelloSpec 可让 HTTP 请求伪装成特定客户端的 TLS 指纹
// 参数:
//   - spec: TLS ClientHelloSpec，通常由 ja3.ParseJA3ToClientHelloSpec 生成
//
// 返回值:
//   - 使用指定 TLS 指纹的 *http.Transport
//
// Example:
// ```
// // 该示例为示意性用法：用 JA3 指纹构造可发起真实请求的 transport
// spec = ja3.ParseJA3ToClientHelloSpec("771,4865-4866-4867,0-23-65281,29-23-24,0")~
// transport = ja3.GetTransportByClientHelloSpec(spec)
// println(transport != nil)
// ```
func GetTransportByClientHelloSpec(spec *tls.ClientHelloSpec) *http.Transport {
	return &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := netx.DialContextWithoutProxy(ctx, addr)
			if err != nil {
				println("Error creating connection #123", err)
				return nil, err
			}
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			config := &tls.Config{ServerName: host}
			uconn := tls.UClient(conn, config, tls.HelloCustom)
			if err := uconn.ApplyPreset(spec); err != nil {
				return nil, err
			}
			if err := uconn.Handshake(); err != nil {
				return nil, err
			}
			return uconn, nil
		},
	}
}
