package codegrpc

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	_ "embed"
	"encoding/gob"
	"encoding/json"
	"github.com/yaklang/yaklang/common/gmsm/sm4"
	"hash"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/dlclark/regexp2"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/authhack"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/common/yserx"
)

//go:embed codec.gob.gzip
var codecDoc []byte

var (
	CodecLibs    *yakdoc.ScriptLib
	CodecLibsDoc []*ypb.CodecMethod // 记录函数的数据，参数类型等，用于前端生成样式
)

type outputType = string

var (
	OUTPUT_RAW    = "raw"
	OUTPUT_HEX    = "hex"
	OUTPUT_BASE64 = "base64"
)

func init() {
	buf, err := utils.GzipDeCompress(codecDoc)
	if err != nil {
		log.Warnf("load embed yak document error: %v", err)
	}
	var CodecDocumentHelper *yakdoc.DocumentHelper
	decoder := gob.NewDecoder(bytes.NewReader(buf))
	if err := decoder.Decode(&CodecDocumentHelper); err != nil {
		log.Warnf("load embed yak document error: %v", err)
	}
	CodecLibs = CodecDocumentHelper.StructMethods["github.com/yaklang/yaklang/common/yak/yaklib/codec/codegrpc.CodecExecFlow"]

	for funcName, funcInfo := range CodecLibs.Functions {
		var CodecMethod ypb.CodecMethod
		_, err = toml.Decode(funcInfo.Document, &CodecMethod)
		if err != nil {
			continue
		}
		CodecMethod.CodecMethod = funcName
		CodecLibsDoc = append(CodecLibsDoc, &CodecMethod)
	}

	mutate.AddFuzzTagToGlobal(&mutate.FuzzTagDescription{
		TagName: "codecflow",
		Handler: func(s string) []string {
			lastDividerIndex := strings.LastIndexByte(s, '|')
			if lastDividerIndex < 0 {
				return []string{}
			}
			flowName, input := s[:lastDividerIndex], s[lastDividerIndex+1:]
			codecFlow, err := yakit.GetCodecFlowByName(consts.GetGormProfileDatabase(), flowName)
			if err != nil {
				return []string{}
			}
			res, err := CodecFlowExec(&ypb.CodecRequestFlow{
				Text:       input,
				Auto:       false,
				WorkFlow:   codecFlow.ToGRPC().WorkFlow,
				InputBytes: nil,
			})
			if err != nil {
				return []string{}
			}
			return []string{res.GetResult()}
		},
		Description: "调用codec模块保存的codec flow，例如 {{codecflow(flowname|test)}}，其中flowname是保存的codecflow名，input是需要编码的输入",
	})
}

type CodecExecFlow struct {
	Text []byte
	Flow []*ypb.CodecWork
}

func NewCodecExecFlow(text []byte, flow []*ypb.CodecWork) *CodecExecFlow {
	return &CodecExecFlow{
		Text: text,
		Flow: flow,
	}
}

func decodeHexKeyAndIV(k string, i string) ([]byte, []byte, error) {
	key, err := codec.DecodeHex(k)
	if err != nil {
		return nil, nil, err
	}
	if funk.IsEmpty(key) {
		key = nil
	}

	iv, err := codec.DecodeHex(i)
	if err != nil {
		return nil, nil, err
	}
	if funk.IsEmpty(iv) {
		iv = nil
	}

	return key, iv, nil
}

func encodeData(text []byte, output outputType) []byte {
	switch output {
	case OUTPUT_RAW:
		return text
	case OUTPUT_HEX:
		return []byte(codec.EncodeToHex(text))
	case OUTPUT_BASE64:
		return []byte(codec.EncodeBase64(text))
	default:
		return text
	}
}

func decodeData(text []byte, input outputType) []byte {
	var data []byte
	var err error
	switch input {
	case OUTPUT_RAW:
		return text
	case OUTPUT_HEX:
		data, err = codec.DecodeHex(string(text))
		if err != nil {
			return text
		}
	case OUTPUT_BASE64:
		data, err = codec.DecodeBase64(string(text))
		if err != nil {
			return text
		}
	default:
		return text
	}
	if funk.IsEmpty(data) {
		return nil
	}
	return data
}

func padding(paddingType string, data []byte, size int) ([]byte, error) {
	switch paddingType {
	case "pkcs":
		return codec.PKCS5Padding(data, size), nil
	case "zeroPadding":
		return codec.ZeroPadding(data, size), nil
	default:
		return nil, utils.Error("unknown paddingType")
	}
}

func unPadding(paddingType string, data []byte) ([]byte, error) {
	switch paddingType {
	case "pkcs":
		return codec.PKCS5UnPadding(data), nil
	case "zeroPadding":
		return codec.ZeroUnPadding(data), nil
	default:
		return nil, utils.Error("unknown unPaddingType")
	}
}

// Tag = "加密"
// CodecName = "AES对称加密"
// Desc ="""高级加密标准（AES）是美国联邦信息处理标准（FIPS）。它是在一个历时5年的过程中，从15个竞争设计中选出的。
// Key：根据密钥的大小，将使用以下算法：
// 16字节 = AES-128
// 24字节 = AES-192
// 32字节 = AES-256
// 你可以使用其中一个KDF操作生成基于密码的密钥。"""
// Params = [
// { Name = "key", Type = "inputSelect", Required = true,Label = "Key", Connector ={ Name = "keyType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "key格式"} },
// { Name = "IV", Type = "inputSelect", Required = false ,Label = "IV", Connector ={ Name = "ivType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "IV格式"} },
// { Name = "mode", Type = "select", DefaultValue = "CBC",Options = ["CBC", "ECB", "CTR"], Required = true, Label = "Mode"},
// { Name = "output", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "输出格式"},
// { Name = "paddingType", Type = "select", DefaultValue = "pkcs", Options = ["pkcs", "zeroPadding"], Required = true,Label = "填充方式"}
// ]
func (flow *CodecExecFlow) AESEncrypt(key string, keyType string, IV string, ivType string, mode string, output outputType, paddingType string) error {
	inData, err := padding(paddingType, flow.Text, 16)
	if err != nil {
		return err
	}
	decodeKey := decodeData([]byte(key), keyType)
	decodeIV := decodeData([]byte(IV), ivType)
	if funk.IsEmpty(decodeIV) {
		decodeIV = decodeKey // if IV is empty, use key as IV
	}
	data, err := codec.AESEnc(decodeKey, inData, decodeIV, mode)
	if err == nil {
		flow.Text = encodeData(data, output)
	}
	return err
}

