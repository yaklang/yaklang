// Package bizhelper 提供业务逻辑相关的帮助函数和工具
// 包含表数据的导入导出、分页查询、数据处理等常用功能
package bizhelper

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/gmsm/sm4"
	"github.com/yaklang/yaklang/common/gmsm/sm4/padding"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// MetaJSONFileName 元数据文件名，存储在ZIP文件中
	MetaJSONFileName = "meta.json"
)

var (
	// ExportFileMagicNumber 加密文件的魔数，用于标识加密文件
	ExportFileMagicNumber = []byte{0xff, 0xff, 0xee, 0xee}
)

// MetaData 定义元数据类型，用于存储导出/导入过程中的额外信息
type MetaData = map[string]any

// ExportConfig 导出配置结构体，包含导出过程中的所有配置选项
type ExportConfig struct {
	FilePath          string                                                                      // 导出文件路径
	IsEncrypted       bool                                                                        // 是否加密导出文件
	PreWriteHandler   func(name string, b []byte, metadata MetaData) (newName string, new []byte) // 写入前数据处理回调
	AfterWriteHandler func(name string, b []byte, metadata MetaData)                              // 写入后处理回调
	MetaData          MetaData                                                                    // 自定义元数据
	Password          string                                                                      // 加密密码
	IndexField        string                                                                      // 用于分页的索引字段
	WriteBufferInMem  bool                                                                        // 是否在内存中缓冲写入数据
}

// NewExportConfig 创建新的导出配置
// filepath: 导出文件路径
func NewExportConfig(filepath string) *ExportConfig {
	return &ExportConfig{FilePath: filepath, IndexField: "id", WriteBufferInMem: false}
}

// ExportOption 导出选项函数类型，用于配置导出参数
type ExportOption func(*ExportConfig)

// WithExportWriteBufferInMem 设置是否在内存中缓冲写入数据
// writeBufferInMem: true表示使用内存缓冲，false表示使用操作系统管道
func WithExportWriteBufferInMem(writeBufferInMem bool) ExportOption {
	return func(config *ExportConfig) {
		config.WriteBufferInMem = writeBufferInMem
	}
}

// WithExportIndexField 设置用于分页查询的索引字段
// field: 字段名，通常为"id"或其他具有顺序性的字段
func WithExportIndexField(field string) ExportOption {
	return func(config *ExportConfig) {
		config.IndexField = field
	}
}

// WithExportPassword 设置导出文件的加密密码
// password: 加密密码，设置后文件将使用SM4算法加密
func WithExportPassword(password string) ExportOption {
	return func(config *ExportConfig) {
		config.IsEncrypted = true
		config.Password = password
	}
}

// WithExportPreWriteHandler 设置写入前数据处理回调
// handler: 处理函数，可用于修改文件名和数据内容
func WithExportPreWriteHandler(handler func(name string, w []byte, metadata MetaData) (newName string, new []byte)) ExportOption {
	return func(config *ExportConfig) {
		config.PreWriteHandler = handler
	}
}

// WithExportAfterWriteHandler 设置写入后处理回调
// handler: 处理函数，可用于记录写入日志或执行其他后续操作
func WithExportAfterWriteHandler(handler func(name string, w []byte, metadata MetaData)) ExportOption {
	return func(config *ExportConfig) {
		config.AfterWriteHandler = handler
	}
}

// WithExportMetadata 设置自定义元数据
// metadata: 将被写入到meta.json文件中的元数据
func WithExportMetadata(metadata MetaData) ExportOption {
	return func(config *ExportConfig) {
		config.MetaData = metadata
	}
}

// tableExportTool 表导出工具，封装了导出逻辑和配置
// 使用泛型T来支持任意类型的表结构
type tableExportTool[T any] struct {
	db          *gorm.DB                  // 数据库连接
	ctx         context.Context           // 上下文，用于取消操作
	marshalFunc func(v T) ([]byte, error) // 数据序列化函数
	config      *ExportConfig             // 导出配置
}

