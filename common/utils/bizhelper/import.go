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
	"unicode"

	"github.com/jinzhu/gorm"
	"github.com/segmentio/ksuid"
	"github.com/xdg-go/pbkdf2"
	"github.com/yaklang/yaklang/common/gmsm/sm4"
	"github.com/yaklang/yaklang/common/gmsm/sm4/padding"
	"github.com/yaklang/yaklang/common/utils"
)

// ImportConfig 定义了导入配置参数
type ImportConfig struct {
	FilePath         string                                                                 // 导入文件路径
	IsEncrypted      bool                                                                   // 是否为加密文件
	Password         string                                                                 // 加密文件密码
	UniqueIndexField string                                                                 // 唯一索引字段名，用于更新已存在的记录
	AllowOverwrite   bool                                                                   // 当记录已存在时是否允许覆盖
	MetaDataHandler  func(metadata MetaData) error                                          // 元数据处理回调
	PreReadHandler   func(name string, b []byte, metadata MetaData) (new []byte, err error) // 读取前处理回调
	AfterReadHandler func(name string, b []byte, metadata MetaData)                         // 读取后处理回调
	ErrorHandler     func(err error) (newErr error)                                         // 错误处理回调
}

// NewImportConfig 创建新的导入配置
func NewImportConfig(filepath string) *ImportConfig {
	return &ImportConfig{
		FilePath: filepath,
	}
}

// CallErrorHandler 调用错误处理器
func (cfg *ImportConfig) CallErrorHandler(err error) error {
	if cfg.ErrorHandler != nil {
		return cfg.ErrorHandler(err)
	}
	return err
}

// ImportOption 导入选项函数类型
type ImportOption func(*ImportConfig)

// WithImportPassword 设置导入文件密码
func WithImportPassword(password string) ImportOption {
	return func(config *ImportConfig) {
		config.IsEncrypted = true
		config.Password = password
	}
}

// WithImportUniqueIndexField 设置唯一索引字段，用于去重和更新
func WithImportUniqueIndexField(uniqueIndex string) ImportOption {
	return func(config *ImportConfig) {
		config.UniqueIndexField = uniqueIndex
	}
}

// WithImportAllowOverwrite 设置是否允许覆盖已存在的记录
func WithImportAllowOverwrite(allowOverwrite bool) ImportOption {
	return func(config *ImportConfig) {
		config.AllowOverwrite = allowOverwrite
	}
}

// WithMetaDataHandler 设置元数据处理回调
func WithMetaDataHandler(handler func(metadata MetaData) error) ImportOption {
	return func(config *ImportConfig) {
		config.MetaDataHandler = handler
	}
}

// WithImportPreReadHandler 设置读取前处理回调
func WithImportPreReadHandler(handler func(name string, b []byte, metadata MetaData) (new []byte, err error)) ImportOption {
	return func(config *ImportConfig) {
		config.PreReadHandler = handler
	}
}

// WithImportAfterReadHandler 设置读取后处理回调
func WithImportAfterReadHandler(handler func(name string, b []byte, metadata MetaData)) ImportOption {
	return func(config *ImportConfig) {
		config.AfterReadHandler = handler
	}
}

// WithImportErrorHandler 设置错误处理回调
func WithImportErrorHandler(handler func(err error) (newErr error)) ImportOption {
	return func(config *ImportConfig) {
		config.ErrorHandler = handler
	}
}

// tableImportTool 表导入工具，封装了导入逻辑
type tableImportTool[T any] struct {
	db            *gorm.DB
	ctx           context.Context
	unmarshalFunc func(b []byte) (*T, error)
	config        *ImportConfig
}

// NewTableImportTool 创建新的表导入工具
func NewTableImportTool[T any](ctx context.Context, db *gorm.DB, filepath string, unmarshalFunc func(b []byte) (*T, error), options ...ImportOption) *tableImportTool[T] {
	config := NewImportConfig(filepath)
	for _, option := range options {
		option(config)
	}

	return &tableImportTool[T]{
		db:            db,
		ctx:           ctx,
		unmarshalFunc: unmarshalFunc,
		config:        config,
	}
}

// generateSM4KeyIV 从密码生成SM4加密需要的密钥和初始向量
func generateSM4KeyIV(password string) (key, iv []byte) {
	dk := pbkdf2.Key([]byte(password), nil, 10000, 32, sha256.New)
	return dk[:sm4.BlockSize], dk[sm4.BlockSize:]
}