// Tag = "解密"
// CodecName = "AES对称解密"
// Desc = """高级加密标准（AES）是美国联邦信息处理标准（FIPS）。它是在一个历时5年的过程中，从15个竞争设计中选出的。
// Key：根据密钥的大小，将使用以下算法：
// 16字节 = AES-128
// 24字节 = AES-192
// 32字节 = AES-256"""
// Params = [
// { Name = "key", Type = "inputSelect", Required = true,Label = "Key", Connector ={ Name = "keyType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "key格式"} },
// { Name = "IV", Type = "inputSelect", Required = false ,Label = "IV", Connector ={ Name = "ivType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "IV格式"} },
// { Name = "mode", Type = "select", DefaultValue = "CBC",Options = ["CBC", "ECB", "CTR"], Required = true, Label = "Mode"},
// { Name = "input", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true,Label = "输入格式"},
// { Name = "paddingType", Type = "select", DefaultValue = "pkcs", Options = ["pkcs", "zeroPadding"], Required = true,Label = "填充方式"}
// ]
func (flow *CodecExecFlow) AESDecrypt(key string, keyType string, IV string, ivType string, mode string, input outputType, paddingType string) error {
	var err error
	decodeKey := decodeData([]byte(key), keyType)
	decodeIV := codec.FixIV(decodeData([]byte(IV), ivType), decodeKey, 16)
	inputText := decodeData(flow.Text, input)
	dec, err := codec.AESDec(decodeKey, inputText, decodeIV, mode)
	if err != nil {
		return err
	}
	dec, err = unPadding(paddingType, dec)
	if err != nil {
		return err
	}
	flow.Text = dec
	return nil
}

// Tag = "加密"
// CodecName = "SM4对称加密"
// Desc = """SM4是一个128位的块密码，目前被确定为中国的国家标准（GB/T 32907-2016）。支持多种块密码模式。"""
// Params = [
// { Name = "key", Type = "inputSelect", Required = true,Label = "Key", Connector ={ Name = "keyType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "key格式"} },
// { Name = "IV", Type = "inputSelect", Required = false ,Label = "IV", Connector ={ Name = "ivType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "IV格式"} },
// { Name = "mode", Type = "select", DefaultValue = "CBC",Options = ["CBC", "ECB", "CTR", "CFB", "OFB"], Required = true, Label = "Mode"},
// { Name = "output", Type = "select", DefaultValue = "hex", Options = ["hex", "raw","base64"], Required = true,Label = "输出格式"},
// { Name = "paddingType", Type = "select", DefaultValue = "pkcs", Options = ["pkcs", "zeroPadding"], Required = true,Label = "填充方式"}
// ]
func (flow *CodecExecFlow) SM4Encrypt(key string, keyType string, IV string, ivType string, mode string, output outputType, paddingType string) error {
	inData, err := padding(paddingType, flow.Text, 16)
	if err != nil {
		return err
	}
	decodeKey := decodeData([]byte(key), keyType)
	decodeIV := decodeData([]byte(IV), ivType)
	if funk.IsEmpty(decodeIV) {
		decodeIV = decodeKey // if IV is empty, use key as IV
	}
	data, err := codec.SM4Enc(decodeKey, inData, decodeIV, mode)
	if err == nil {
		flow.Text = encodeData(data, output)
	}
	return err
}

// Tag = "解密"
// CodecName = "SM4对称解密"
// Desc = """SM4是一个128位的块密码，目前被确定为中国的国家标准（GB/T 32907-2016）。支持多种块密码模式。"""
// Params = [
// { Name = "key", Type = "inputSelect", Required = true,Label = "Key", Connector ={ Name = "keyType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "key格式"} },
// { Name = "IV", Type = "inputSelect", Required = false ,Label = "IV", Connector ={ Name = "ivType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "IV格式"} },
// { Name = "mode", Type = "select",DefaultValue = "CBC", Options = ["CBC", "ECB", "CTR", "CFB", "OFB"], Required = true, Label = "Mode"},
// { Name = "input", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "输入格式"},
// { Name = "paddingType", Type = "select", DefaultValue = "pkcs", Options = ["pkcs", "zeroPadding"], Required = true,Label = "填充方式"}
// ]
func (flow *CodecExecFlow) SM4Decrypt(key string, keyType string, IV string, ivType string, mode string, input outputType, paddingType string) error {
	var err error
	decodeKey := decodeData([]byte(key), keyType)
	decodeIV := codec.FixIV(decodeData([]byte(IV), ivType), decodeKey, 16)
	inputText := decodeData(flow.Text, input)
	dec, err := codec.SM4Dec(decodeKey, inputText, decodeIV, mode)
	if err != nil {
		return err
	}
	dec, err = unPadding(paddingType, dec)
	if err != nil {
		return err
	}
	flow.Text = dec
	return nil
}

