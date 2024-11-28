package bizhelper

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestExportAndImport(t *testing.T) {
	runtimeID := ksuid.New().String()
	db := consts.GetGormProjectDatabase().Model(&schema.HTTPFlow{})
	times := 5
	flows := make([]*schema.HTTPFlow, 0, times)
	for i := 0; i < times; i++ {
		flow := &schema.HTTPFlow{
			Url:       "http://example.com",
			Request:   ksuid.New().String(),
			RuntimeId: runtimeID,
		}
		resultDB := db.Save(&flow)
		require.NoError(t, resultDB.Error)
		flows = append(flows, flow)
	}
	t.Cleanup(func() {
		db.Unscoped().Where("runtime_id = ?", runtimeID).Delete(&schema.HTTPFlow{})
	})
	queryDB := db.Where("runtime_id = ?", runtimeID)

	t.Run("export: no encrypt", func(t *testing.T) {
		fp := filepath.Join(t.TempDir(), "test1.zip")
		err := ExportTableZip[*schema.HTTPFlow](context.Background(), queryDB, fp)
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
		err := ExportTableZip[*schema.HTTPFlow](context.Background(), queryDB, fp, WithExportPreWriteHandler(func(_ string, w []byte, _ map[string]string) (name string, new []byte) {
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
		m := map[string]string{"test": ksuid.New().String()}
		token := ksuid.New().String()
		err := ExportTableZip[*schema.HTTPFlow](context.Background(), queryDB, fp, WithExportPreWriteHandler(func(name string, w []byte, metadata map[string]string) (newName string, new []byte) {
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
				var metadata map[string]string
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
		err := ExportTableZip[*schema.HTTPFlow](context.Background(), queryDB, fp, WithExportPassword("password"))
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

	exportHelper := func(t *testing.T, newRuntimeID string, password string, metadatas ...map[string]string) string {
		t.Helper()

		fp := filepath.Join(t.TempDir(), fmt.Sprintf("%s.zip", ksuid.New().String()))
		if password != "" {
			fp = fmt.Sprintf("%s.enc", fp)
		}

		options := make([]ExportOption, 0)
		options = append(options, WithExportPreWriteHandler(func(name string, w []byte, _ map[string]string) (newName string, new []byte) {
			s, err := jsonpath.ReplaceStringWithError(string(w), "$.RuntimeId", newRuntimeID)
			require.NoError(t, err)
			s, err = jsonpath.ReplaceStringWithError(s, "$.Request", ksuid.New().String())
			require.NoError(t, err)
			return name, utils.InterfaceToBytes(s)
		}))
		if password != "" {
			options = append(options, WithExportPassword(password))
		}
		if len(metadatas) > 0 {
			options = append(options, WithExportMetadata(metadatas[0]))
		}
		err := ExportTableZip[*schema.HTTPFlow](context.Background(), queryDB, fp, options...)
		require.NoError(t, err)
		require.FileExists(t, fp)
		return fp
	}

	t.Run("import: no encrypt", func(t *testing.T) {
		runtimeID := ksuid.New().String()
		fp := exportHelper(t, runtimeID, "")

		err := ImportTableZip[schema.HTTPFlow](context.Background(), db, fp)
		t.Cleanup(func() {
			db.Unscoped().Where("runtime_id = ?", runtimeID).Delete(&schema.HTTPFlow{})
		})
		require.NoError(t, err)

		var flows []*schema.HTTPFlow
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
		m := map[string]string{"test": ksuid.New().String()}
		fp := exportHelper(t, runtimeID, "", m)

		err := ImportTableZip[schema.HTTPFlow](context.Background(), db, fp, WithImportPreReadHandler(func(name string, b []byte, metadata map[string]string) (new []byte, err error) {
			if name != MetaJSONFileName {
				require.Equalf(t, m, metadata, "metadata not match")
			}
			return b, nil
		}))
		t.Cleanup(func() {
			db.Unscoped().Where("runtime_id = ?", runtimeID).Delete(&schema.HTTPFlow{})
		})
		require.NoError(t, err)

		var flows []*schema.HTTPFlow
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
		err := ImportTableZip[schema.HTTPFlow](context.Background(), db, fp, WithImportPassword(password))
		t.Cleanup(func() {
			db.Unscoped().Where("runtime_id = ?", runtimeID).Delete(&schema.HTTPFlow{})
		})
		require.NoError(t, err)

		var flows []*schema.HTTPFlow
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
