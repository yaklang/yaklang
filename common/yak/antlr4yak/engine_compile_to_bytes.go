package antlr4yak

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"yaklang.io/yaklang/common/consts"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yak/antlr4yak/yakvm"
	"yaklang.io/yaklang/common/yak/yaklib/codec"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protowire"
)

var (
	MAGIC_NUMBER        = []byte{0xbc, 0xed}
	CRYPTO_MAGIC_NUMBER = []byte{0xcc, 0xed}
)

func IsYakc(b []byte) bool {
	return IsNormalYakc(b) || IsCryptoYakc(b)
}

var yakcCache = new(sync.Map)

func HaveYakcCache(code string) ([]byte, bool) {
	return HaveYakcCacheWithKey(code, nil)
}

func calcHash(code string, key []byte) string {
	codeHashBasic := codec.Sha256(code + consts.GetYakVersion())
	if key != nil {
		return codec.Sha256(codeHashBasic + string(key))
	}
	return codeHashBasic
}

func HaveYakcCacheWithKey(code string, key []byte) ([]byte, bool) {
	if len(code) <= YAKC_CACHE_MAX_LENGTH {
		return nil, false
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("fetch yakc cache failed: %s", err)
		}
	}()
	codeHash := calcHash(code, key)

	yakcBytes, ok := yakcCache.Load(codeHash)
	if ok {
		return yakcBytes.([]byte), true
	}

	// 完整性校验双 Hash
	dir := consts.GetDefaultYakitBaseTempDir()
	absPath := filepath.Join(dir, fmt.Sprintf(".%v.yakc", codeHash))
	SipAbsPath := filepath.Join(dir, fmt.Sprintf(".%v.yakc.sip", codeHash))
	if stat, _ := os.Stat(absPath); stat != nil && !stat.IsDir() {
		raw, _ := ioutil.ReadFile(absPath)
		if raw != nil {
			sipHashExpected := codec.Sha256(string(raw))
			sipHash, _ := ioutil.ReadFile(SipAbsPath)
			if sipHash == nil || sipHashExpected != string(sipHash) {
				os.RemoveAll(absPath)
				os.RemoveAll(SipAbsPath)
				return nil, false
			}
			yakcCache.Store(codeHash, raw)
			return raw, true
		}
		return nil, false
	}
	return nil, false
}

func SaveYakcCache(code string, yakc []byte) {
	SaveYakcCacheWithKey(code, yakc, nil)
}

func SaveYakcCacheWithKey(code string, yakc []byte, key []byte) {
	if len(code) <= YAKC_CACHE_MAX_LENGTH {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("save yakc cache failed: %s", err)
		}
	}()

	if !IsYakc(yakc) {
		return
	}

	hash := calcHash(code, key)

	_, ok := yakcCache.Load(hash)
	if ok {
		return
	}
	yakcCache.Store(hash, yakc)

	dir := consts.GetDefaultYakitBaseTempDir()
	sipHash := codec.Sha256(yakc)
	absPath := filepath.Join(dir, fmt.Sprintf(".%v.yakc", hash))
	SipAbsPath := filepath.Join(dir, fmt.Sprintf(".%v.yakc.sip", hash))

	err := ioutil.WriteFile(absPath, yakc, 0644)
	if err != nil {
		log.Errorf("cache %v failed: %s", absPath, err)
	}
	err = ioutil.WriteFile(SipAbsPath, []byte(sipHash), 0644)
	if err != nil {
		log.Errorf("cache %v failed: %s", SipAbsPath, err)
	}
}

func IsNormalYakc(b []byte) bool {
	return bytes.HasPrefix(b, MAGIC_NUMBER)
}

func IsCryptoYakc(b []byte) bool {
	return bytes.HasPrefix(b, CRYPTO_MAGIC_NUMBER)
}

func (n *Engine) _marshal(symtbl *yakvm.SymbolTable, codes []*yakvm.Code, key []byte) ([]byte, error) {
	hasKey := len(key) > 0

	// create marshal
	m := yakvm.NewCodesMarshaller()
	b, err := m.Marshal(symtbl, codes)
	if err != nil {
		return nil, err
	}

	var header []byte
	// header
	if hasKey {
		header = CRYPTO_MAGIC_NUMBER[:]
	} else {
		header = MAGIC_NUMBER[:]
	}
	// 	version
	header = protowire.AppendBytes(header, []byte(consts.GetYakVersion()))

	// gzip
	b, err = utils.GzipCompress(b)
	if err != nil {
		return nil, errors.Wrapf(err, "gzip compress failed")
	}
	// SM4GCM if key
	if hasKey {
		b, err = codec.SM4GCMEnc(key, b, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "sm4 encrypt failed")
		}
	}

	b = append(header, b...)
	return b, nil
}

func (n *Engine) Marshal(code string, key []byte) ([]byte, error) {
	cl, err := n._compile(code)
	if err != nil {
		return nil, err
	}
	return n._marshal(cl.GetRootSymbolTable(), cl.GetOpcodes(), key)
}

func (n *Engine) UnMarshal(b []byte, key []byte) (*yakvm.SymbolTable, []*yakvm.Code, error) {
	var err error
	hasKey, isCrypto := len(key) > 0, IsCryptoYakc(b)

	if !IsYakc(b) {
		return nil, nil, utils.Errorf("invalid yakc file, bad magic number: %s", hex.EncodeToString(b[:2]))
	}
	if !hasKey && isCrypto {
		return nil, nil, utils.Errorf("The yakc file has been encrypted, need key to decrypt(use --key/-k to use key)")
	}

	// header
	b = b[2:]
	// 	version
	version, i := protowire.ConsumeBytes(b)
	_ = version
	b = b[i:]

	// SM4GCM if key and is encrypted
	if hasKey {
		if isCrypto {
			b, err = codec.SM4GCMDec(key, b, nil)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "sm4 decrypt failed")
			}
		} else {
			log.Warnf("the key is provided but not used")
		}

	}

	// gzip
	b, err = utils.GzipDeCompress(b)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "maybe sm4 decrypt failed: gzip decompress failed")
	}

	m := yakvm.NewCodesMarshaller()
	return m.Unmarshal(b)
}

func (n *Engine) SafeExecYakc(ctx context.Context, b []byte, key []byte) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = fmt.Errorf("exec yakc failed: %s", err)
		}
	}()
	return n.ExecYakc(ctx, b, key)
}

func (n *Engine) ExecYakc(ctx context.Context, b []byte, key []byte) error {
	symbolTable, codes, err := n.UnMarshal(b, key)
	if err != nil {
		return err
	}

	n.vm.SetSymboltable(symbolTable)
	err = n.vm.Exec(ctx, func(frame *yakvm.Frame) {
		frame.SetVerbose("global code")
		frame.Exec(codes)
	})
	return err
}

func (n *Engine) SafeExecYakcWithCode(ctx context.Context, b []byte, key []byte, code string) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = fmt.Errorf("exec yakc failed: %s", err)
		}
	}()
	return n.ExecYakcWithCode(ctx, b, key, code)
}

func (n *Engine) ExecYakcWithCode(ctx context.Context, b []byte, key []byte, code string) error {
	symtbl, codes, err := n.UnMarshal(b, key)
	if err != nil {
		return err
	}
	n.vm.SetSymboltable(symtbl)
	return n.vm.ExecYakCode(ctx, code, codes, yakvm.None)
}