// Tag = "加密"
// CodecName = "DES对称加密"
// Desc = """DES（Data Encryption Standard）是一种对称密钥加密算法，使用固定有效长度为56位的密钥对数据进行64位的分组加密。尽管曾广泛使用，但由于密钥太短，现已被认为不够安全。"""
// Params = [
// { Name = "key", Type = "inputSelect", Required = true,Label = "Key", Connector ={ Name = "keyType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "key格式"} },
// { Name = "IV", Type = "inputSelect", Required = false ,Label = "IV", Connector ={ Name = "ivType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "IV格式"} },
// { Name = "mode", Type = "select",DefaultValue = "CBC", Options = ["CBC", "ECB"], Required = true , Label = "Mode"},
// { Name = "output", Type = "select", DefaultValue = "hex", Options = ["hex", "raw","base64"], Required = true,Label = "输出格式"},
// { Name = "paddingType", Type = "select", DefaultValue = "pkcs", Options = ["pkcs", "zeroPadding"], Required = true,Label = "填充方式"}
// ]
func (flow *CodecExecFlow) DESEncrypt(key string, keyType string, IV string, ivType string, mode string, output outputType, paddingType string) error {
	inData, err := padding(paddingType, flow.Text, 8)
	if err != nil {
		return err
	}
	decodeKey := decodeData([]byte(key), keyType)
	decodeIV := decodeData([]byte(IV), ivType)
	if funk.IsEmpty(decodeIV) {
		decodeIV = decodeKey // if IV is empty, use key as IV
	}
	data, err := codec.DESEnc(decodeKey, inData, decodeIV, mode)
	if err == nil {
		flow.Text = encodeData(data, output)
	}
	return err
}

// Tag = "解密"
// CodecName = "DES对称解密"
// Desc = """DES（Data Encryption Standard）是一种对称密钥加密算法，使用固定有效长度为56位的密钥对数据进行64位的分组加密。尽管曾广泛使用，但由于密钥太短，现已被认为不够安全。"""
// Params = [
// { Name = "key", Type = "inputSelect", Required = true,Label = "Key", Connector ={ Name = "keyType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "key格式"} },
// { Name = "IV", Type = "inputSelect", Required = false ,Label = "IV", Connector ={ Name = "ivType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "IV格式"} },
// { Name = "mode", Type = "select",DefaultValue = "CBC", Options = ["CBC", "ECB"], Required = true , Label = "Mode"},
// { Name = "input", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "输入格式"},
// { Name = "paddingType", Type = "select", DefaultValue = "pkcs", Options = ["pkcs", "zeroPadding"], Required = true,Label = "填充方式"}
// ]
func (flow *CodecExecFlow) DESDecrypt(key string, keyType string, IV string, ivType string, mode string, input outputType, paddingType string) error {
	decodeKey := decodeData([]byte(key), keyType)
	decodeIV := codec.FixIV(decodeData([]byte(IV), ivType), decodeKey, 8)
	inputText := decodeData(flow.Text, input)
	dec, err := codec.DESDec(decodeKey, inputText, decodeIV, mode)
	if err != nil {
		return err
	}
	dec, err = unPadding(paddingType, dec)
	if err != nil {
		return err
	}
	flow.Text = dec
	return nil
}

// Tag = "加密"
// CodecName = "TripleDES对称加密"
// Desc = """TripleDES（3DES）是DES的改进版，通过连续三次应用DES算法（可以使用三个不同的密钥）来增加加密的强度，提供了更高的安全性。"""
// Params = [
// { Name = "key", Type = "inputSelect", Required = true,Label = "Key", Connector ={ Name = "keyType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "key格式"} },
// { Name = "IV", Type = "inputSelect", Required = false ,Label = "IV", Connector ={ Name = "ivType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "IV格式"} },
// { Name = "mode", Type = "select",DefaultValue = "CBC", Options = ["CBC", "ECB"], Required = true, Label = "Mode"},
// { Name = "output", Type = "select",DefaultValue = "hex", Options = ["hex", "raw","base64"], Required = true ,Label = "输出格式"},
// { Name = "paddingType", Type = "select", DefaultValue = "pkcs", Options = ["pkcs", "zeroPadding"], Required = true,Label = "填充方式"}
// ]
func (flow *CodecExecFlow) TripleDESEncrypt(key string, keyType string, IV string, ivType string, mode string, output outputType, paddingType string) error {
	inData, err := padding(paddingType, flow.Text, 8)
	if err != nil {
		return err
	}
	decodeKey := decodeData([]byte(key), keyType)
	decodeIV := decodeData([]byte(IV), ivType)
	if funk.IsEmpty(decodeIV) && len(decodeKey) == 24 {
		decodeIV = decodeKey[:8] // if IV is empty, use key as IV
	}
	data, err := codec.TripleDesEnc(decodeKey, inData, decodeIV, mode)
	if err == nil {
		flow.Text = encodeData(data, output)
	}
	return err
}

// Tag = "解密"
// CodecName = "TripleDES对称解密"
// Desc = """TripleDES（3DES）是DES的改进版，通过连续三次应用DES算法（可以使用三个不同的密钥）来增加加密的强度，提供了更高的安全性。"""
// Params = [
// { Name = "key", Type = "inputSelect", Required = true,Label = "Key", Connector ={ Name = "keyType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "key格式"} },
// { Name = "IV", Type = "inputSelect", Required = false ,Label = "IV", Connector ={ Name = "ivType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "IV格式"} },
// { Name = "mode", Type = "select",DefaultValue = "CBC",  Options = ["CBC", "ECB"], Required = true , Label = "Mode"},
// { Name = "input", Type = "select",DefaultValue = "hex",  Options = ["hex", "raw", "base64"], Required = true ,Label = "输入格式"},
// { Name = "paddingType", Type = "select", DefaultValue = "pkcs", Options = ["pkcs", "zeroPadding"], Required = true,Label = "填充方式"}
// ]
func (flow *CodecExecFlow) TripleDESDecrypt(key string, keyType string, IV string, ivType string, mode string, input outputType, paddingType string) error {
	decodeKey := decodeData([]byte(key), keyType)
	decodeIV := codec.FixIV(decodeData([]byte(IV), ivType), decodeKey, 8)
	inputText := decodeData(flow.Text, input)
	dec, err := codec.TripleDesDec(decodeKey, inputText, decodeIV, mode)
	if err != nil {
		return err
	}
	dec, err = unPadding(paddingType, dec)
	if err != nil {
		return err
	}
	flow.Text = dec
	return nil
}

