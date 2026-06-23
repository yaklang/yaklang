package yakurl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAPISpecFileName(t *testing.T) {
	require.Equal(t, "swagger-demo.json", openAPISpecFileName("swagger-demo.json", `{}`))
	require.Equal(t, "spec.yaml", openAPISpecFileName("", "openapi: 3.0.0\ninfo:\n  title: demo"))
	require.Equal(t, "spec.json", openAPISpecFileName("", `{"swagger":"2.0"}`))
}

func TestOpenAPIDocumentDiskRoundTrip(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	ResetOpenAPIDocumentStoreForTest()

	content := `{"swagger":"2.0","info":{"title":"Disk Demo","version":"1.0.0"},"paths":{}}`
	docID := "11111111-1111-4111-8111-111111111111"
	now := time.Now().Unix()
	doc := &cachedOpenAPIDocument{
		Content: content,
		Session: newOpenAPIDocumentSession(docID, "Disk Demo", "disk-demo.json", "disk-demo.json", now),
	}
	require.NoError(t, storeOpenAPIDocument(docID, doc))

	docDir := filepath.Join(openAPIDocumentBaseDir(), docID)
	require.FileExists(t, filepath.Join(docDir, "session.json"))
	require.FileExists(t, filepath.Join(docDir, "disk-demo.json"))

	ResetOpenAPIDocumentStoreForTest()
	loaded, err := loadOpenAPIDocumentFromDisk(docID)
	require.NoError(t, err)
	require.Equal(t, content, loaded.Content)
	require.Equal(t, "Disk Demo", loaded.Parsed.Info.Title)
	require.Equal(t, "Disk Demo", loaded.Session.Title)
	require.Equal(t, "disk-demo.json", loaded.Session.FileName)
	require.Equal(t, openAPIDocumentSource, loaded.Session.Source)

	require.NoError(t, removeOpenAPIDocument(docID))
	_, err = os.Stat(docDir)
	require.True(t, os.IsNotExist(err))
}

func TestOpenAPIDocumentLegacyMetaMigration(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	ResetOpenAPIDocumentStoreForTest()

	docID := "22222222-2222-4222-8222-222222222222"
	docDir := filepath.Join(openAPIDocumentBaseDir(), docID)
	require.NoError(t, os.MkdirAll(docDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(docDir, "meta.json"), []byte(`{"uploaded_at":1710000000,"file_name":"legacy.json","spec_file":"legacy.json"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(docDir, "legacy.json"), []byte(`{"swagger":"2.0","info":{"title":"Legacy Demo","version":"1.0.0"},"paths":{}}`), 0o644))

	loaded, err := loadOpenAPIDocumentFromDisk(docID)
	require.NoError(t, err)
	require.Equal(t, int64(1710000000), loaded.Session.CreatedAt)
	require.Equal(t, "legacy.json", loaded.Session.FileName)
	require.Equal(t, "Legacy Demo", loaded.Session.Title)
}