// NewTableExportTool 创建新的表导出工具
// ctx: 上下文
// db: 数据库连接
// filepath: 导出文件路径
// marshalFunc: 数据序列化函数，为nil时使用JSON序列化
// options: 导出选项
func NewTableExportTool[T any](ctx context.Context, db *gorm.DB, filepath string, marshalFunc func(v T) ([]byte, error), options ...ExportOption) *tableExportTool[T] {
	config := NewExportConfig(filepath)
	for _, option := range options {
		option(config)
	}
	fp := config.FilePath
	if config.IsEncrypted && !strings.HasSuffix(fp, ".enc") {
		config.FilePath = fmt.Sprintf("%s.enc", fp)
	}
	if config.MetaData == nil {
		config.MetaData = make(MetaData)
	}
	return &tableExportTool[T]{
		db:          db,
		ctx:         ctx,
		marshalFunc: marshalFunc,
		config:      config,
	}
}

// preWriteHandler 调用写入前处理回调
// b: 原始数据
// name: 文件名
// metadata: 元数据
// 返回: 处理后的文件名和数据
func (t *tableExportTool[T]) preWriteHandler(b []byte, name string, metadata MetaData) (string, []byte) {
	if t.config.PreWriteHandler != nil {
		name, b = t.config.PreWriteHandler(name, b, metadata)
	}
	name = fixCustomFileName(name)
	return name, b
}

// afterWriteHandler 调用写入后处理回调
// name: 文件名
// b: 数据
// metadata: 元数据
func (t *tableExportTool[T]) afterWriteHandler(name string, b []byte, metadata MetaData) {
	if t.config.AfterWriteHandler != nil {
		t.config.AfterWriteHandler(name, b, metadata)
	}
}

// marshal 序列化数据
// v: 要序列化的数据
// 返回: 序列化后的字节数组
func (t *tableExportTool[T]) marshal(v T) (b []byte, err error) {
	if t.marshalFunc == nil {
		return json.Marshal(v)
	}
	return t.marshalFunc(v)
}

// writeFile 写入文件到ZIP
// zipWriter: ZIP写入器
// name: 文件名
// b: 文件内容
// callHandle: 是否调用处理回调
// 返回: 写入字节数和错误
func (t *tableExportTool[T]) writeFile(zipWriter *zip.Writer, name string, b []byte, callHandle bool) (n int, err error) {
	if callHandle {
		name, b = t.preWriteHandler(b, name, t.config.MetaData)
		defer func() {
			t.afterWriteHandler(name, b, t.config.MetaData)
		}()
	}

	w, err := zipWriter.Create(name)
	if err != nil {
		return 0, err
	}

	if n, err = w.Write(b); err != nil {
		return n, err
	}
	zipWriter.Flush()

	return n, nil
}

// writeTable 将表数据写入ZIP
// zipWriter: ZIP写入器
// 返回: 错误信息
func (t *tableExportTool[T]) writeTable(zipWriter *zip.Writer) (err error) {
	ch := YieldModel[T](t.ctx, t.db, WithYieldModel_IndexField(t.config.IndexField))
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
		b, err := t.marshal(d)
		if err != nil {
			return err
		}

		name := ""
		t.writeFile(zipWriter, name, b, true)

	}
	metadata := t.config.MetaData
	// write meta.json finally
	if len(metadata) > 0 {
		b, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		t.writeFile(zipWriter, MetaJSONFileName, b, false)
	}

	if err = zipWriter.Close(); err != nil {
		return err
	}
	return nil
}

// newPipe 创建数据管道
// 根据配置选择使用内存缓冲或操作系统管道
// 返回: 读取器、写入器、关闭函数、错误
func (t *tableExportTool[T]) newPipe() (r io.Reader, w io.Writer, close func(), err error) {
	if t.config.WriteBufferInMem {
		reader, writer := utils.NewBufPipe(nil)

		return reader, writer, func() {
			writer.Close()
		}, nil
	} else {
		reader, writer, err := os.Pipe()
		return reader, writer, func() {
			writer.Close()
		}, err
	}
}