// Tag = "加密"
// CodecName = "RSA加密"
// Desc = """RSA加密算法是一种非对称加密算法，在公开密钥加密和电子商业中被广泛使用。RSA是被研究得最广泛的公钥算法，从提出后经历了各种攻击的考验，逐渐为人们接受，普遍认为是目前最优秀的公钥方案之一。"""
// Params = [
// { Name = "pubKey", Type = "text", Required = true,Label = "pem公钥"},
// { Name = "encryptSchema", Type = "select",DefaultValue = "RSA-OAEP", Options = ["RSA-OAEP", "PKCS1v15"], Required = true, Label = "填充方式"},
// { Name = "algorithm", Type = "select",DefaultValue = "SHA-256", Options = ["SHA-1", "SHA-256","SHA-384","SHA-512","MD5"], Required = true ,Label = "hash算法"}
// ]
func (flow *CodecExecFlow) RSAEncrypt(pubKey string, encryptSchema string, algorithm string) error {
	var data []byte
	var err error
	var hashFunc hash.Hash

	switch algorithm { // choose alg
	case "SHA-256":
		hashFunc = sha256.New()
	case "SHA-384":
		hashFunc = sha512.New384()
	case "SHA-512":
		hashFunc = sha512.New()
	case "MD5":
		hashFunc = md5.New()
	case "SHA-1":
		fallthrough
	default:
		hashFunc = sha1.New()
	}

	switch encryptSchema {
	case "RSA-OAEP":
		data, err = tlsutils.PemPkcsOAEPEncryptWithHash([]byte(pubKey), flow.Text, hashFunc)
	case "PKCS1v15":
		data, err = tlsutils.PemPkcs1v15Encrypt([]byte(pubKey), flow.Text)
	default:
		return utils.Error("RSA encrypt error: 未知的填充方式")
	}
	if err == nil {
		flow.Text = data
	}
	return err
}

// Tag = "解密"
// CodecName = "RSA解密"
// Desc = """RSA加密算法是一种非对称加密算法，在公开密钥加密和电子商业中被广泛使用。RSA是被研究得最广泛的公钥算法，从提出后经历了各种攻击的考验，逐渐为人们接受，普遍认为是目前最优秀的公钥方案之一。"""
// Params = [
// { Name = "priKey", Type = "text", Required = true,Label = "pem私钥"},
// { Name = "decryptSchema", Type = "select",DefaultValue = "RSA-OAEP", Options = ["RSA-OAEP", "PKCS1v15"], Required = true, Label = "填充方式"},
// { Name = "algorithm", Type = "select",DefaultValue = "SHA-256", Options = ["SHA-1", "SHA-256","SHA-384","SHA-512","MD5"], Required = true ,Label = "hash算法"}
// ]
func (flow *CodecExecFlow) RSADecrypt(priKey string, decryptSchema string, algorithm string) error {
	var data []byte
	var err error
	var hashFunc hash.Hash

	switch algorithm { // choose alg
	case "SHA-256":
		hashFunc = sha256.New()
	case "SHA-384":
		hashFunc = sha512.New384()
	case "SHA-512":
		hashFunc = sha512.New()
	case "MD5":
		hashFunc = md5.New()
	case "SHA-1":
		fallthrough
	default:
		hashFunc = sha1.New()
	}

	switch decryptSchema {
	case "RSA-OAEP":
		data, err = tlsutils.PemPkcsOAEPDecryptWithHash([]byte(priKey), flow.Text, hashFunc)
	case "PKCS1v15":
		data, err = tlsutils.PemPkcs1v15Decrypt([]byte(priKey), flow.Text)
	default:
		return utils.Error("RSA decrypt error: 未知的填充方式")
	}
	if err == nil {
		flow.Text = data
	}
	return err
}

// Tag = "Java"
// CodecName = "反序列化"
// Desc = """Java反序列化是一种将字节流转换为Java对象的机制，以便可以在网络上传输或将其保存到文件中。
// Yak中提供了两种反序列化方式： dumper 和 object-stream ，其中object-stream是Yak独有的一种伪代码表达形式，更直观易读"""
// Params = [
// { Name = "input", Type = "select",DefaultValue = "raw",  Options = ["raw", "hex", "base64"], Required = true , Label = "输入格式"},
// { Name = "output", Type = "select",DefaultValue = "dumper", Options = ["dumper", "object-stream"], Required = true , Label = "输出格式"}
// ]
func (flow *CodecExecFlow) JavaUnserialize(input string, output string) error {
	var err error
	raw := flow.Text
	switch input {
	case "raw":
		raw = flow.Text
	case "hex":
		raw, err = codec.DecodeHex(string(flow.Text))
		if err != nil {
			return err
		}
	case "base64":
		raw, err = codec.DecodeBase64(string(flow.Text))
		if err != nil {
			return err
		}
	default:
		return utils.Error("JavaUnserialize: unknown input mod")
	}

	switch output {
	case "dumper":
		raw = []byte(yserx.JavaSerializedDumper(raw))
	case "object-stream":
		objs, err := yserx.ParseJavaSerialized(raw)
		if err != nil {
			return err
		}
		raw, err = yserx.ToJson(objs)
		if err != nil {
			return err
		}
	}
	flow.Text = raw
	return nil
}

