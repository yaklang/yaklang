package codegrpc

import (
	"bytes"
	_ "embed"
	"encoding/gob"
	"encoding/json"
	"github.com/BurntSushi/toml"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/authhack"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/common/yserx"
	"strings"
)

//go:embed codec.gob.gzip
var codecDoc []byte

var CodecLibs *yakdoc.ScriptLib
var CodecLibsDoc []*ypb.CodecMethod // 记录函数的数据，参数类型等，用于前端生成样式

type outputType = string

var (
	OUTPUT_RAW = "raw"
	OUTPUT_HEX = "hex"
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

	iv, err := codec.DecodeHex(i)
	if err != nil {
		return nil, nil, err
	}
	return key, iv, nil
}

func convertOutput(text []byte, output outputType) []byte {
	switch output {
	case OUTPUT_RAW:
		return text
	case OUTPUT_HEX:
		return utils.UnsafeStringToBytes(codec.EncodeToHex(text))
	default:
		return text
	}
}

// Tag = "对称加密"
// CodecName = "AES对称加密"
// Desc ="""高级加密标准（AES）是美国联邦信息处理标准（FIPS）。它是在一个历时5年的过程中，从15个竞争设计中选出的。
// Key：根据密钥的大小，将使用以下算法：
// 16字节 = AES-128
// 24字节 = AES-192
// 32字节 = AES-256
// 你可以使用其中一个KDF操作生成基于密码的密钥。"""
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{32}|[a-fA-F0-9]{48}|[a-fA-F0-9]{64}$",Label = "Key"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{32}$",Label = "IV"},
// { Name = "mode", Type = "select", Options = ["CBC", "ECB", "GCM"], Required = true, Label = "Mode"},
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true ,Label = "输出格式"}
// ]
func (flow *CodecExecFlow) AESEncrypt(hexKey string, hexIV string, mode string, output outputType) error {
	var data []byte
	var err error
	key, iv, err := decodeHexKeyAndIV(hexKey, hexIV)
	if err != nil {
		return err
	}
	switch mode {
	case "CBC":
		data, err = codec.AESCBCEncrypt(key, flow.Text, iv)
	case "ECB":
		data, err = codec.AESECBEncrypt(key, flow.Text, iv)
	case "GCM":
		data, err = codec.AESGCMEncrypt(key, flow.Text, iv)
	default:
		return utils.Error("AESEncryptEx: unknown mode")
	}
	if err == nil {
		flow.Text = convertOutput(data, output)
	}
	return err
}

// Tag = "对称解密"
// CodecName = "AES对称解密"
// Desc = """高级加密标准（AES）是美国联邦信息处理标准（FIPS）。它是在一个历时5年的过程中，从15个竞争设计中选出的。
// Key：根据密钥的大小，将使用以下算法：
// 16字节 = AES-128
// 24字节 = AES-192
// 32字节 = AES-256"""
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{32}|[a-fA-F0-9]{48}|[a-fA-F0-9]{64}$",Label = "Key"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{32}$",Label = "IV"},
// { Name = "mode", Type = "select", Options = ["CBC", "ECB", "GCM"], Required = true, Label = "Mode"},
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true,Label = "输出格式"}
// ]
func (flow *CodecExecFlow) AESDecrypt(hexKey string, hexIV string, mode string, output outputType) error {
	var data []byte
	var err error
	key, iv, err := decodeHexKeyAndIV(hexKey, hexIV)
	if err != nil {
		return err
	}
	switch mode {
	case "CBC":
		data, err = codec.AESCBCEncrypt(key, flow.Text, iv)
	case "ECB":
		data, err = codec.AESECBEncrypt(key, flow.Text, iv)
	case "GCM":
		data, err = codec.AESGCMEncrypt(key, flow.Text, iv)
	default:
		return utils.Error("AESEncryptEx: unknown mode")
	}
	if err == nil {
		flow.Text = convertOutput(data, output)
	}
	return err
}

// Tag = "对称加密"
// CodecName = "SM4对称加密"
// Desc = """SM4是一个128位的块密码，目前被确定为中国的国家标准（GB/T 32907-2016）。支持多种块密码模式。"""
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{32}$",Label = "Key"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{32}$",Label = "IV"},
// { Name = "mode", Type = "select", Options = ["CBC", "ECB", "GCM", "CFB", "OFB"], Required = true, Label = "Mode"},
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true,Label = "输出格式"}
// ]
func (flow *CodecExecFlow) SM4Encrypt(hexKey string, hexIV string, mode string, output outputType) error {
	var data []byte
	var err error
	key, iv, err := decodeHexKeyAndIV(hexKey, hexIV)
	if err != nil {
		return err
	}
	switch mode {
	case "CBC":
		data, err = codec.SM4CBCEnc(key, flow.Text, iv)
	case "ECB":
		data, err = codec.SM4ECBEnc(key, flow.Text, iv)
	case "GCM":
		data, err = codec.SM4GCMEnc(key, flow.Text, iv)
	case "CFB":
		data, err = codec.SM4CFBEnc(key, flow.Text, iv)
	case "OFB":
		data, err = codec.SM4OFBEnc(key, flow.Text, iv)
	default:
		return utils.Error("AESEncryptEx: unknown mode")
	}
	if err == nil {
		flow.Text = convertOutput(data, output)
	}
	return err
}