// unmarshal 反序列化数据
func (t *tableImportTool[T]) unmarshal(b []byte) (*T, error) {
	if t.unmarshalFunc == nil {
		d := new(T)
		if err := json.Unmarshal(b, &d); err != nil {
			return d, err
		}
		return d, nil
	}
	return t.unmarshalFunc(b)
}

// preReadHandler 调用读取前处理器
func (t *tableImportTool[T]) preReadHandler(name string, b []byte, metadata MetaData) ([]byte, error) {
	if t.config.PreReadHandler != nil {
		return t.config.PreReadHandler(name, b, metadata)
	}
	return b, nil
}

// afterReadHandler 调用读取后处理器
func (t *tableImportTool[T]) afterReadHandler(name string, b []byte, metadata MetaData) {
	if t.config.AfterReadHandler != nil {
		t.config.AfterReadHandler(name, b, metadata)
	}
}

// openZipReader 打开ZIP文件并返回读取器
func (t *tableImportTool[T]) openZipReader() (*zip.Reader, func(), error) {
	f, err := os.OpenFile(t.config.FilePath, os.O_RDONLY, 0644)
	if err != nil {
		return nil, nil, err
	}

	closeFunc := func() {
		f.Close()
	}

	info, err := f.Stat()
	if err != nil {
		closeFunc()
		return nil, nil, err
	}

	var decryptedReaderAt io.ReaderAt
	var tempFile *os.File

	if t.config.IsEncrypted {
		// 处理加密文件
		bufReader := bufio.NewReaderSize(f, 4096)
		magic := make([]byte, len(ExportFileMagicNumber))
		n, err := bufReader.Read(magic)
		if err != nil {
			closeFunc()
			return nil, nil, err
		}
		if n != len(ExportFileMagicNumber) || !bytes.Equal(magic, ExportFileMagicNumber) {
			closeFunc()
			return nil, nil, utils.Error("invalid magic number, maybe file is broken")
		}

		// 创建临时文件用于解密
		tempFile, err = utils.OpenTempFile(fmt.Sprintf("import_table_%s.zip", ksuid.New().String()))
		if err != nil {
			closeFunc()
			return nil, nil, utils.Wrap(err, "failed to create temp file")
		}

		key, iv := generateSM4KeyIV(t.config.Password)
		_, err = sm4.GCMDecryptStream(key, iv, nil, bufReader, padding.NewPKCSPaddingWriter(tempFile, 16))
		if err != nil {
			tempFile.Close()
			closeFunc()
			return nil, nil, err
		}

		decryptedReaderAt = tempFile
		// 更新关闭函数
		oldCloseFunc := closeFunc
		closeFunc = func() {
			tempFile.Close()
			oldCloseFunc()
		}
	} else {
		decryptedReaderAt = f
	}

	zipReader, err := zip.NewReader(decryptedReaderAt, info.Size())
	if err != nil {
		closeFunc()
		return nil, nil, err
	}

	return zipReader, closeFunc, nil
}

// readMetadata 读取元数据
func (t *tableImportTool[T]) readMetadata(zipReader *zip.Reader) (MetaData, error) {
	var metadata MetaData

	// 读取 meta.json 文件
	if f, err := zipReader.Open(MetaJSONFileName); err == nil {
		defer f.Close()

		b, err := io.ReadAll(f)
		if err != nil {
			if err = t.config.CallErrorHandler(err); err != nil {
				return nil, err
			}
		}
		if err = json.Unmarshal(b, &metadata); err != nil {
			if err = t.config.CallErrorHandler(err); err != nil {
				return nil, err
			}
		}
	}

	// 调用元数据处理器
	if t.config.MetaDataHandler != nil {
		err := t.config.MetaDataHandler(metadata)
		if err != nil {
			return nil, err
		}
	}

	return metadata, nil
}

