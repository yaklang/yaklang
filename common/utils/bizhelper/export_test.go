package bizhelper

import (
	"archive/zip"
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/utils"
)

type HTTPFlow struct {
	gorm.Model

	HiddenIndex        string `gorm:"index" json:"hidden_index,omitempty"`
	NoFixContentLength bool   `json:"no_fix_content_length" json:"no_fix_content_length,omitempty"`
	Hash               string `gorm:"unique_index" json:"unique_index,omitempty"`
	IsHTTPS            bool   `json:"is_https,omitempty"`
	Url                string `gorm:"index" json:"url,omitempty"`
	Path               string `json:"path,omitempty"`
	Method             string `json:"method,omitempty"`
	RequestLength      int64  `json:"request_length,omitempty"`
	BodyLength         int64  `json:"body_length,omitempty"`
	ContentType        string `json:"content_type,omitempty"`
	StatusCode         int64  `json:"status_code,omitempty"`
	SourceType         string `json:"source_type,omitempty"`
	Request            string `json:"request,omitempty"`
	Response           string `json:"response,omitempty"`
	Duration           int64  `json:"duration,omitempty"`
	GetParamsTotal     int    `json:"get_params_total,omitempty"`
	PostParamsTotal    int    `json:"post_params_total,omitempty"`
	CookieParamsTotal  int    `json:"cookie_params_total,omitempty"`
	IPAddress          string `json:"ip_address,omitempty"`
	RemoteAddr         string `json:"remote_addr,omitempty"`
	IPInteger          int    `json:"ip_integer,omitempty"`
	Tags               string `json:"tags,omitempty"` // 用来打标！
	Payload            string `json:"payload,omitempty"`

	// Websocket 相关字段
	IsWebsocket bool `json:"is_websocket,omitempty"`
	// 用来计算 websocket hash, 每次连接都不一样，一般来说，内部对象 req 指针足够了
	WebsocketHash string `json:"websocket_hash,omitempty"`

	RuntimeId   string         `json:"runtime_id,omitempty" gorm:"index"`
	FromPlugin  string         `json:"from_plugin,omitempty"`
	ProcessName sql.NullString `json:"process_name,omitempty"`

	// friendly for gorm build instance, not for store
	// 这两个字段不参与数据库存储，但是在序列化的时候，会被覆盖
	// 主要用来标记用户的 Request 和 Response 是否超大
	IsRequestOversize  bool `gorm:"-" json:"is_request_oversize,omitempty"`
	IsResponseOversize bool `gorm:"-" json:"is_response_oversize,omitempty"`

	IsReadTooSlowResponse      bool   `json:"is_read_too_slow_response,omitempty"`
	IsTooLargeResponse         bool   `json:"is_too_large_response,omitempty"`
	TooLargeResponseHeaderFile string `json:"too_large_response_header_file,omitempty"`
	TooLargeResponseBodyFile   string `json:"too_large_response_body_file,omitempty"`
	// 同步到企业端
	UploadOnline bool `json:"upload_online,omitempty"`
}

func (f *HTTPFlow) BeforeSave() error {
	f.Hash = ksuid.New().String()
	return nil
}

