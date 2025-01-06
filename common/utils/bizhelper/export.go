package bizhelper

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/segmentio/ksuid"
	"github.com/xdg-go/pbkdf2"
	"github.com/yaklang/yaklang/common/gmsm/sm4"
	"github.com/yaklang/yaklang/common/gmsm/sm4/padding"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	MetaJSONFileName = "meta.json"
)

var (
	ExportFileMagicNumber = []byte{0xff, 0xff, 0xee, 0xee}
)

type MetaData = map[string]any

type ExportConfig struct {
	FilePath          string
	IsEncrypted       bool
	PreWriteHandler   func(name string, b []byte, metadata MetaData) (newName string, new []byte)
	AfterWriteHandler func(name string, b []byte, metadata MetaData)
	MetaData          MetaData
	Password          string // Password for encrypted file
}

func NewExportConfig(filepath string) *ExportConfig {
	return &ExportConfig{FilePath: filepath}
}

type ExportOption func(*ExportConfig)

func WithExportPassword(password string) ExportOption {
	return func(config *ExportConfig) {
		config.IsEncrypted = true
		config.Password = password
	}
}

func WithExportPreWriteHandler(handler func(name string, w []byte, metadata MetaData) (newName string, new []byte)) ExportOption {
	return func(config *ExportConfig) {
		config.PreWriteHandler = handler
	}
}

func WithExportAfterWriteHandler(handler func(name string, w []byte, metadata MetaData)) ExportOption {
	return func(config *ExportConfig) {
		config.AfterWriteHandler = handler
	}
}

func WithExportMetadata(metadata MetaData) ExportOption {
	return func(config *ExportConfig) {
		config.MetaData = metadata
	}
}

type ImportConfig struct {
	FilePath         string
	IsEncrypted      bool
	Password         string // Password for encrypted file
	MetaDataHandler  func(metadata MetaData) error
	PreReadHandler   func(name string, b []byte, metadata MetaData) (new []byte, err error)
	AfterReadHandler func(name string, b []byte, metadata MetaData)
}

func NewImportConfig(filepath string) *ImportConfig {
	return &ImportConfig{FilePath: filepath}
}

type ImportOption func(*ImportConfig)

func WithImportPassword(password string) ImportOption {
	return func(config *ImportConfig) {
		config.IsEncrypted = true
		config.Password = password
	}
}

func WithMetaDataHandler(handler func(metadata MetaData) error) ImportOption {
	return func(config *ImportConfig) {
		config.MetaDataHandler = handler
	}
}

func WithImportPreReadHandler(handler func(name string, b []byte, metadata MetaData) (new []byte, err error)) ImportOption {
	return func(config *ImportConfig) {
		config.PreReadHandler = handler
	}
}

func WithImportAfterReadHandler(handler func(name string, b []byte, metadata MetaData)) ImportOption {
	return func(config *ImportConfig) {
		config.AfterReadHandler = handler
	}
}

func generateSM4KeyIV(password string) (key, iv []byte) {
	dk := pbkdf2.Key([]byte(password), nil, 10000, 32, sha256.New)
	return dk[:sm4.BlockSize], dk[sm4.BlockSize:]
}

