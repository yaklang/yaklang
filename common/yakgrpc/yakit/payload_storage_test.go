package yakit

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func newPayloadStorageTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.Payload{}, &schema.GeneralStorage{}).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

func writePayloadStorageTestFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "payload.txt")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func requirePayloadGroupMode(t *testing.T, db *gorm.DB, group string, expected PayloadGroupStorageMode) {
	t.Helper()

	actual, err := InspectPayloadGroupStorage(db, group)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestSavePayloadByFilenameStoresDatabasePayloads(t *testing.T) {
	db := newPayloadStorageTestDB(t)
	path := writePayloadStorageTestFile(t, "alpha\nbeta\n")

	require.NoError(t, SavePayloadByFilename(db, "database-import", path))
	requirePayloadGroupMode(t, db, "database-import", PayloadGroupStorageDatabase)

	payloads, err := GetPayloadsByGroup(db, "database-import")
	require.NoError(t, err)
	require.Len(t, payloads, 2)

	contents := make([]string, 0, len(payloads))
	for _, payload := range payloads {
		require.False(t, payload.GetIsFile())
		contents = append(contents, payload.GetContent())
	}
	sort.Strings(contents)
	require.Equal(t, []string{"alpha", "beta"}, contents)

	_, err = GetPayloadGroupFileName(db, "database-import")
	require.ErrorContains(t, err, "not file-backed")
}

func TestSavePayloadByFilenameRepairsLegacyFileFlags(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		lines   []string
	}{
		{
			name:    "single line",
			content: "alpha\n",
			lines:   []string{"alpha"},
		},
		{
			name:    "multiple lines",
			content: "alpha\nbeta\n",
			lines:   []string{"alpha", "beta"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newPayloadStorageTestDB(t)
			group := "legacy-" + tc.name
			for _, line := range tc.lines {
				require.NoError(t, CreateOrUpdatePayload(db, strconv.Quote(line), group, "", 0, true))
			}
			requirePayloadGroupMode(t, db, group, PayloadGroupStorageLegacyFileFlag)

			path := writePayloadStorageTestFile(t, tc.content)
			require.NoError(t, SavePayloadByFilename(db, group, path))
			requirePayloadGroupMode(t, db, group, PayloadGroupStorageDatabase)

			payloads, err := GetPayloadsByGroup(db, group)
			require.NoError(t, err)
			require.Len(t, payloads, len(tc.lines))
			for _, payload := range payloads {
				require.False(t, payload.GetIsFile())
				require.Equal(t, payload.CalcHash(), payload.Hash)
			}
		})
	}
}

func TestSavePayloadByFilenameRollsBackLegacyRepairOnReadError(t *testing.T) {
	db := newPayloadStorageTestDB(t)
	group := "legacy-rollback"
	require.NoError(t, CreateOrUpdatePayload(db, strconv.Quote("payload"), group, "", 0, true))
	requirePayloadGroupMode(t, db, group, PayloadGroupStorageLegacyFileFlag)

	err := SavePayloadByFilename(db, group, filepath.Join(t.TempDir(), "missing.txt"))
	require.Error(t, err)
	requirePayloadGroupMode(t, db, group, PayloadGroupStorageLegacyFileFlag)
}

func TestSavePayloadByFilenameRejectsFileBackedGroup(t *testing.T) {
	db := newPayloadStorageTestDB(t)
	backingFile := writePayloadStorageTestFile(t, "existing\n")
	importFile := writePayloadStorageTestFile(t, "new\n")
	group := "file-backed"

	require.NoError(t, CreateOrUpdatePayload(db, backingFile, group, "", 0, true))
	requirePayloadGroupMode(t, db, group, PayloadGroupStorageFile)

	err := SavePayloadByFilename(db, group, importFile)
	require.ErrorContains(t, err, "is file-backed")
	requirePayloadGroupMode(t, db, group, PayloadGroupStorageFile)

	payloads, queryErr := GetPayloadsByGroup(db, group)
	require.NoError(t, queryErr)
	require.Len(t, payloads, 1)
	require.Equal(t, backingFile, *payloads[0].Content)
}

func TestRepairLegacyPayloadFileFlagsPatch(t *testing.T) {
	db := newPayloadStorageTestDB(t)

	require.NoError(t, CreateOrUpdatePayload(db, strconv.Quote("single"), "legacy-single", "", 0, true))
	require.NoError(t, CreateOrUpdatePayload(db, strconv.Quote("first"), "legacy-multiple", "", 0, true))
	require.NoError(t, CreateOrUpdatePayload(db, strconv.Quote("second"), "legacy-multiple", "", 0, true))

	backingFile := writePayloadStorageTestFile(t, "real file payload\n")
	require.NoError(t, CreateOrUpdatePayload(db, backingFile, "real-file", "", 0, true))

	require.NoError(t, CreateOrUpdatePayload(db, backingFile, "inconsistent", "", 0, true))
	require.NoError(t, CreateOrUpdatePayload(db, strconv.Quote("database row"), "inconsistent", "", 0, false))

	repairLegacyPayloadFileFlagsPatch(db)

	require.Equal(t, "done", GetKey(db, legacyPayloadFileFlagRepairKey))
	requirePayloadGroupMode(t, db, "legacy-single", PayloadGroupStorageDatabase)
	requirePayloadGroupMode(t, db, "legacy-multiple", PayloadGroupStorageDatabase)
	requirePayloadGroupMode(t, db, "real-file", PayloadGroupStorageFile)
	requirePayloadGroupMode(t, db, "inconsistent", PayloadGroupStorageInconsistent)

	for _, group := range []string{"legacy-single", "legacy-multiple"} {
		payloads, err := GetPayloadsByGroup(db, group)
		require.NoError(t, err)
		for _, payload := range payloads {
			require.Equal(t, payload.CalcHash(), payload.Hash)
		}
	}
}