// createOrUpdateByUniqueIndex 根据唯一索引创建或更新记录
func (t *tableImportTool[T]) createOrUpdateByUniqueIndex(d *T) error {
	typ := reflect.TypeOf(d)

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	// 检查值是否为结构体并且有指定的索引字段
	if typ.Kind() != reflect.Struct {
		return utils.Error("table must be a struct")
	}

	field, ok := typ.FieldByName(t.config.UniqueIndexField)
	if !ok {
		return utils.Errorf("field %s not found in struct %s", t.config.UniqueIndexField, typ.Name())
	}
	colName := field.Tag.Get("json")
	if colName == "" {
		colName = toSnakeCase(t.config.UniqueIndexField)
	}

	value := reflect.ValueOf(d).Elem().FieldByName(field.Name)
	existing := new(T)
	err := t.db.Where(fmt.Sprintf("%s = ?", colName), value.Interface()).First(existing).Error
	if err == nil {
		// 记录存在，只有在允许覆盖时才更新
		if t.config.AllowOverwrite {
			if err = t.db.Model(existing).Updates(d).Error; err != nil {
				if err = t.config.CallErrorHandler(err); err != nil {
					return err
				}
			}
		}
	} else if gorm.IsRecordNotFoundError(err) {
		// 记录不存在，创建新记录
		if err = t.db.Create(d).Error; err != nil {
			if err = t.config.CallErrorHandler(err); err != nil {
				return err
			}
		}
	} else {
		// 其他错误
		if err = t.config.CallErrorHandler(err); err != nil {
			return err
		}
	}
	return nil
}

// processFile 处理ZIP文件中的单个文件
func (t *tableImportTool[T]) processFile(file *zip.File, metadata MetaData) error {
	rc, err := file.Open()
	if err != nil {
		if err = t.config.CallErrorHandler(err); err != nil {
			return err
		}
		return nil
	}
	defer rc.Close()

	name := file.Name
	b, err := io.ReadAll(rc)
	if err != nil {
		if err = t.config.CallErrorHandler(err); err != nil {
			return err
		}
		return nil
	}

	// 应用读取前处理器
	if b, err = t.preReadHandler(name, b, metadata); err != nil {
		if err = t.config.CallErrorHandler(err); err != nil {
			return err
		}
		return nil
	}

	// 反序列化数据
	d, err := t.unmarshal(b)
	if err != nil {
		if err = t.config.CallErrorHandler(err); err != nil {
			return err
		}
		return nil
	}

	// 根据配置创建或更新记录
	if t.config.UniqueIndexField != "" {
		if err = t.createOrUpdateByUniqueIndex(d); err != nil {
			if err = t.config.CallErrorHandler(err); err != nil {
				return err
			}
		}
	} else {
		if err = t.db.Create(d).Error; err != nil {
			if err = t.config.CallErrorHandler(err); err != nil {
				return err
			}
		}
	}

	// 调用读取后处理器
	t.afterReadHandler(name, b, metadata)

	return nil
}

// importFromZip 从ZIP文件导入数据
func (t *tableImportTool[T]) importFromZip() error {
	zipReader, closeFunc, err := t.openZipReader()
	if err != nil {
		return err
	}
	defer closeFunc()

	// 读取元数据
	metadata, err := t.readMetadata(zipReader)
	if err != nil {
		return err
	}

	// 处理每个文件
	for _, file := range zipReader.File {
		if file.Name == MetaJSONFileName {
			continue // 跳过元数据文件
		}

		if err := t.processFile(file, metadata); err != nil {
			return err
		}
	}

	return nil
}

// ImportTableZip 导入ZIP格式的表数据，使用默认的JSON反序列化
func ImportTableZip[T any](ctx context.Context, db *gorm.DB, filepath string, options ...ImportOption) (err error) {
	tool := NewTableImportTool[T](ctx, db, filepath, func(b []byte) (*T, error) {
		d := new(T)
		if err := json.Unmarshal(b, &d); err != nil {
			return d, err
		}
		return d, nil
	}, options...)
	return tool.importFromZip()
}

// ImportTableZipWithMarshalFunc 导入ZIP格式的表数据，使用自定义的反序列化函数
func ImportTableZipWithMarshalFunc[T any](ctx context.Context, db *gorm.DB, filepath string, unmarshalFunc func(b []byte) (*T, error), options ...ImportOption) (err error) {
	tool := NewTableImportTool[T](ctx, db, filepath, unmarshalFunc, options...)
	return tool.importFromZip()
}

// toSnakeCase 将驼峰命名转换为蛇形命名
func toSnakeCase(s string) string {
	var buf bytes.Buffer
	for i, c := range s {
		if unicode.IsUpper(c) {
			if i > 0 && (i+1 >= len(s) || unicode.IsLower(rune(s[i+1]))) {
				buf.WriteRune('_')
			}
			buf.WriteRune(unicode.ToLower(c))
		} else {
			buf.WriteRune(c)
		}
	}
	return buf.String()
}