func ImportTableZip[T any](ctx context.Context, db *gorm.DB, filepath string, options ...ImportOption) (err error) {
	config := NewImportConfig(filepath)
	for _, option := range options {
		option(config)
	}
	fp := config.FilePath
	if config.IsEncrypted && !strings.HasSuffix(fp, ".enc") {
		return utils.Error("file is encrypted but not end with .enc")
	}

	f, err := os.OpenFile(config.FilePath, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}
	defer func() {
		nErr := f.Close()
		if err == nil {
			err = nErr
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	var bufReader *bufio.Reader // only for encrypted file
	if config.IsEncrypted {
		bufReader = bufio.NewReaderSize(f, 4096)
		magic := make([]byte, len(ExportFileMagicNumber))
		n, err := bufReader.Read(magic)
		if err != nil {
			return err
		}
		if n != len(ExportFileMagicNumber) || !bytes.Equal(magic, ExportFileMagicNumber) {
			return utils.Error("invalid magic number, maybe file is broken")
		}
	}
	var decryptedReaderAt io.ReaderAt

	if config.IsEncrypted {
		nf, err := utils.OpenTempFile(fmt.Sprintf("import_table_%s.zip", ksuid.New().String()))
		if err != nil {
			return utils.Wrap(err, "failed to create temp file")
		}
		defer func() {
			nErr := nf.Close()
			if err == nil {
				err = nErr
			}
		}()
		key, iv := generateSM4KeyIV(config.Password)

		_, err = sm4.GCMDecryptStream(key, iv, nil, bufReader, padding.NewPKCSPaddingWriter(nf, 16))
		if err != nil {
			return err
		}
		decryptedReaderAt = nf
	} else {
		decryptedReaderAt = f
	}

	zipReader, err := zip.NewReader(decryptedReaderAt, info.Size())
	if err != nil {
		return err
	}
	var metadata MetaData
	// read meta.json first
	if f, err := zipReader.Open(MetaJSONFileName); err == nil {
		defer f.Close()

		b, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(b, &metadata); err != nil {
			return err
		}
	}
	if config.MetaDataHandler != nil {
		err = config.MetaDataHandler(metadata)
		if err != nil {
			return err
		}
	}

	for _, f := range zipReader.File {
		if f.Name == MetaJSONFileName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		name := f.Name

		b, err := io.ReadAll(rc)
		if err != nil {
			return err
		}
		if config.PreReadHandler != nil {
			if b, err = config.PreReadHandler(name, b, metadata); err != nil {
				return err
			}
		}

		d := new(T)
		if err = json.Unmarshal(b, d); err != nil {
			return err
		}
		if err = db.Create(d).Error; err != nil {
			return err
		}
		if config.AfterReadHandler != nil {
			config.AfterReadHandler(name, b, metadata)
		}
	}

	return nil
}

func ExportTableZip[T any](ctx context.Context, db *gorm.DB, filepath string, options ...ExportOption) (err error) {
	config := NewExportConfig(filepath)
	for _, option := range options {
		option(config)
	}
	fp := config.FilePath
	if config.IsEncrypted && !strings.HasSuffix(fp, ".enc") {
		config.FilePath = fmt.Sprintf("%s.enc", fp)
	}

	f, err := os.OpenFile(config.FilePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() {
		nErr := f.Close()
		if err == nil {
			err = nErr
		}
	}()
	bufWriter := bufio.NewWriterSize(f, 4096)
	defer func() {
		nErr := bufWriter.Flush()
		if err == nil {
			err = nErr
		}
	}()

	r, w := utils.NewBufPipe(make([]byte, 4096))
	zipWriter := zip.NewWriter(w)
	contentCh := make(chan []byte, 16)

	preWriteHandler := func(b []byte, name string, metadata MetaData) (string, []byte) {
		if config.PreWriteHandler != nil {
			name, b = config.PreWriteHandler(name, b, metadata)
		}
		return name, b
	}
	fixName := func(name string) string {
		if name == "" {
			name = fmt.Sprintf("%s.json", ksuid.New().String())
		} else if !strings.HasSuffix(name, ".json") {
			name = fmt.Sprintf("%s.json", name)
		}
		return name
	}

	writeZipFile := func(p []byte, name string) (n int, err error) {
		w, err := zipWriter.Create(name)
		if err != nil {
			return 0, err
		}

		if n, err = w.Write(p); err != nil {
			return n, err
		}
		zipWriter.Flush()
		return n, nil
	}

	metadata := config.MetaData
	if config.MetaData == nil {
		metadata = make(MetaData)
	}
	chErr := make(chan error, 1)
	go func() {
		defer func() {
			close(chErr)
			close(contentCh)
			w.Close()
		}()

		ch := YieldModel[T](ctx, db)
		for d := range ch {
			v := reflect.ValueOf(d)
			if v.Kind() == reflect.Pointer {
				v = v.Elem()
			}
			if v.Kind() == reflect.Struct {
				id := v.FieldByName("ID")
				if id.IsValid() && id.CanSet() {
					id.SetUint(0)
				}
			}
			b, err := json.Marshal(d)
			if err != nil {
				chErr <- err
				return
			}

			name := ""
			name, b = preWriteHandler(b, name, metadata)
			name = fixName(name)
			writeZipFile(b, name)
			if config.AfterWriteHandler != nil {
				config.AfterWriteHandler(name, b, metadata)
			}
		}

		// write meta.json finally
		if len(metadata) > 0 {
			b, err := json.Marshal(metadata)
			if err != nil {
				chErr <- err
				return
			}
			writeZipFile(b, MetaJSONFileName)
		}

		if err = zipWriter.Close(); err != nil {
			chErr <- err
		}
	}()

	if config.IsEncrypted {
		bufWriter.Write(ExportFileMagicNumber)

		key, iv := generateSM4KeyIV(config.Password)
		_, err = sm4.GCMEncryptStream(key, iv, nil, padding.NewPKCSPaddingReader(r, 16), bufWriter)
	} else {
		_, err = io.Copy(bufWriter, r)
	}
	if err != nil {
		return err
	}

	select {
	case err = <-chErr:
		if err != nil {
			return err
		}
	default:
	}

	return nil
}
