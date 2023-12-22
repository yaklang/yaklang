package codecutils

import (
	"bytes"
	_ "embed"
	"encoding/gob"
	"github.com/BurntSushi/toml"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed codecDoc.gob.gzip
var codecDoc []byte

var CodecLibs *yakdoc.ScriptLib
var CodecLibsDoc []*ypb.CodecMethod // 记录函数的数据，参数类型等，用于前端生成样式

type outputType = string

var (
	OUTPUT_RAW outputType = "raw"
	OUTPUT_HEX outputType = "hex"
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
	CodecLibs = CodecDocumentHelper.StructMethods["github.com/yaklang/yaklang/common/yakgrpc/codecutils.CodecExecFlow"]

	for funcName, funcInfo := range CodecLibs.Functions {
		var CodecMethod ypb.CodecMethod
		_, err = toml.Decode(funcInfo.Document, &CodecMethod)
		if err != nil {
			continue
		}
		CodecMethod.CodecName = funcName
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
// CodecMethod = "AES对称加密"
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{32}|[a-fA-F0-9]{48}|[a-fA-F0-9]{64}$"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{32}$"},
// { Name = "mode", Type = "select", Options = ["CBC", "ECB", "GCM"], Required = true },
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true }
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
// CodecMethod = "AES对称解密"
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{32}|[a-fA-F0-9]{48}|[a-fA-F0-9]{64}$"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{32}$"},
// { Name = "mode", Type = "select", Options = ["CBC", "ECB", "GCM"], Required = true },
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true }
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
// CodecMethod = "SM4对称加密"
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{32}$"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{32}$"},
// { Name = "mode", Type = "select", Options = ["CBC", "ECB", "GCM", "CFB", "OFB"], Required = true },
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true }
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
// CodecMethod = "SM4对称解密"
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{32}$"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{32}$"},
// { Name = "mode", Type = "select", Options = ["CBC", "ECB", "GCM", "CFB", "OFB"], Required = true },
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true }
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
// CodecMethod = "DES对称加密"
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{16}$"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{16}$"},
// { Name = "mode", Type = "select", Options = ["CBC", "ECB"], Required = true },
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true }
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
// CodecMethod = "DES对称解密"
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{16}$"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{16}$"},
// { Name = "mode", Type = "select", Options = ["CBC", "ECB"], Required = true },
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true }
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
// CodecMethod = "TripleDES对称加密"
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{32}|[a-fA-F0-9]{48}$"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{16}$" },
// { Name = "mode", Type = "select", Options = ["CBC", "ECB"], Required = true },
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true }
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
// CodecMethod = "TripleDES对称解密"
// Params = [
// { Name = "hexKey", Type = "input", Required = true, Regex = "^[a-fA-F0-9]{32}|[a-fA-F0-9]{48}$"},
// { Name = "hexIV", Type = "input", Required = false, Regex = "^[a-fA-F0-9]{16}$" },
// { Name = "mode", Type = "select", Options = ["CBC", "ECB"], Required = true },
// { Name = "output", Type = "select", Options = ["hex", "raw"], Required = true }
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

//func (flow *CodecExecFlow) JavaUnserialize(inputMod string) {
//	var data []byte
//	var err error
//	switch inputMod {
//	case "raw":
//	case "hex":
//	case "base64":
//	}
//}
