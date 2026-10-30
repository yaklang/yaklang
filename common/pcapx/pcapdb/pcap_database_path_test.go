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
