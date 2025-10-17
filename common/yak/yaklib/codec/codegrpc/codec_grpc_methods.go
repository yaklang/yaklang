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
	"fmt"
	"hash"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/jsonpath"

	"github.com/samber/lo"
	xml_tools "github.com/yaklang/yaklang/common/utils/yakxml/xml-tools"

	"github.com/yaklang/yaklang/common/gmsm/sm4"
	charsetLib "golang.org/x/net/html/charset"

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
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
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
	CodecLibsDoc = make(map[string]*ypb.CodecMethod) // 记录函数的数据，参数类型等，用于前端生成样式
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
		CodecLibsDoc[funcName] = &CodecMethod
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

			var workFlow []*ypb.CodecWork
			err = json.Unmarshal(codecFlow.WorkFlow, &workFlow)
			if err != nil {
				log.Errorf("unmarshal codec flow failed: %s", err)
			}

			getWorkStatus := func(index int) string {
				return utils.InterfaceToString(jsonpath.Find(codecFlow.WorkFlowUI, fmt.Sprintf("$.rightItems[%d].status", index)))
			}

			filterWorkFlow := func(allWork []*ypb.CodecWork) (shouldRunWorkFlow []*ypb.CodecWork) {
				for i, work := range allWork {
					switch getWorkStatus(i) {
					case "suspend":
						return
					case "shield":
						continue
					default:
						shouldRunWorkFlow = append(shouldRunWorkFlow, work)
					}
				}
				return
			}

			res, err := CodecFlowExec(&ypb.CodecRequestFlow{
				Text:       input,
				Auto:       false,
				WorkFlow:   filterWorkFlow(workFlow),
				InputBytes: nil,
			})
			if err != nil {
				return []string{}
			}
			return []string{res.GetResult()}
		},
		Description:         "调用codec模块保存的codec flow，例如 {{codecflow(flowname|test)}}，其中flowname是保存的codecflow名，input是需要编码的输入",
		TagNameVerbose:      "调用codec模块保存的codec flow",
		ArgumentDescription: "{{string_split(name:codecflow名)}}{{string(abc:输入)}}",
	})
}

var (
	getCodecLibsDocMethodsMu     sync.Mutex
	codecLibsDocMethods          []*ypb.CodecMethod
	getCodecLibsDocMethodNamesMu sync.Mutex
	codecLibsDocMethodNames      []string
)

func GetCodecLibsDocMethods() []*ypb.CodecMethod {
	if codecLibsDocMethods == nil {
		getCodecLibsDocMethodsMu.Lock()
		defer getCodecLibsDocMethodsMu.Unlock()

		codecLibsDocMethods = lo.Values(CodecLibsDoc)
	}

	return codecLibsDocMethods
}