// Tag = "对称解密"
// CodecName = "SM4对称解密"
// Desc = """SM4是一个128位的块密码，目前被确定为中国的国家标准（GB/T 32907-2016）。支持多种块密码模式。"""
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{32}$",Label = "Key"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{32}$",Label = "IV"},
// { Name = "mode", Type = "select", Options = ["CBC", "ECB", "GCM", "CFB", "OFB"], Required = true, Label = "Mode"},
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true ,Label = "输出格式"}
// ]
func (flow *CodecExecFlow) SM4Decrypt(hexKey string, hexIV string, mode string, output outputType) error {
	var data []byte
	var err error
	key, iv, err := decodeHexKeyAndIV(hexKey, hexIV)
	if err != nil {
		return err
	}
	switch mode {
	case "CBC":
		data, err = codec.SM4CBCDec(key, flow.Text, iv)
	case "ECB":
		data, err = codec.SM4ECBDec(key, flow.Text, iv)
	case "GCM":
		data, err = codec.SM4GCMDec(key, flow.Text, iv)
	case "CFB":
		data, err = codec.SM4CFBDec(key, flow.Text, iv)
	case "OFB":
		data, err = codec.SM4OFBDec(key, flow.Text, iv)
	default:
		return utils.Error("AESEncryptEx: unknown mode")
	}
	if err == nil {
		flow.Text = convertOutput(data, output)
	}
	return err
}

// Tag = "对称加密"
// CodecName = "DES对称加密"
// Desc = """DES（Data Encryption Standard）是一种对称密钥加密算法，使用固定有效长度为56位的密钥对数据进行64位的分组加密。尽管曾广泛使用，但由于密钥太短，现已被认为不够安全。"""
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{16}$",	Label = "Key"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{16}$",Label = "IV"},
// { Name = "mode", Type = "select", Options = ["CBC", "ECB"], Required = true , Label = "Mode"},
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true,Label = "输出格式"}
// ]
func (flow *CodecExecFlow) DESEncrypt(hexKey string, hexIV string, mode string, output outputType) error {
	var data []byte
	var err error
	key, iv, err := decodeHexKeyAndIV(hexKey, hexIV)
	if err != nil {
		return err
	}
	switch mode {
	case "CBC":
		data, err = codec.DESCBCEnc(key, flow.Text, iv)
	case "ECB":
		data, err = codec.DESECBEnc(key, flow.Text)
	default:
		return utils.Error("AESEncryptEx: unknown mode")
	}
	if err == nil {
		flow.Text = convertOutput(data, output)
	}
	return err

}

// Tag = "对称解密"
// CodecName = "DES对称解密"
// Desc = """DES（Data Encryption Standard）是一种对称密钥加密算法，使用固定有效长度为56位的密钥对数据进行64位的分组加密。尽管曾广泛使用，但由于密钥太短，现已被认为不够安全。"""
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{16}$",	Label = "Key"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{16}$",Label = "IV"},
// { Name = "mode", Type = "select", Options = ["CBC", "ECB"], Required = true , Label = "Mode"},
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true ,Label = "输出格式"}
// ]
func (flow *CodecExecFlow) DESDecrypt(hexKey string, hexIV string, mode string, output outputType) error {
	var data []byte
	var err error
	key, iv, err := decodeHexKeyAndIV(hexKey, hexIV)
	if err != nil {
		return err
	}
	switch mode {
	case "CBC":
		data, err = codec.DESCBCDec(key, flow.Text, iv)
	case "ECB":
		data, err = codec.DESECBDec(key, flow.Text)
	default:
		return utils.Error("AESEncryptEx: unknown mode")
	}
	if err == nil {
		flow.Text = convertOutput(data, output)
	}
	return err
}

// Tag = "对称加密"
// CodecName = "TripleDES对称加密"
// Desc = """TripleDES（3DES）是DES的改进版，通过连续三次应用DES算法（可以使用三个不同的密钥）来增加加密的强度，提供了更高的安全性。"""
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{32}|[a-fA-F0-9]{48}$",Label = "Key"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{16}$",Label = "IV"},
// { Name = "mode", Type = "select", Options = ["CBC", "ECB"], Required = true, Label = "Mode"},
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true ,Label = "输出格式"}
// ]
func (flow *CodecExecFlow) TripleDESEncrypt(hexKey string, hexIV string, mode string, output outputType) error {
	var data []byte
	var err error
	key, iv, err := decodeHexKeyAndIV(hexKey, hexIV)
	if err != nil {
		return err
	}
	switch mode {
	case "CBC":
		data, err = codec.TripleDES_CBCEnc(key, flow.Text, iv)
	case "ECB":
		data, err = codec.TripleDES_ECBEnc(key, flow.Text)
	default:
		return utils.Error("AESEncryptEx: unknown mode")
	}
	if err == nil {
		flow.Text = convertOutput(data, output)
	}
	return err
}