// writeZipStreamToFile 将ZIP数据流写入文件
// zipReadStream: ZIP数据流
// 返回: 错误信息
func (t *tableExportTool[T]) writeZipStreamToFile(zipReadStream io.Reader) (err error) {
	f, err := os.OpenFile(t.config.FilePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	// bufWriter := bufio.NewWriterSize(f, 4096)
	defer func() {
		// defer bufWriter.Flush()
		defer f.Close()
	}()
	bufWriter := f
	if t.config.IsEncrypted {
		bufWriter.Write(ExportFileMagicNumber)
		key, iv := generateSM4KeyIV(t.config.Password)
		_, err = sm4.GCMEncryptStream(key, iv, nil, padding.NewPKCSPaddingReader(zipReadStream, 16), bufWriter)
	} else {
		_, err = io.Copy(bufWriter, zipReadStream)
	}
	return
}

// export 执行导出操作
// 主要流程: 创建管道 -> 启动写入ZIP的goroutine -> 启动写入文件的goroutine -> 等待完成
// 返回: 错误信息
func (t *tableExportTool[T]) export() (err error) {
	// 创建管道，用于写入zip文件
	zipReadStream, zipWriteStream, closePipe, err := t.newPipe()
	if err != nil {
		return err
	}

	// 将zip写入管道
	zipWriter := zip.NewWriter(zipWriteStream)
	writeTableErr := make(chan error, 1)
	writeWg := &sync.WaitGroup{}
	writeWg.Add(1)
	go func() {
		defer func() {
			closePipe()
			close(writeTableErr)
			writeWg.Done()
		}()
		if err := t.writeTable(zipWriter); err != nil {
			writeTableErr <- err
		}
	}()

	// 从管道读取zip文件并写入到目标文件
	writeFileErr := make(chan error, 1)
	writeFileWg := &sync.WaitGroup{}
	writeFileWg.Add(1)
	go func() {
		defer func() {
			close(writeFileErr)
			writeFileWg.Done()
		}()
		err := t.writeZipStreamToFile(zipReadStream)
		if err != nil {
			writeFileErr <- err
		}
	}()

	// 先等待zip写入完成，再等待文件写入完成
	writeWg.Wait()
	writeFileWg.Wait()

	writeFileError := <-writeFileErr
	if writeFileError != nil {
		return writeFileError
	}
	writeTableError := <-writeTableErr
	if writeTableError != nil {
		return writeTableError
	}

	return nil
}

// ExportTableZipWithMarshalFunc 导出表数据为ZIP格式，使用自定义序列化函数
// 这是导出功能的核心函数，支持自定义数据序列化方式
// ctx: 上下文
// db: 数据库连接
// filepath: 导出文件路径
// marshalFunc: 数据序列化函数
// options: 导出选项
// 返回: 错误信息
func ExportTableZipWithMarshalFunc[T any](ctx context.Context, db *gorm.DB, filepath string, marshalFunc func(v T) ([]byte, error), options ...ExportOption) (err error) {
	tool := NewTableExportTool[T](ctx, db, filepath, func(v T) ([]byte, error) {
		return marshalFunc(v)
	}, options...)
	return tool.export()
}

// ExportTableZip 导出表数据为ZIP格式，使用默认的JSON序列化
// 这是最常用的导出函数，适用于大多数场景
// ctx: 上下文
// db: 数据库连接
// filepath: 导出文件路径
// options: 导出选项
// 返回: 错误信息
func ExportTableZip[T any](ctx context.Context, db *gorm.DB, filepath string, options ...ExportOption) (err error) {
	tool := NewTableExportTool[T](ctx, db, filepath, func(v T) ([]byte, error) {
		return json.Marshal(v)
	}, options...)
	return tool.export()
}