func GetCodecLibsDocMethodNames() []string {
	if codecLibsDocMethodNames == nil {
		getCodecLibsDocMethodsMu.Lock()
		defer getCodecLibsDocMethodsMu.Unlock()

		codecLibsDocMethodNames = lo.Keys(CodecLibsDoc)
	}

	return codecLibsDocMethodNames
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

func getKDF(kdfMode string, hashHandle hash.Hash) codec.KeyDerivationFunc {
	if hashHandle == nil {
		return nil
	}
	switch kdfMode {
	case "PBKDF2":
		return codec.NewPBKDF2Generator(func() hash.Hash { return hashHandle }, codec.DefaultPBKDF2Iterations)
	case "Openssl":
		return codec.NewBytesToKeyGenerator(func() hash.Hash { return hashHandle }, codec.DefaultBytesToKeyIterations)
	}
	return nil
}

func getHash(hashFunc string) hash.Hash {
	switch hashFunc {
	case "MD5":
		return md5.New()
	case "SHA-1":
		return sha1.New()
	case "SHA-256":
		return sha256.New()
	case "SHA-384":
		return sha512.New384()
	case "SHA-512":
		return sha512.New()
	}
	return nil
}

// Tag = "加密"
// CodecName = "AES对称加密-KDF密钥生成"
// Desc ="""高级加密标准（AES）是美国联邦信息处理标准（FIPS）。它是在一个历时5年的过程中，从15个竞争设计中选出的。
// 你可以使用下列的其中一个KDF操作生成基于密码的密钥。"""
// Params = [
// { Name = "password", Type = "input", Required = true,Label = "密码" },
// { Name = "kdfMode", Type = "select", DefaultValue = "Openssl", Options = ["Openssl", "PBKDF2"], Required = true ,Label = "密钥派生算法-KDF"},
// { Name = "hashFunc", Type = "select",DefaultValue = "MD5", Options = ["MD5","SHA-1", "SHA-256","SHA-384","SHA-512"], Required = true ,Label = "哈希"},
// { Name = "noSalt", Type = "checkbox", Required = true , Label = "不加盐值"},
// { Name = "mode", Type = "select", DefaultValue = "CBC",Options = ["CBC", "ECB", "CTR"], Required = true, Label = "Mode"},
// { Name = "output", Type = "select", DefaultValue = "base64", Options = ["hex", "raw", "base64"], Required = true ,Label = "输出格式"},
// { Name = "paddingType", Type = "select", DefaultValue = "pkcs", Options = ["pkcs", "zeroPadding"], Required = true,Label = "填充方式"}
// ]
func (flow *CodecExecFlow) AESEncryptKDF(password string, kdfMode string, hashFunc string, noSalt bool, mode string, output outputType, paddingType string) error {
	inData, err := padding(paddingType, flow.Text, 16)
	if err != nil {
		return err
	}

	hashHandle := getHash(hashFunc)
	if hashHandle == nil {
		return utils.Error("unknown hash function")
	}

	kdf := getKDF(kdfMode, hashHandle)
	if kdf == nil {
		return utils.Error("unknown kdf mode")
	}

	salt := []byte("")
	if !noSalt {
		salt = codec.RandBytes(8)
	}

	cipherText, err := codec.AESEncWithPassphrase([]byte(password), inData, salt, kdf, mode)
	if err != nil {
		return err
	}
	if !noSalt {
		prev := append([]byte("Salted__"), salt...)
		cipherText = append(prev, cipherText...)
	}
	flow.Text = encodeData(cipherText, output)
	return nil
}

// Tag = "解密"
// CodecName = "AES对称解密-KDF密钥生成"
// Desc = """高级加密标准（AES）是美国联邦信息处理标准（FIPS）。它是在一个历时5年的过程中，从15个竞争设计中选出的。
// 你可以使用下列的其中一个KDF操作生成基于密码的密钥。
// """
// Params = [
// { Name = "password", Type = "input", Required = true,Label = "密码" },
// { Name = "kdfMode", Type = "select", DefaultValue = "Openssl", Options = ["Openssl", "PBKDF2"], Required = true ,Label = "密钥派生算法-KDF"},
// { Name = "hashFunc", Type = "select",DefaultValue = "MD5", Options = ["MD5","SHA-1", "SHA-256","SHA-384","SHA-512"], Required = true ,Label = "哈希"},
// { Name = "mode", Type = "select", DefaultValue = "CBC",Options = ["CBC", "ECB", "CTR"], Required = true, Label = "Mode"},
// { Name = "input", Type = "select", DefaultValue = "base64", Options = ["hex", "raw", "base64"], Required = true,Label = "输入格式"},
// { Name = "paddingType", Type = "select", DefaultValue = "pkcs", Options = ["pkcs", "zeroPadding"], Required = true,Label = "填充方式"}
// ]
func (flow *CodecExecFlow) AESDecryptKDF(password string, kdfMode string, hashFunc string, mode string, input outputType, paddingType string) error {
	var err error
	inputText := decodeData(flow.Text, input)
	hashHandle := getHash(hashFunc)
	if hashHandle == nil {
		return utils.Error("unknown hash function")
	}
	kdf := getKDF(kdfMode, hashHandle)
	if kdf == nil {
		return utils.Error("unknown kdf mode")
	}

	salt := []byte("")
	if bytes.HasPrefix(inputText, []byte("Salted__")) {
		salt = inputText[8:16]
		inputText = inputText[16:]
	}
	cipherText, err := codec.AESDecWithPassphrase([]byte(password), inputText, salt, kdf, mode)
	if err != nil {
		return err
	}
	cipherText, err = unPadding(paddingType, cipherText)
	if err != nil {
		return err
	}
	flow.Text = cipherText
	return nil
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
// CodecName = "AES-GCM加密"
// Desc = """AES-GCM (Galois/Counter Mode) 是一种认证加密模式，它同时提供数据的机密性和完整性保护。GCM模式结合了CTR模式的加密和GMAC的认证，是现代密码学中广泛使用的安全加密方式。输出格式为: nonce + ciphertext + tag (当nonce为空时自动生成并包含在输出中)。"""
// Params = [
// { Name = "key", Type = "inputSelect", Required = true,Label = "Key", Connector ={ Name = "keyType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "key格式"} },
// { Name = "nonce", Type = "inputSelect", Required = false ,Label = "Nonce/IV", Connector ={ Name = "nonceType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "Nonce格式"} },
// { Name = "nonceSize", Type = "select", DefaultValue = "12", Options = ["12", "16"], Required = true, Label = "Nonce长度"},
// { Name = "output", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "输出格式"}
// ]
func (flow *CodecExecFlow) AESGCMEncrypt(key string, keyType string, nonce string, nonceType string, nonceSize string, output outputType) error {
	decodeKey := decodeData([]byte(key), keyType)
	decodeNonce := decodeData([]byte(nonce), nonceType)

	var nonceSizeInt int
	switch nonceSize {
	case "12":
		nonceSizeInt = 12
	case "16":
		nonceSizeInt = 16
	default:
		nonceSizeInt = 12
	}

	data, err := codec.AESGCMEncryptWithNonceSize(decodeKey, flow.Text, decodeNonce, nonceSizeInt)
	if err != nil {
		return err
	}

	flow.Text = encodeData(data, output)
	return nil
}

// Tag = "解密"
// CodecName = "AES-GCM解密"
// Desc = """AES-GCM (Galois/Counter Mode) 是一种认证加密模式，它同时提供数据的机密性和完整性保护。GCM模式结合了CTR模式的加密和GMAC的认证，是现代密码学中广泛使用的安全加密方式。输入格式应为: nonce + ciphertext + tag 或者单独的 ciphertext (如果nonce单独提供)。"""
// Params = [
// { Name = "key", Type = "inputSelect", Required = true,Label = "Key", Connector ={ Name = "keyType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "key格式"} },
// { Name = "nonce", Type = "inputSelect", Required = false ,Label = "Nonce/IV", Connector ={ Name = "nonceType", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true ,Label = "Nonce格式"} },
// { Name = "nonceSize", Type = "select", DefaultValue = "12", Options = ["12", "16"], Required = true, Label = "Nonce长度"},
// { Name = "input", Type = "select", DefaultValue = "hex", Options = ["hex", "raw", "base64"], Required = true,Label = "输入格式"}
// ]
func (flow *CodecExecFlow) AESGCMDecrypt(key string, keyType string, nonce string, nonceType string, nonceSize string, input outputType) error {
	decodeKey := decodeData([]byte(key), keyType)
	decodeNonce := decodeData([]byte(nonce), nonceType)
	inputText := decodeData(flow.Text, input)

	var nonceSizeInt int
	switch nonceSize {
	case "12":
		nonceSizeInt = 12
	case "16":
		nonceSizeInt = 16
	default:
		nonceSizeInt = 12
	}

	dec, err := codec.AESGCMDecryptWithNonceSize(decodeKey, inputText, decodeNonce, nonceSizeInt)
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
// CodecName = "SM2加密"
// Desc = """​​SM2​​ 是中国国家密码管理局（OSCCA）于 2010 年发布的一种基于​​椭圆曲线密码学（Elliptic Curve Cryptography, ECC）​​的公钥密码算法标准，属于​​国家商用密码体系（SM系列算法）​​中的重要组成部分。"""
// Params = [
// { Name = "pubKey", Type = "text", Required = true,Label = "公钥"},
// { Name = "encryptSchema", Type = "select",DefaultValue = "C1C2C3", Options = ["ASN1", "C1C2C3", "C1C3C2"], Required = true, Label = "编码格式"},
// ]
func (flow *CodecExecFlow) SM2Encrypt(pubKey string, encryptSchema string) error {
	var data []byte
	var err error

	switch encryptSchema { // choose alg
	case "ASN1":
		data, err = codec.SM2EncryptASN1([]byte(pubKey), []byte(flow.Text))
	case "C1C2C3":
		data, err = codec.SM2EncryptC1C2C3([]byte(pubKey), []byte(flow.Text))
	case "C1C3C2":
		data, err = codec.SM2EncryptC1C3C2([]byte(pubKey), []byte(flow.Text))
	default:
	}
	if err == nil {
		flow.Text = data
	}

	return err
}

// Tag = "解密"
// CodecName = "SM2解密"
// Desc = """​​SM2​​ 是中国国家密码管理局（OSCCA）于 2010 年发布的一种基于​​椭圆曲线密码学（Elliptic Curve Cryptography, ECC）​​的公钥密码算法标准，属于​​国家商用密码体系（SM系列算法）​​中的重要组成部分。"""
// Params = [
// { Name = "priKey", Type = "text", Required = true,Label = "私钥"},
// { Name = "decryptSchema", Type = "select",DefaultValue = "C1C2C3", Options = ["ASN1", "C1C2C3", "C1C3C2"], Required = true, Label = "编码格式"},
// ]
func (flow *CodecExecFlow) SM2Decrypt(priKey string, decryptSchema string) error {
	var data []byte
	var err error

	switch decryptSchema { // choose alg
	case "ASN1":
		data, err = codec.SM2DecryptASN1([]byte(priKey), []byte(flow.Text))
	case "C1C2C3":
		data, err = codec.SM2DecryptC1C2C3([]byte(priKey), []byte(flow.Text))
	case "C1C3C2":
		data, err = codec.SM2DecryptC1C3C2([]byte(priKey), []byte(flow.Text))
	default:
	}
	if err == nil {
		flow.Text = data
	}

	return err
}

// Tag = "加密"
// CodecName = "RSA加密"
// Desc = """RSA加密算法是一种非对称加密算法，在公开密钥加密和电子商业中被广泛使用。RSA是被研究得最广泛的公钥算法，从提出后经历了各种攻击的考验，逐渐为人们接受，普遍认为是目前最优秀的公钥方案之一。"""
// Params = [
// { Name = "pubKey", Type = "text", Required = true,Label = "公钥"},
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
		data, err = tlsutils.PkcsOAEPEncryptWithHash([]byte(pubKey), flow.Text, hashFunc)
	case "PKCS1v15":
		data, err = tlsutils.Pkcs1v15Encrypt([]byte(pubKey), flow.Text)
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
// { Name = "priKey", Type = "text", Required = true,Label = "私钥"},
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
		data, err = tlsutils.PkcsOAEPDecryptWithHash([]byte(priKey), flow.Text, hashFunc)
	case "PKCS1v15":
		data, err = tlsutils.Pkcs1v15Decrypt([]byte(priKey), flow.Text)
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

// Tag = "数据美化"
// CodecName = "XML美化"
// Desc = """可扩展标记语言（英语：Extensible Markup Language，简称：XML）是一种标记语言和用于存储、传输和重构松散数据的文件格式。它定义了一系列编码文档的规则以使其在人类可读的同时机器可读。"""
func (flow *CodecExecFlow) XMLFormat() error {
	flow.Text = []byte(xml_tools.XmlPrettify(flow.Text))
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
		res, err = lowhttp.CurlToRawHTTPRequest(string(flow.Text))
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
	if err == authhack.ErrKeyNotFound {
		err = nil
	}
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
	return err
}

// Tag = "其他"
// CodecName = "JWT签名"
// Desc = """JWT（JSON Web Token）是一种开放标准（RFC 7519），用于在网络应用间安全地传输声明信息，通常用于身份验证和信息交换。"""
// Params = [
// { Name = "algorithm", Type = "select",DefaultValue = "HS256",Options = ["ES384","ES256","ES512","HS256","HS384","HS512","PS256","PS384","PS512","RS256","RS384","RS512","None"], Required = true , Label = "签名算法"},
// { Name = "key", Type = "input", Required = false , Label = "JWT密钥"},
// { Name = "isBase64", Type = "checkbox", Required = true , Label = "base64编码"},
// ]
func (flow *CodecExecFlow) JwtSign(algorithm string, key []byte, isBase64 bool) error {
	if len(key) == 0 && algorithm != "None" {
		return utils.Error("codec JWT签名失败: 未提供密钥")
	}
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
// CodecName = "JWT解析签名"
// Desc = """JWT（JSON Web Token）是一种开放标准（RFC 7519），用于在网络应用间安全地传输声明信息，通常用于身份验证和信息交换。此处提供了JWT解析签名功能,将JWT解析中返回的json格式重新签名为JWT。"""
// Params = [
// ]
func (flow *CodecExecFlow) JwtReverseSign() error {
	if !gjson.Valid(string(flow.Text)) {
		return utils.Error("codec JWT解析签名失败: json格式错误")
	}
	res := gjson.ParseBytes(flow.Text)
	alg := res.Get("alg").String()
	claims := make(map[string]any)
	res.Get("claims").ForEach(func(key, value gjson.Result) bool {
		claims[key.String()] = value.Value()
		return true
	})
	headers := make(map[string]any)
	headerMap := res.Get("header").Map()
	for k, v := range headerMap {
		headers[k] = v.Value()
	}
	typ := "None"
	if iTyp, ok := headerMap["typ"]; ok {
		typ = iTyp.String()
	}
	key := res.Get("secret_key").String()

	jwtSign, err := authhack.JwtGenerateEx(alg, headers, claims, typ, []byte(key))
	if err != nil {
		return utils.Wrapf(err, "codec JWT签名失败")
	}
	flow.Text = []byte(jwtSign)
	return nil
}

// Tag = "其他"
// CodecName = "fuzztag渲染"
// Desc = """渲染fuzztag"""
// Params = [
// { Name = "timeout", Type = "input", Label = "超时时间"},
// { Name = "limit", Type = "input", Label = "生成个数"},
// ]
func (flow *CodecExecFlow) Fuzz(timeout, limit string) error {

	opts := []mutate.FuzzConfigOpt{
		mutate.Fuzz_WithEnableDangerousTag(),
	}
	if timeout != "" {
		t, err := strconv.ParseFloat(timeout, 64)
		if err != nil {
			return utils.Error("invalid timeout")
		}
		opts = append(opts, mutate.Fuzz_WithContext(utils.TimeoutContextSeconds(t)))
	}
	if limit != "" {
		l, err := strconv.Atoi(limit)
		if err != nil {
			return utils.Error("invalid limit")
		}
		opts = append(opts, mutate.Fuzz_WithResultLimit(l))
	}

	res, err := mutate.FuzzTagExec(flow.Text, opts...)
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

// Tag = "其他"
// CodecName = "转义字符串"
// Desc ="""转义一串字符串，转义后的字符串用双引号包裹。返回的字符串将转义控制字符和不可打印字符以及双引号。"""
// Params = [
// ]
func (flow *CodecExecFlow) StrQuote() error {
	flow.Text = []byte(strconv.Quote(string(flow.Text)))
	return nil
}

// Tag = "其他"
// CodecName = "解转义字符串"
// Desc ="""解转义一串字符串，输入需要用双引号包裹。返回的字符串将转义控制字符和不可打印字符以及双引号。"""
// Params = [
// ]
func (flow *CodecExecFlow) StrUnQuote() error {
	text, err := strconv.Unquote(string(flow.Text))
	if err != nil {
		return err
	}
	flow.Text = []byte(text)
	return nil
}

// Tag = "其他"
// CodecName = "GB18030ToUTF8"
// Desc ="""GB18030是一种中文编码标准，支持简体中文、繁体中文、日文和韩文等多种字符集，是GB2312和GBK的扩展。该函数将GB18030编码的文本转换为UTF-8编码。"""
// Params = [
// ]
func (flow *CodecExecFlow) GB18030ToUTF8() error {
	text, err := codec.GB18030ToUtf8(flow.Text)
	if err != nil {
		return err
	}
	flow.Text = text
	return nil
}

// Tag = "其他"
// CodecName = "字符集转换为UTF8字符集"
// Desc ="""尝试将文本从指定的字符集转换为UTF-8字符集。"""
// Params = [
// { Name = "charset", Type = "select",DefaultValue = "gb18030",Options = ["gb18030", "windows-1252", "iso-8859-1", "big5", "utf-16"], Required = true , Label = "字符集"},
// ]
func (flow *CodecExecFlow) CharsetToUTF8(charset string) error {
	enc, name := charsetLib.Lookup(charset)
	if enc == nil {
		return utils.Errorf("Can't find charset: %s", charset)
	}
	decoded, err := enc.NewDecoder().Bytes(flow.Text)
	if err != nil {
		return utils.Wrapf(err, "transform %s to utf8 error", name)
	}
	flow.Text = decoded
	return nil
}

// Tag = "其他"
// CodecName = "UTF8字符集转换为其他字符集"
// Desc ="""尝试将文本从UTF-8字符集转换为指定的字符集。"""
// Params = [
// { Name = "charset", Type = "select",DefaultValue = "gb18030",Options = ["gb18030", "windows-1252", "iso-8859-1", "big5", "utf-16"], Required = true , Label = "字符集"},
// ]
func (flow *CodecExecFlow) UTF8ToCharset(charset string) error {
	enc, name := charsetLib.Lookup(charset)
	if enc == nil {
		return utils.Errorf("Can't find charset: %s", charset)
	}
	encoded, err := enc.NewEncoder().Bytes(flow.Text)
	if err != nil {
		return utils.Wrapf(err, "transform utf8 to %s error", name)
	}
	flow.Text = encoded
	return nil
}

// Tag = "其他"
// CodecName = "HTTP数据包变形"
// Desc ="""将HTTP数据包变形，修改请求方法等"""
// Params = [
// { Name = "transform", Type = "select", DefaultValue = "GET",Options = ["GET", "POST", "HEAD", "Chunk 编码", "上传数据包", "上传数据包(仅POST参数)"], Required = true , Label = "转换方法"},
// ]
func (flow *CodecExecFlow) HTTPRequestMutate(transform string) error {
	rawRequest := flow.Text
	result := rawRequest
	method := ""
	chunkEncode, uploadEncode, onlyUsePostParams := false, false, false
	switch transform {
	case "GET", "POST", "HEAD":
		method = transform
	case "Chunk 编码":
		chunkEncode = true
	case "上传数据包":
		uploadEncode = true
	case "上传数据包(仅POST参数)":
		uploadEncode = true
		onlyUsePostParams = true
	}
	// get params
	totalParams := lowhttp.GetFullHTTPRequestQueryParams(rawRequest)
	contentType := lowhttp.GetHTTPPacketHeader(rawRequest, "Content-Type")
	transferEncoding := lowhttp.GetHTTPPacketHeader(rawRequest, "Transfer-Encoding")
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rawRequest)
	// chunked 转 Content-Length
	if !chunkEncode && utils.IContains(transferEncoding, "chunked") {
		result = lowhttp.ReplaceHTTPPacketBody(result, body, false)
		_, body = lowhttp.SplitHTTPHeadersAndBodyFromPacket(result)
	}
	// post params
	postParams, _, _ := lowhttp.GetParamsFromBody(contentType, body)
	if totalParams == nil {
		totalParams = make(map[string][]string)
	}
	if len(postParams.Items) > 0 {
		for _, param := range postParams.Items {
			totalParams[param.Key] = append(totalParams[param.Key], param.Values...)
		}
	}

	switch method {
	case "POST":
		result = poc.FixPacketByPocOptions(lowhttp.TrimLeftHTTPPacket(result),
			poc.WithReplaceHttpPacketMethod("POST"),
			poc.WithReplaceHttpPacketQueryParamRaw(""),
			poc.WithReplaceHttpPacketHeader("Content-Type", "application/x-www-form-urlencoded"),
			poc.WithDeleteHeader("Transfer-Encoding"),
			poc.WithAppendHeaderIfNotExist("User-Agent", consts.DefaultUserAgent),
			poc.WithReplaceFullHttpPacketPostParamsWithoutEscape(totalParams),
		)

	default:
		if len(method) > 0 {
			result = poc.FixPacketByPocOptions(lowhttp.TrimLeftHTTPPacket(result),
				poc.WithReplaceHttpPacketMethod(method),
				poc.WithReplaceFullHttpPacketQueryParamsWithoutEscape(totalParams),
				poc.WithDeleteHeader("Transfer-Encoding"),
				poc.WithDeleteHeader("Content-Type"),
				poc.WithAppendHeaderIfNotExist("User-Agent", consts.DefaultUserAgent),
				poc.WithReplaceHttpPacketBody(nil, false),
			)
		} else if chunkEncode {
			// chunk编码
			_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(result)
			result = lowhttp.ReplaceHTTPPacketBody(result, body, true)
		} else if uploadEncode {
			opts := make([]poc.PocConfigOption, 0)
			opts = append(opts, poc.WithReplaceHttpPacketBody(nil, false))
			if !onlyUsePostParams {
				opts = append(opts, poc.WithReplaceHttpPacketQueryParamRaw(""))
			}
			if len(totalParams) > 0 || (onlyUsePostParams && len(postParams.Items) > 0) {
				if onlyUsePostParams {
					// 使用有序的 postParams
					for _, param := range postParams.Items {
						for _, v := range param.Values {
							opts = append(opts, poc.WithAppendHttpPacketUploadFile(param.Key, "", v, ""))
						}
					}
				} else {
					// 使用 totalParams map
					for k, values := range totalParams {
						for _, v := range values {
							opts = append(opts, poc.WithAppendHttpPacketUploadFile(k, "", v, ""))
						}
					}
				}
			} else {
				opts = append(opts, poc.WithAppendHttpPacketUploadFile("key", "", "[value]", ""))
			}
			result = poc.FixPacketByPocOptions(lowhttp.TrimLeftHTTPPacket(result), opts...)
		}
	}

	flow.Text = result
	return nil
}