func TestExportAndImport(t *testing.T) {
	runtimeID := ksuid.New().String()
	// db := consts.GetGormProjectDatabase().Model(&HTTPFlow{})
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	db = db.AutoMigrate(&HTTPFlow{})
	times := 5
	flows := make([]*HTTPFlow, 0, times)
	for i := 0; i < times; i++ {
		flow := &HTTPFlow{
			Url:       "http://example.com",
			Request:   ksuid.New().String(),
			RuntimeId: runtimeID,
		}
		resultDB := db.Save(&flow)
		require.NoError(t, resultDB.Error)
		flows = append(flows, flow)
	}
	t.Cleanup(func() {
		db.Unscoped().Where("runtime_id = ?", runtimeID).Delete(&HTTPFlow{})
	})
	queryDB := db.Where("runtime_id = ?", runtimeID)

	t.Run("export: no encrypt", func(t *testing.T) {
		fp := filepath.Join(t.TempDir(), "test1.zip")
		err := ExportTableZip[*HTTPFlow](context.Background(), queryDB, fp)
		require.NoError(t, err)
		require.FileExists(t, fp)
		r, err := zip.OpenReader(fp)
		defer r.Close()
		require.NoError(t, err)
		for _, f := range r.File {
			r, err := f.Open()
			require.NoError(t, err)
			data, err := io.ReadAll(r)
			require.NoError(t, err)
			require.Contains(t, string(data), runtimeID)
			require.Contains(t, string(data), `"ID":0`)
		}
		require.Len(t, r.File, times)
	})

	t.Run("export: no encrypt with pre handler", func(t *testing.T) {
		fp := filepath.Join(t.TempDir(), "test2.zip")
		count := 0
		err := ExportTableZip[*HTTPFlow](context.Background(), queryDB, fp, WithExportPreWriteHandler(func(_ string, w []byte, _ map[string]any) (name string, new []byte) {
			count++
			return strconv.Itoa(count), w
		}))
		require.NoError(t, err)
		require.FileExists(t, fp)
		r, err := zip.OpenReader(fp)
		defer r.Close()
		require.NoError(t, err)
		verifyMap := make(map[string]bool, count)
		for i := 0; i < count; i++ {
			verifyMap[strconv.Itoa(i+1)] = false
		}
		for _, f := range r.File {
			name := filepath.Base(f.Name)
			name = strings.TrimSuffix(name, ".json")
			if _, ok := verifyMap[name]; ok {
				verifyMap[name] = true
			} else {
				t.Fatalf("unexpected file name: %s", name)
			}

			r, err := f.Open()
			require.NoError(t, err)
			data, err := io.ReadAll(r)
			require.NoError(t, err)
			require.Contains(t, string(data), runtimeID)
		}
		require.Len(t, r.File, times)
		for _, v := range verifyMap {
			require.True(t, v)
		}
	})

	t.Run("export: no encrypt with metadata", func(t *testing.T) {
		fp := filepath.Join(t.TempDir(), "test2.zip")
		m := map[string]any{"test": ksuid.New().String()}
		token := ksuid.New().String()
		err := ExportTableZip[*HTTPFlow](context.Background(), queryDB, fp, WithExportPreWriteHandler(func(name string, w []byte, metadata map[string]any) (newName string, new []byte) {
			m["test2"] = token
			return name, w
		}), WithExportMetadata(m))

		require.NoError(t, err)
		require.FileExists(t, fp)
		r, err := zip.OpenReader(fp)
		defer r.Close()
		require.NoError(t, err)

		foundMeta := false

		for _, f := range r.File {
			name := filepath.Base(f.Name)

			r, err := f.Open()
			require.NoError(t, err)
			data, err := io.ReadAll(r)
			require.NoError(t, err)
			if name == MetaJSONFileName {
				foundMeta = true
				var metadata map[string]any
				require.NoError(t, json.Unmarshal(data, &metadata))
				require.Equal(t, m, metadata)
			} else {
				require.Contains(t, string(data), runtimeID)
			}
		}
		require.Len(t, r.File, times+1)
		require.True(t, foundMeta)
	})

	t.Run("export: encrypt", func(t *testing.T) {
		fp := filepath.Join(t.TempDir(), "test3.zip.enc")
		err := ExportTableZip[*HTTPFlow](context.Background(), queryDB, fp, WithExportPassword("password"))
		require.NoError(t, err)
		require.FileExists(t, fp)

		fh, err := os.Open(fp)
		require.NoError(t, err)
		defer fh.Close()

		r := bufio.NewReader(fh)
		gotMagicNumber, err := r.Peek(4)
		require.NoError(t, err)
		require.Equal(t, ExportFileMagicNumber, gotMagicNumber, "magic number not match")
	})

	exportHelper := func(t *testing.T, newRuntimeID string, password string, metadatas ...map[string]any) string {
		t.Helper()

		fp := filepath.Join(t.TempDir(), fmt.Sprintf("%s.zip", ksuid.New().String()))
		if password != "" {
			fp = fmt.Sprintf("%s.enc", fp)
		}

		options := make([]ExportOption, 0)
		options = append(options, WithExportPreWriteHandler(func(name string, w []byte, _ map[string]any) (newName string, new []byte) {
			s, err := jsonpath.ReplaceStringWithError(string(w), "$.runtime_id", newRuntimeID)
			require.NoError(t, err)
			s, err = jsonpath.ReplaceStringWithError(s, "$.request", ksuid.New().String())
			require.NoError(t, err)
			return name, utils.InterfaceToBytes(s)
		}))
		if password != "" {
			options = append(options, WithExportPassword(password))
		}
		if len(metadatas) > 0 {
			options = append(options, WithExportMetadata(metadatas[0]))
		}
		err := ExportTableZip[*HTTPFlow](context.Background(), queryDB, fp, options...)
		require.NoError(t, err)
		require.FileExists(t, fp)
		return fp
	}

	t.Run("import: no encrypt", func(t *testing.T) {
		runtimeID := ksuid.New().String()
		fp := exportHelper(t, runtimeID, "")
		err := ImportTableZip[HTTPFlow](context.Background(), db, fp)
		t.Cleanup(func() {
			db.Unscoped().Where("runtime_id = ?", runtimeID).Delete(&HTTPFlow{})
		})
		require.NoError(t, err)
		var flows []*HTTPFlow
		queryDB := db.Where("runtime_id = ?", runtimeID)
		if err := queryDB.Find(&flows).Error; err != nil {
			require.NoError(t, err)
		}
		require.Len(t, flows, times)
		for _, flow := range flows {
			require.Equal(t, runtimeID, flow.RuntimeId)
		}
	})

	t.Run("import: no encrypt with metadata", func(t *testing.T) {
		runtimeID := ksuid.New().String()
		m := map[string]any{"test": ksuid.New().String()}
		fp := exportHelper(t, runtimeID, "", m)

		err := ImportTableZip[HTTPFlow](context.Background(), db, fp, WithImportPreReadHandler(func(name string, b []byte, metadata map[string]any) (new []byte, err error) {
			if name != MetaJSONFileName {
				require.Equalf(t, m, metadata, "metadata not match")
			}
			return b, nil
		}))
		t.Cleanup(func() {
			db.Unscoped().Where("runtime_id = ?", runtimeID).Delete(&HTTPFlow{})
		})
		require.NoError(t, err)

		var flows []*HTTPFlow
		queryDB := db.Where("runtime_id = ?", runtimeID)
		if err := queryDB.Find(&flows).Error; err != nil {
			require.NoError(t, err)
		}
		require.Len(t, flows, times)
		for _, flow := range flows {
			require.Equal(t, runtimeID, flow.RuntimeId)
		}
	})

	t.Run("import: encrypt", func(t *testing.T) {
		password := ksuid.New().String() + ksuid.New().String()
		runtimeID := ksuid.New().String()
		fp := exportHelper(t, runtimeID, password)
		err := ImportTableZip[HTTPFlow](context.Background(), db, fp, WithImportPassword(password))
		t.Cleanup(func() {
			db.Unscoped().Where("runtime_id = ?", runtimeID).Delete(&HTTPFlow{})
		})
		require.NoError(t, err)

		var flows []*HTTPFlow
		queryDB := db.Where("runtime_id = ?", runtimeID)
		if err := queryDB.Find(&flows).Error; err != nil {
			require.NoError(t, err)
		}
		require.Len(t, flows, times)
		for _, flow := range flows {
			require.Equal(t, runtimeID, flow.RuntimeId)
		}
	})
}