// Tag = "Java"
// CodecName = "序列化"
// Desc = """Java序列化是一种将Java对象转换为字节流的机制，以便可以在网络上传输或将其保存到文件中。 """
// Params = [
// { Name = "output", Type = "select",DefaultValue = "raw", Options = ["raw", "hex", "base64"], Required = true , Label = "输出格式"}
// ]
func (flow *CodecExecFlow) JavaSerialize(output string) error {
	var err error
	obj, err := yserx.FromJson(flow.Text)
	if err != nil {
		return err
	}
	raw := yserx.MarshalJavaObjects(obj...)
	switch output {
	case "raw":
		flow.Text = raw
	case "hex":
		flow.Text = []byte(codec.EncodeToHex(raw))
	case "base64":
		flow.Text = []byte(codec.EncodeBase64(raw))
	default:
		return utils.Error("JavaUnserialize: unknown input mod")
	}
	return nil
}

// Tag = "编码"
// CodecName = "base64编码"
// Desc = """Base64是一种基于64个可打印字符来表示二进制数据的表示方法。常用于在通常处理文本数据的场合，表示、传输、存储一些二进制数据，包括MIME的电子邮件及XML的一些复杂数据。
// eg: yak -> eWFr"""
// Params = [
// { Name = "Alphabet", Type = "select",DefaultValue = "standard", Options = ["standard", "urlsafe"], Required = true,Label = "Alphabet"}
// ]
func (flow *CodecExecFlow) Base64Encode(Alphabet string) error {
	switch Alphabet {
	case "standard":
		flow.Text = []byte(codec.EncodeBase64(flow.Text))
	case "urlsafe":
		flow.Text = []byte(codec.EncodeBase64Url(flow.Text))
	default:
		return utils.Error("Base64Encode: unknown alphabet")
	}
	return nil
}

// Tag = "解码"
// CodecName = "base64解码"
// Desc = """Base64是一种基于64个可打印字符来表示二进制数据的表示方法。常用于在通常处理文本数据的场合，表示、传输、存储一些二进制数据，包括MIME的电子邮件及XML的一些复杂数据。
// eg: eWFr -> yak"""
// Params = [
// { Name = "Alphabet", Type = "select",DefaultValue = "standard",Options = ["standard", "urlsafe"], Required = true,Lable = "Alphabet" }
// ]
func (flow *CodecExecFlow) Base64Decode(Alphabet string) error {
	var raw []byte
	var err error
	switch Alphabet {
	case "standard":
		raw, err = codec.DecodeBase64(string(flow.Text))
		if err != nil {
			return err
		}
	case "urlsafe":
		raw, err = codec.DecodeBase64Url(flow.Text)
		if err != nil {
			return err
		}
	default:
		return utils.Error("Base64Encode: unknown alphabet")
	}
	flow.Text = raw
	return nil
}

// Tag = "编码"
// CodecName = "HTML编码"
// Desc = """HTML编码是一种将特殊字符转换为HTML实体的编码方式。"""
// Params = [
// { Name = "entityRef", Type = "select",DefaultValue = "named", Options = ["dec", "hex", "named"], Required = true ,Label = "实体编码格式"},
// { Name = "fullEncode", Type = "checkbox", Required = true , Label = "全部编码"}
// ]
func (flow *CodecExecFlow) HtmlEncode(entityRef string, fullEncode bool) error {
	flow.Text = []byte(codec.EncodeHtmlEntityEx(flow.Text, entityRef, fullEncode))
	return nil
}

// Tag = "解码"
// CodecName = "HTML解码"
// Desc = """HTML编码是一种将特殊字符转换为HTML实体的编码方式。"""
func (flow *CodecExecFlow) HtmlDecode() error {
	flow.Text = []byte(codec.UnescapeHtmlString(string(flow.Text)))
	return nil
}

// Tag = "编码"
// CodecName = "URL编码"
// Desc = """URL编码，又称百分号编码，是一种互联网标准，用于将非ASCII字符、保留字符或任何可能在URL中产生歧义的字符转换为一个百分号后跟两位十六进制数的形式，以确保网络传输的无歧义性和安全性。"""
// Params = [
// { Name = "fullEncode", Type = "checkbox", Required = true , Label = "全部编码"}
// ]
func (flow *CodecExecFlow) URLEncode(fullEncode bool) error {
	if fullEncode {
		flow.Text = []byte(codec.EncodeUrlCode(flow.Text))
	} else {
		flow.Text = []byte(codec.QueryEscape(string(flow.Text)))
	}
	return nil
}

// Tag = "解码"
// CodecName = "URL解码"
// Desc = """URL编码，又称百分号编码，是一种互联网标准，用于将非ASCII字符、保留字符或任何可能在URL中产生歧义的字符转换为一个百分号后跟两位十六进制数的形式，以确保网络传输的无歧义性和安全性。"""
func (flow *CodecExecFlow) URLDecode() error {
	res, err := codec.QueryUnescape(string(flow.Text))
	if err != nil {
		return err
	}
	flow.Text = []byte(res)
	return nil
}

// Tag = "编码"
// CodecName = "十六进制编码"
// Desc = """十六进制编码是一种数字表示法，使用0到9和A到F共16个字符来表示数值。在计算机科学中，它广泛用于简化二进制数据的表示，因为每4位二进制数（比特）可以用单个十六进制数精确表示。"""
func (flow *CodecExecFlow) HexEncode() error {
	flow.Text = []byte(codec.EncodeToHex(flow.Text))
	return nil
}

// Tag = "解码"
// CodecName = "十六进制解码"
// Desc = """十六进制编码是一种数字表示法，使用0到9和A到F共16个字符来表示数值。在计算机科学中，它广泛用于简化二进制数据的表示，因为每4位二进制数（比特）可以用单个十六进制数精确表示。"""
func (flow *CodecExecFlow) HexDecode() error {
	res, err := codec.DecodeHex(string(flow.Text))
	if err != nil {
		return err
	}
	flow.Text = res
	return nil
}

