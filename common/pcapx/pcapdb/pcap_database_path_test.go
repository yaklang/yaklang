package pcapdb

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPCAPDBIndexPathKeepsDriveUnicodeAndURICharacters(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "capture 中 ü #%")
	require.NoError(t, os.Mkdir(directory, 0700))
	path := filepath.Join(directory, "index #%.sqlite")
	writer, err := openIndex(path, true, true)
	require.NoError(t, err)
	require.NoError(t, writer.Exec("CREATE TABLE wire_proof (value TEXT NOT NULL)").Error)
	require.NoError(t, writer.Exec("INSERT INTO wire_proof(value) VALUES (?)", "unchanged captured bytes").Error)
	require.NoError(t, writer.Close())
	reader, err := openIndex(path, false, false)
	require.NoError(t, err)
	var value string
	require.NoError(t, reader.Raw("SELECT value FROM wire_proof").Row().Scan(&value))
	require.Equal(t, "unchanged captured bytes", value)
	require.Error(t, reader.Exec("DELETE FROM wire_proof").Error, "path normalization must preserve read-only mode")
	require.NoError(t, reader.Close())
	missing := filepath.Join(directory, "missing.sqlite")
	_, err = openIndex(missing, false, false)
	require.Error(t, err)
	_, err = os.Stat(missing)
	require.True(t, os.IsNotExist(err), "read-only open must not create an index")
}

func TestPCAPDBIndexFilesystemPaths(t *testing.T) {
	// Exercise the real SQLite driver: a URL round-trip alone would not catch
	// Windows drive letters being interpreted as a URI authority.
	dir := filepath.Join(t.TempDir(), "流量库 space #100% &")
	require.NoError(t, os.MkdirAll(dir, 0700))
	path := filepath.Join(dir, "index.sqlite")
	writer, err := openIndex(path, true, true)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writer.Close() })
	require.NoError(t, writer.Exec("CREATE TABLE path_probe (value TEXT)").Error)
	require.NoError(t, writer.Exec("INSERT INTO path_probe VALUES (?)", "流量证据").Error)
	require.NoError(t, writer.Close())
	_, err = os.Stat(path)
	require.NoError(t, err, "SQLite must create the exact filesystem path")
	for _, filename := range []string{path, filepath.ToSlash(path)} {
		reader, err := openIndex(filename, false, false)
		require.NoError(t, err)
		var value string
		err = reader.Raw("SELECT value FROM path_probe").Row().Scan(&value)
		require.NoError(t, err)
		require.Equal(t, "流量证据", value)
		require.Error(t, reader.Exec("INSERT INTO path_probe VALUES ('forbidden')").Error)
		require.NoError(t, reader.Close())
	}
	// Existing-only modes must still fail closed rather than silently creating
	// an empty database when opening a missing library.
	for _, write := range []bool{false, true} {
		missing := filepath.Join(dir, "missing.sqlite")
		db, err := openIndex(missing, write, false)
		if db != nil {
			_ = db.Close()
		}
		require.Error(t, err)
		_, err = os.Stat(missing)
		require.True(t, os.IsNotExist(err))
	}
}
