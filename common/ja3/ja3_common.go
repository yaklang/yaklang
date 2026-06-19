package ja3

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// ParseJA3 解析 JA3 全字符串(由 5 个逗号分隔字段组成)，返回结构化的 JA3 指纹对象
// JA3 用于标识 TLS 客户端，字段依次为 TLS 版本、加密套件、扩展类型、椭圆曲线、椭圆曲线点格式
// 参数:
//   - ja3FullString: JA3 全字符串，形如 "771,4865-4866-4867,0-23-65281,29-23-24,0"
//
// 返回值:
//   - 解析得到的 JA3 指纹对象
//   - 错误信息，字段数量不为 5 时返回错误
//
// Example:
// ```
// ja3str = "771,4865-4866-4867,0-23-65281,29-23-24,0"
// obj, err = ja3.ParseJA3(ja3str)
// assert err == nil, "valid ja3 string should parse"
// // 加密套件字段含 3 个套件
// println(len(obj.CipherSuites))   // OUT: 3
// assert len(obj.CipherSuites) == 3, "should parse three cipher suites"
// ```
func ParseJA3(ja3FullString string) (*JA3, error) {
	fields := strings.Split(ja3FullString, ",")
	fieldLen := len(fields)
	if fieldLen != 5 {
		if fieldLen == 3 {
			return nil, errors.New("not a valid JA3 full string is it JA3S")
		}
		return nil, errors.New("not a valid JA3 full string")
	}
	ja3 := &JA3{}
	ja3.JA3FullStr = ja3FullString
	for index, field := range fields {
		if index == 0 { // TLS version field
			ja3.TLSVersion = ParseTLSVersion(field)
			continue
		}
		if index == 1 { // CipherSuites
			ja3.CipherSuites = ParseCipherSuites(field)
		}
		if index == 2 { // Extension Types
			ja3.ExtensionsTypes = ParseExtensionsTypes(field)
		}
		if index == 3 { // EllipticCurves
			ja3.EllipticCurves = ParseEllipticCurves(field)
		}
		if index == 4 { // EllipticCurvePointFormats
			ja3.EllipticCurvePointFormats = ParseEllipticCurvePointFormats(field)
		}
	}
	return ja3, nil
}

// ParseJA3S 解析 JA3S 全字符串(由 3 个逗号分隔字段组成)，返回结构化的 JA3S 指纹对象
// JA3S 用于标识 TLS 服务端，字段依次为 TLS 版本、选定加密套件、扩展类型
// 参数:
//   - ja3sFullString: JA3S 全字符串，形如 "771,4865,0-23"
//
// 返回值:
//   - 解析得到的 JA3S 指纹对象
//   - 错误信息，字段数量不为 3 时返回错误
//
// Example:
// ```
// ja3sstr = "771,4865,0-23"
// obj, err = ja3.ParseJA3S(ja3sstr)
// assert err == nil, "valid ja3s string should parse"
// // 扩展类型字段含 2 个扩展
// println(len(obj.ExtensionsTypes))   // OUT: 2
// assert len(obj.ExtensionsTypes) == 2, "should parse two extension types"
// ```
func ParseJA3S(ja3sFullString string) (*JA3S, error) {
	fields := strings.Split(ja3sFullString, ",")
	fieldLen := len(fields)
	if fieldLen != 3 {
		if fieldLen == 5 {
			return nil, errors.New("not a valid JA3S full string is it JA3")
		}
		return nil, errors.New("not a valid JA3S full string")
	}
	ja3s := &JA3S{}
	ja3s.JA3SFullStr = ja3sFullString
	for index, field := range fields {
		if index == 0 { // TLS version field
			ja3s.TLSVersion = ParseTLSVersion(field)
			continue
		}
		if index == 1 { // Accepted Cipher
			ja3s.AcceptedCipher = ParseCipherSuites(field)[0]
		}
		if index == 2 { // Extension Types
			ja3s.ExtensionsTypes = ParseExtensionsTypes(field)
		}

	}
	return ja3s, nil
}

type TLSVersion struct {
	Version     uint16
	VersionName string
}

// CipherSuite is a TLS cipher suite. Note that most functions in this package
// accept and expose cipher suite IDs instead of this type.
type CipherSuite struct {
	ID   uint16
	Name string

	// Supported versions is the list of TLS protocol versions that can
	// negotiate this cipher suite.
	SupportedVersions []uint16

	// Insecure is true if the cipher suite has known security issues
	// due to its primitives, design, or implementation.
	Insecure bool
}

type ExtensionsType struct {
	Type     uint16
	TypeName string
}

type EllipticCurve struct {
	CurveID   uint16
	CurveName string
}

type EllipticCurvePointFormat struct {
	CurvePoint           uint8
	CurvePointFormatName string
}

type JA3 struct {
	TLSVersion                *TLSVersion
	CipherSuites              []*CipherSuite
	ExtensionsTypes           []*ExtensionsType
	EllipticCurves            []*EllipticCurve
	EllipticCurvePointFormats []*EllipticCurvePointFormat
	JA3FullStr                string
}

func (j JA3) String() string {
	jsonBytes, err := json.Marshal(j)
	if err != nil {
		log.Error(err)
		return ""
	}
	return string(jsonBytes)
}

func (j JA3) Calc() string {
	return codec.Md5(j.JA3FullStr)
}

type JA3S struct {
	TLSVersion      *TLSVersion
	AcceptedCipher  *CipherSuite
	ExtensionsTypes []*ExtensionsType
	JA3SFullStr     string
}

func (j JA3S) String() string {
	jsonBytes, err := json.Marshal(j)
	if err != nil {
		log.Error(err)
		return ""
	}
	return string(jsonBytes)
}

func (j JA3S) Calc() string {
	return codec.Md5(j.JA3SFullStr)
}