// Tag = "编码"
// CodecName = "Unicode 编码"
// Desc = """Unicode 编解码是将世界各种文字符号映射到唯一码点，并通过编码方案（如UTF-8、UTF-16）转为字节序列的过程，以支持全球文本的统一表示和处理。"""
func (flow *CodecExecFlow) UnicodeEncode() error {
	flow.Text = []byte(codec.JsonUnicodeEncode(string(flow.Text)))
	return nil
}

// Tag = "解码"
// CodecName = "Unicode 中文解码"
// Desc = """Unicode 编解码是将世界各种文字符号映射到唯一码点，并通过编码方案（如UTF-8、UTF-16）转为字节序列的过程，以支持全球文本的统一表示和处理。"""
func (flow *CodecExecFlow) UnicodeDecode() error {
	flow.Text = []byte(codec.JsonUnicodeDecode(string(flow.Text)))
	return nil
}

// Tag = "Hash"
// CodecName = "MD5"
// Desc = """MD5是一种广泛使用的加密哈希函数，它接受任意长度的输入并输出固定长度（128位）的哈希值。常用于验证数据完整性，但不适用于安全加密，因为存在碰撞漏洞。"""
func (flow *CodecExecFlow) MD5() error {
	flow.Text = []byte(codec.Md5(flow.Text))
	return nil
}

// Tag = "Hash"
// CodecName = "SM3"
// Desc = """SM3是一种密码哈希函数，由中国国家密码管理局发布，输出长度为256位。它用于确保数据的完整性和一致性，与MD5和SHA-1相比，SM3设计更安全，主要应用于中国的商用密码系统中。"""
func (flow *CodecExecFlow) SM3() error {
	flow.Text = codec.SM3(flow.Text)
	return nil
}

// Tag = "Hash"
// CodecName = "SHA-1"
// Desc = """SHA-1（安全哈希算法1）是一种加密哈希函数，输出160位哈希值，用于确保数据完整性。虽然曾广泛应用于安全领域，但由于潜在的安全漏洞，现在不再推荐用于敏感数据保护。"""
func (flow *CodecExecFlow) SHA1() error {
	flow.Text = []byte(codec.Sha1(flow.Text))
	return nil
}

// Tag = "Hash"
// CodecName = "SHA-2"
// Desc = """SHA-2是安全哈希算法家族的一部分，包括多个版本（如SHA-256和SHA-512），输出哈希值长度不同，用于数据完整性验证和数字签名，相较于SHA-1提供更强的安全性。"""
// Params = [
// { Name = "size", Type = "select",DefaultValue = "SHA-512", Options = ["SHA-224", "SHA-256","SHA-384","SHA-512"], Required = true ,Label = "哈希版本"}
// ]
func (flow *CodecExecFlow) SHA2(size string) error {
	switch size {
	case "SHA-224":
		flow.Text = []byte(codec.Sha224(flow.Text))
	case "SHA-256":
		flow.Text = []byte(codec.Sha256(flow.Text))
	case "SHA-384":
		flow.Text = []byte(codec.Sha384(flow.Text))
	case "SHA-512":
		fallthrough
	default:
		flow.Text = []byte(codec.Sha512(flow.Text))
	}
	return nil
}

// Tag = "MAC"
// CodecName = "Hmac"
// Desc = """HMAC（Hash-based Message Authentication Code）是一种密钥相关的哈希运算消息认证码，主要用于服务器对访问者进行鉴权认证流程中。"""
// Params = [
// { Name = "key", Type = "inputSelect", Required = true,Label = "Key", Connector ={ Name = "keyType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "key格式"} },
// { Name = "hashMethod", Type = "select",DefaultValue = "SHA-512", Options = ["SHA-1", "SHA-256","SHA-512","MD5","SM3"], Required = true ,Label = "哈希方法"},
// { Name = "output", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "输出格式"}
// ]
func (flow *CodecExecFlow) Hmac(key string, keyType string, hashMethod string, output outputType) error {
	keyByte := decodeData([]byte(key), keyType)
	var res []byte
	switch hashMethod {
	case "MD5":
		res = codec.HmacMD5(keyByte, flow.Text)
	case "SHA-1":
		res = codec.HmacSha1(keyByte, flow.Text)
	case "SHA-256":
		res = codec.HmacSha256(keyByte, flow.Text)
	case "SHA-512":
		res = codec.HmacSha512(keyByte, flow.Text)
	case "SM3":
		res = codec.HmacSM3(keyByte, flow.Text)
	default:
		return utils.Error("Hmac: unknown hash method")
	}
	flow.Text = encodeData(res, output)
	return nil
}

// Tag = "MAC"
// CodecName = "CBC-MAC"
// Desc = """CBC-MAC（Cipher Block Chaining Message Authentication Code）是一种基于块密码的消息认证码（MAC）构造技术。它通过在密码块链（CBC）模式下加密消息来创建一个块链，其中每个块都依赖于前一个块的正确加密。"""
// Params = [
// { Name = "key", Type = "inputSelect", Required = true,Label = "Key", Connector ={ Name = "keyType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "key格式"} },
// { Name = "alg", Type = "select",DefaultValue = "AES", Options = ["SM4", "DES","AES"], Required = true ,Label = "加密算法"},
// { Name = "output", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "输出格式"},
// { Name = "paddingType", Type = "select", DefaultValue = "pkcs", Options = ["pkcs", "zeroPadding"], Required = true,Label = "填充方式"}
// ]
func (flow *CodecExecFlow) CbcMac(alg, key, keyType string, output outputType, paddingType string) error {
	keyByte := decodeData([]byte(key), keyType)
	var err error
	var c cipher.Block

	switch alg {
	case "SM4":
		c, err = sm4.NewCipher(keyByte)
		if err != nil {
			return err
		}
	case "DES":
		c, err = des.NewCipher(keyByte)
		if err != nil {
			return err
		}
	case "AES":
		c, err = aes.NewCipher(keyByte)
		if err != nil {
			return err
		}
	default:
		return utils.Error("CbcMac: unknown alg method")
	}
	data, err := padding(paddingType, flow.Text, c.BlockSize())
	if err != nil {
		return err
	}
	res, err := codec.CBCEncode(c, make([]byte, c.BlockSize()), data)
	if err != nil {
		return err
	}
	res = res[len(res)-c.BlockSize():]
	flow.Text = encodeData(res, output)
	return nil
}