type TestData struct {
	Key   string
	Value int
}

func TestZipIsValid(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	db = db.AutoMigrate(&TestData{})
	times := 5
	for i := 0; i < times; i++ {
		flow := &TestData{
			Key:   ksuid.New().String(),
			Value: i,
		}
		resultDB := db.Save(&flow)
		require.NoError(t, resultDB.Error)
	}
	fp := filepath.Join(t.TempDir(), "test.zip")
	err = ExportTableZip[*TestData](context.Background(), db, fp)
	require.NoError(t, err)
	require.FileExists(t, fp)

	data, err := os.ReadFile(fp)
	require.NoError(t, err)
	require.Len(t, data, 1067)
	os.Remove(fp)
}

type TestModel struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestExportTableZipWithMarshalFunc(t *testing.T) {
	// 创建临时数据库
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	defer db.Close()

	// 创建表
	err = db.AutoMigrate(&TestModel{}).Error
	require.NoError(t, err)

	// 插入测试数据
	testData := []TestModel{
		{Name: "Alice", Age: 25},
		{Name: "Bob", Age: 30},
		{Name: "Charlie", Age: 35},
	}

	for _, data := range testData {
		err = db.Create(&data).Error
		require.NoError(t, err)
	}

	// 创建临时文件路径
	tempDir, err := os.MkdirTemp("", "test_export")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	zipPath := filepath.Join(tempDir, "test_export.zip")

	// 测试导出
	err = ExportTableZipWithMarshalFunc[TestModel](
		context.Background(),
		db,
		zipPath,
		func(v TestModel) ([]byte, error) {
			return json.Marshal(v)
		},
	)
	require.NoError(t, err)

	// 验证文件存在
	_, err = os.Stat(zipPath)
	require.NoError(t, err)

	// 验证zip文件可以正常打开和解压
	zipFile, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer zipFile.Close()

	// 验证zip文件内容
	require.True(t, len(zipFile.File) > 0, "zip文件应该包含文件")

	// 读取并验证每个文件
	var extractedData []TestModel
	for _, file := range zipFile.File {
		if file.Name == MetaJSONFileName {
			continue // 跳过元数据文件
		}

		rc, err := file.Open()
		require.NoError(t, err)

		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		rc.Close()

		var model TestModel
		err = json.Unmarshal(data, &model)
		require.NoError(t, err)

		extractedData = append(extractedData, model)
	}

	// 验证数据完整性
	require.Equal(t, len(testData), len(extractedData), "导出的数据数量应该与原始数据相同")

	// 验证数据内容
	for i, original := range testData {
		found := false
		for _, extracted := range extractedData {
			if original.Name == extracted.Name && original.Age == extracted.Age {
				found = true
				break
			}
		}
		require.True(t, found, "应该能找到原始数据项 %d", i)
	}
}

