package yakit

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func TestSetKeyIfAbsent(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&schema.GeneralStorage{}).Error)
	for _, value := range []string{"", "value\nwith \"quotes\" and \\slashes"} {
		key := "key\n" + value
		require.NoError(t, SetKeyIfAbsent(db, key, value))
		require.NoError(t, SetKeyIfAbsent(db, key, "replacement"))
		require.Equal(t, value, GetKey(db, key))
		kv, err := GetKeyModel(db, key)
		require.NoError(t, err)
		require.Equal(t, strconv.Quote(key), kv.Key)
		// The existing setter must retain its overwrite behavior.
		require.NoError(t, SetKey(db, key, "replacement"))
		require.Equal(t, "replacement", GetKey(db, key))
	}
	require.Error(t, SetKeyIfAbsent(nil, "key", "value"))
	require.NoError(t, db.Close())
	require.Error(t, SetKeyIfAbsent(db, "new-key", "value"))
}