// Tag = "对称解密"
// CodecName = "TripleDES对称解密"
// Desc = """TripleDES（3DES）是DES的改进版，通过连续三次应用DES算法（可以使用三个不同的密钥）来增加加密的强度，提供了更高的安全性。"""
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{32}|[a-fA-F0-9]{48}$",Label = "Key"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{16}$",Label = "IV" },
// { Name = "mode", Type = "select", Options = ["CBC", "ECB"], Required = true , Label = "Mode"},
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true ,Label = "输出格式"}
// ]
func (flow *CodecExecFlow) TripleDESDecrypt(hexKey string, hexIV string, mode string, output outputType) error {
	var data []byte
	var err error
	key, iv, err := decodeHexKeyAndIV(hexKey, hexIV)
	if err != nil {
		return err
	}
	switch mode {
	case "CBC":
		data, err = codec.TripleDES_CBCDec(key, flow.Text, iv)
	case "ECB":
		data, err = codec.TripleDES_ECBDec(key, flow.Text)
	default:
		return utils.Error("AESEncryptEx: unknown mode")
	}
	if err == nil {
		flow.Text = convertOutput(data, output)
	}
	return err
}

// Tag = "Java"
// CodecName = "反序列化"
// Desc = """Java反序列化是一种将字节流转换为Java对象的机制，以便可以在网络上传输或将其保存到文件中。
// Yak中提供了两种反序列化方式： dumper 和 object-stream ，其中object-stream是Yak独有的一种伪代码表达形式，更直观易读"""
// Params = [
// { Name = "input", Type = "select", Options = ["raw", "hex", "base64"], Required = true , Label = "输入格式"},
// { Name = "output", Type = "select", Options = ["dumper", "object-stream"], Required = true , Label = "输出格式"}
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
// { Name = "output", Type = "select", Options = ["raw", "hex", "base64"], Required = true , Label = "输出格式"}
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
// { Name = "Alphabet", Type = "select", Options = ["standard", "urlsafe"], Required = true,Label = "Alphabet"}
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
// { Name = "Alphabet", Type = "select", Options = ["A-Za-z0-9+/=", "A-Za-z0-9-_"], Required = true,Lable = "Alphabet" }
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
// { Name = "entityRef", Type = "select", Options = ["dec", "hex", "named"], Required = true ,Label = "实体编码格式"}},
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
// { Name = "size", Type = "select", Options = ["SHA-224", "SHA-256","SHA-384","SHA-512"], Required = true ,Label = "哈希版本"}
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

// Tag = "数据美化"
// CodecName = "Json处理"
// Desc = """JSON（JavaScript Object Notation）是一种轻量级数据交换格式，易于人阅读和编写，同时也易于机器解析和生成。它基于JavaScript语言标准，但独立于语言，被广泛应用于网络应用程序中数据的传输。"""
// Params = [
// { Name = "mode", Type = "select", Options = ["四格缩进", "两格缩进","压缩"], Required = true ,Label = "处理方式"}
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
// { Name = "mode", Type = "select", Options = ["cURL", "URL"], Required = true ,Label = "输入格式"}
// ]
func (flow *CodecExecFlow) MakePacket(mode string) error {
	var res []byte
	var err error
	switch mode {
	case "cURL":
		res, err = lowhttp.CurlToHTTPRequest(string(flow.Text))
	case "URL":
		res, err = lowhttp.UrlToHTTPRequest(string(flow.Text))
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
	if err != nil {
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
// { Name = "algorithm", Type = "select",Options = ["ES384","ES256","ES512","HS256","HS384","HS512","PS256","PS384","PS512","RS256","RS384","RS512","None"], Required = true , Label = "签名算法"},
// { Name = "key", Type = "input", Required = true , Label = "JWT密钥"},
// { Name = "isBase64", Type = "checkbox", Required = true , Label = "base64编码"},
// ]
func (flow *CodecExecFlow) JwtSign(algorithm string, key []byte, isBase64 bool) error {
	if !gjson.Valid(string(flow.Text)) {
		return utils.Error("codec JWT签名失败: json格式错误")
	}
	var data map[string]interface{}
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
	res, err := mutate.FuzzTagExec(flow.Text, mutate.Fuzz_WithEnableFiletag())
	if err != nil {
		return err
	}
	flow.Text = []byte(strings.Join(res, "\n"))
	return nil
}