func TestExportTableZipWithMarshalFuncEncrypted(t *testing.T) {
	// 创建临时数据库
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	defer db.Close()

	// 创建表
	err = db.AutoMigrate(&TestModel{}).Error
	require.NoError(t, err)

	// 插入测试数据
	testData := []TestModel{
		{Name: "Alice", Age: 25},
		{Name: "Bob", Age: 30},
	}

	for _, data := range testData {
		err = db.Create(&data).Error
		require.NoError(t, err)
	}

	// 创建临时文件路径
	tempDir, err := os.MkdirTemp("", "test_export_encrypted")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	zipPath := filepath.Join(tempDir, "test_export.zip")
	password := "test_password_123"

	// 测试加密导出
	err = ExportTableZipWithMarshalFunc[TestModel](
		context.Background(),
		db,
		zipPath,
		func(v TestModel) ([]byte, error) {
			return json.Marshal(v)
		},
		WithExportPassword(password),
	)
	require.NoError(t, err)

	// 验证加密文件存在且有.enc后缀
	encryptedPath := zipPath + ".enc"
	_, err = os.Stat(encryptedPath)
	require.NoError(t, err)

	// 测试解密导入
	err = ImportTableZipWithMarshalFunc[TestModel](
		context.Background(),
		db.Where("1=0"), // 空查询，我们只想测试解析
		encryptedPath,
		func(b []byte) (*TestModel, error) {
			var model TestModel
			err := json.Unmarshal(b, &model)
			return &model, err
		},
		WithImportPassword(password),
		WithImportErrorHandler(func(err error) error {
			// 忽略创建错误，我们只关心解析是否成功
			return nil
		}),
	)
	require.NoError(t, err)
}