// Tag = "MAC"
// CodecName = "CMAC"
// Desc = """CMAC（Cipher-based Message Authentication Code）是一种基于密码的消息认证码（MAC）算法，它使用对称加密算法来生成消息的认证码。CMAC的主要目的是确保消息的完整性和身份验证。它通过将消息与密钥一起处理，生成一个固定长度的认证码，该认证码可以被发送者和接收者用来验证消息的完整性和来源。"""
// Params = [
// { Name = "key", Type = "inputSelect", Required = true,Label = "Key", Connector ={ Name = "keyType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "key格式"} },
// { Name = "alg", Type = "select",DefaultValue = "AES", Options = ["SM4", "DES","AES","3DES"], Required = true ,Label = "加密算法"},
// { Name = "output", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "输出格式"}
// ]
func (flow *CodecExecFlow) Cmac(alg string, key string, keyType string, output outputType) error {
	keyByte := decodeData([]byte(key), keyType)
	cmacByte, err := codec.Cmac(alg, keyByte, flow.Text)
	if err != nil {
		return err
	}
	flow.Text = encodeData(cmacByte, output)
	return nil
}

// Tag = "数据美化"
// CodecName = "Json处理"
// Desc = """JSON（JavaScript Object Notation）是一种轻量级数据交换格式，易于人阅读和编写，同时也易于机器解析和生成。它基于JavaScript语言标准，但独立于语言，被广泛应用于网络应用程序中数据的传输。"""
// Params = [
// { Name = "mode", Type = "select",DefaultValue = "两格缩进", Options = ["四格缩进", "两格缩进","压缩"], Required = true ,Label = "处理方式"}
// ]
func (flow *CodecExecFlow) JsonFormat(mode string) error {
	var dst interface{}
	err := json.Unmarshal(flow.Text, &dst)
	if err != nil {
		return err
	}
	var res []byte
	switch mode {
	case "两格缩进":
		res, err = json.MarshalIndent(dst, "", "  ")
	case "压缩":
		res, err = json.Marshal(dst)
	case "四格缩进":
		fallthrough
	default:
		res, err = json.MarshalIndent(dst, "", "    ")
	}
	if err != nil {
		return err
	}
	flow.Text = res
	return nil
}

// Tag = "其他"
// CodecName = "生成数据包"
// Desc = """生成HTTP数据包，支持使用cURL和URL"""
// Params = [
// { Name = "mode", Type = "select",DefaultValue = "URL", Options = ["cURL", "URL"], Required = true ,Label = "输入格式"}
// ]
func (flow *CodecExecFlow) MakePacket(mode string) error {
	var res []byte
	var err error
	switch mode {
	case "cURL":
		res, err = lowhttp.CurlToHTTPRequest(string(flow.Text))
	case "URL":
		res, err = lowhttp.UrlToHTTPRequest(strings.TrimSpace(string(flow.Text)))
	default:
		return utils.Error("MakeHTTPPacket: unknown mode")
	}
	if err != nil {
		return err
	}
	flow.Text = res
	return nil
}

// Tag = "其他"
// CodecName = "数据包生成cURL命令"
// Desc = """通过数据包生成cURL命令，以导出数据包"""
// Params = [
// { Name = "https", Type = "checkbox", Required = true , Label = "https"}
// ]
func (flow *CodecExecFlow) Packet2cURL(https bool) error {
	cmd, err := lowhttp.GetCurlCommand(https, flow.Text)
	if err != nil {
		return utils.Errorf("codec[%v] failed: %s", `packet-to-curl`, err)
	}
	flow.Text = []byte(cmd.String())
	return nil
}

// Tag = "其他"
// CodecName = "JWT解析"
// Desc = """JWT（JSON Web Token）是一种开放标准（RFC 7519），用于在网络应用间安全地传输声明信息，通常用于身份验证和信息交换。"""
func (flow *CodecExecFlow) JwtParse() error {
	token, key, err := authhack.JwtParse(string(flow.Text))
	if err != nil || token == nil {
		return utils.Errorf("codec JWT解析 failed: %s", err)
	}
	flow.Text, err = json.MarshalIndent(map[string]interface{}{
		"raw":                       token.Raw,
		"alg":                       token.Method.Alg(),
		"is_valid":                  token.Valid,
		"brute_secret_key_finished": token.Valid,
		"header":                    token.Header,
		"claims":                    token.Claims,
		"secret_key":                utils.EscapeInvalidUTF8Byte(key),
	}, "", "    ")
	return nil
}

// Tag = "其他"
// CodecName = "JWT签名"
// Desc = """JWT（JSON Web Token）是一种开放标准（RFC 7519），用于在网络应用间安全地传输声明信息，通常用于身份验证和信息交换。"""
// Params = [
// { Name = "algorithm", Type = "select",DefaultValue = "HS256",Options = ["ES384","ES256","ES512","HS256","HS384","HS512","PS256","PS384","PS512","RS256","RS384","RS512","None"], Required = true , Label = "签名算法"},
// { Name = "key", Type = "input", Required = true , Label = "JWT密钥"},
// { Name = "isBase64", Type = "checkbox", Required = true , Label = "base64编码"},
// ]
func (flow *CodecExecFlow) JwtSign(algorithm string, key []byte, isBase64 bool) error {
	if !gjson.Valid(string(flow.Text)) {
		return utils.Error("codec JWT签名失败: json格式错误")
	}
	data := make(map[string]interface{})
	var err error
	gjson.Parse(string(flow.Text)).ForEach(func(key, value gjson.Result) bool {
		data[key.String()] = value.Value()
		return true
	})
	if isBase64 {
		key, err = codec.DecodeBase64(string(key))
		if err != nil {
			return utils.Wrapf(err, "codec JWT签名失败")
		}
	}
	jwtSign, err := authhack.JwtGenerate(algorithm, data, "", key)
	if err != nil {
		return utils.Wrapf(err, "codec JWT签名失败")
	}
	flow.Text = []byte(jwtSign)
	return nil
}

// Tag = "其他"
// CodecName = "fuzztag渲染"
// Desc = """渲染fuzztag"""
func (flow *CodecExecFlow) Fuzz() error {
	res, err := mutate.FuzzTagExec(flow.Text, mutate.Fuzz_WithEnableDangerousTag())
	if err != nil {
		return err
	}
	flow.Text = []byte(strings.Join(res, "\n"))
	return nil
}

// Tag = "其他"
// CodecName = "Replace"
// Desc = """替换字符串处理本文"""
// Params = [
// { Name = "find", Type = "input", Required = true , Label = "Find"},
// { Name = "replace", Type = "input", Required = false , Label = "Replace"},
// { Name = "findType", Type = "select",DefaultValue = "regexp",Options = ["regexp","raw"], Required = true , Label = "查找方式"},
// { Name = "Global", Type = "checkbox", Required = true , Label = "全部匹配"},
// { Name = "IgnoreCase", Type = "checkbox", Required = true , Label = "忽略大小写"},
// { Name = "Multiline", Type = "checkbox", Required = true , Label = "多行匹配"},
// ]
func (flow *CodecExecFlow) Replace(find string, replace string, findType string, Global, Multiline, IgnoreCase bool) error {
	count := 1
	if Global {
		count = -1
	}

	if findType == "raw" {
		find = regexp.QuoteMeta(find)
	}

	regFlag := regexp2.None
	if Multiline {
		regFlag = regFlag | regexp2.Multiline
	}
	if IgnoreCase {
		regFlag = regFlag | regexp2.IgnoreCase
	}

	reg, err := regexp2.Compile(find, regFlag)
	if err != nil {
		return err
	}

	text, err := reg.Replace(string(flow.Text), replace, -1, count)
	if err != nil {
		return err
	}

	flow.Text = []byte(text)
	return nil
}

// Tag = "其他"
// CodecName = "Find"
// Desc = """替换字符串处理本文"""
// Params = [
// { Name = "find", Type = "input", Required = true , Label = "Find"},
// { Name = "findType", Type = "select",DefaultValue = "regexp",Options = ["regexp","raw"], Required = true , Label = "查找方式"},
// { Name = "Global", Type = "checkbox", Required = true , Label = "全部匹配"},
// { Name = "IgnoreCase", Type = "checkbox", Required = true , Label = "忽略大小写"},
// { Name = "Multiline", Type = "checkbox", Required = true , Label = "多行匹配"},
// ]
func (flow *CodecExecFlow) Find(find string, findType string, Global, Multiline, IgnoreCase bool) error {
	if findType == "raw" {
		find = regexp.QuoteMeta(find)
	}

	regFlag := regexp2.None
	if Multiline {
		regFlag = regFlag | regexp2.Multiline
	}
	if IgnoreCase {
		regFlag = regFlag | regexp2.IgnoreCase
	}

	reg, err := regexp2.Compile(find, regFlag)
	if err != nil {
		return err
	}

	match, err := reg.FindStringMatch(string(flow.Text))
	if err != nil || match == nil { // match fail return []byte("")
		flow.Text = []byte("")
		return nil
	}
	text := match.String()
	if Global {
		for {
			match, err = reg.FindNextMatch(match)
			if err != nil || match == nil {
				break
			}
			text = strings.Join([]string{text, match.String()}, "\n")
		}
	}

	flow.Text = []byte(text)
	return nil
}

// Tag = "Yak脚本"
// CodecName = "本地Codec插件"
// Desc = """本地Codec插件"""
// Params = [
// { Name = "pluginName", Type = "search", Required = true , Label = "插件名"},
// ]
func (flow *CodecExecFlow) CodecPlugin(pluginName string) error {
	script, err := yakit.GetYakScriptByName(consts.GetGormProfileDatabase(), pluginName)
	if err != nil {
		return err
	}
	engine, err := yak.NewScriptEngine(1000).ExecuteEx(script.Content, map[string]interface{}{
		"YAK_FILENAME": pluginName,
	})
	if err != nil {
		return utils.Errorf("execute file %s code failed: %s", pluginName, err.Error())
	}
	pluginRes, err := engine.CallYakFunction(context.Background(), "handle", []interface{}{string(flow.Text)})
	if err != nil {
		return utils.Errorf("import %v' s handle failed: %s", pluginName, err)
	}
	flow.Text = utils.InterfaceToBytes(pluginRes)
	return nil
}

// Tag = "Yak脚本"
// CodecName = "临时Codec插件"
// Desc = """自定义临时Codec插件"""
// Params = [
// { Name = "pluginContent", Type = "monaco", Required = true , Label = "插件内容"},
// ]
func (flow *CodecExecFlow) CustomCodecPlugin(pluginContent string) error {
	engine, err := yak.NewScriptEngine(1000).ExecuteEx(pluginContent, map[string]interface{}{
		"YAK_FILENAME": "temp-codec",
	})
	if err != nil {
		return utils.Errorf("execute file %s code failed: %s", "temp-codec", err.Error())
	}
	pluginRes, err := engine.CallYakFunction(context.Background(), "handle", []interface{}{string(flow.Text)})
	if err != nil {
		return utils.Errorf("import %v' s handle failed: %s", "temp-codec", err)
	}
	flow.Text = utils.InterfaceToBytes(pluginRes)
	return nil
}
