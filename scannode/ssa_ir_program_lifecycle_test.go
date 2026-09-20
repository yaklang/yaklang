package scannode

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestHandleSSAIRProgramDeleteWithPublishesBusinessRejection(t *testing.T) {
	var published ssaIRProgramDeleteResponse
	err := handleSSAIRProgramDeleteWith(
		context.Background(),
		[]byte(`{"request_id":"request-1","store_identity":"ir-store-v1:test","program_name":"demo-old"}`),
		func(_ context.Context, storeIdentity, programName string) (bool, bool, string, error) {
			require.Equal(t, "ir-store-v1:test", storeIdentity)
			require.Equal(t, "demo-old", programName)
			return false, false, "", errors.New("该 IR 程序仍是当前增量编译基线，未删除")
		},
		func(_ context.Context, requestID string, response ssaIRProgramDeleteResponse) error {
			require.Equal(t, "request-1", requestID)
			published = response
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, "request-1", published.RequestID)
	require.Equal(t, "ir-store-v1:test", published.StoreIdentity)
	require.Equal(t, "demo-old", published.ProgramName)
	require.False(t, published.Success)
	require.False(t, published.Deleted)
	require.Contains(t, published.Reason, "当前增量编译基线")
}

func TestDeleteSSAIRProgramForLifecycleKeepsNewestAndDeletesOlder(t *testing.T) {
	oldDB := ssadb.GetDB()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "ssa-ir.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(ssadb.SSAProjectTables...).Error)
	ssadb.SetDB(db)
	t.Cleanup(func() {
		ssadb.SetDB(oldDB)
		require.NoError(t, db.Close())
	})

	older := ssadb.IrProgram{ProgramName: "demo(2026-09-19 10:00:00)"}
	newer := ssadb.IrProgram{ProgramName: "demo(2026-09-20 10:00:00)"}
	require.NoError(t, db.Create(&older).Error)
	require.NoError(t, db.Create(&newer).Error)
	require.NoError(t, db.Model(&older).UpdateColumn("updated_at", time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC)).Error)
	require.NoError(t, db.Model(&newer).UpdateColumn("updated_at", time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)).Error)
	storeIdentity, err := ssaIRStoreIdentity(db)
	require.NoError(t, err)

	deleted, absent, _, err := deleteSSAIRProgramForLifecycle(context.Background(), storeIdentity, newer.ProgramName)
	require.ErrorContains(t, err, "当前增量编译基线")
	require.False(t, deleted)
	require.False(t, absent)

	deleted, absent, _, err = deleteSSAIRProgramForLifecycle(context.Background(), storeIdentity, older.ProgramName)
	require.NoError(t, err)
	require.True(t, deleted)
	require.False(t, absent)

	var olderCount, newerCount int
	require.NoError(t, db.Model(&ssadb.IrProgram{}).Where("program_name = ?", older.ProgramName).Count(&olderCount).Error)
	require.NoError(t, db.Model(&ssadb.IrProgram{}).Where("program_name = ?", newer.ProgramName).Count(&newerCount).Error)
	require.Zero(t, olderCount)
	require.Equal(t, 1, newerCount)
}

func TestDeleteSSAIRProgramForLifecycleRejectsWrongStoreBeforeLookup(t *testing.T) {
	oldDB := ssadb.GetDB()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "ssa-ir.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(ssadb.SSAProjectTables...).Error)
	ssadb.SetDB(db)
	t.Cleanup(func() {
		ssadb.SetDB(oldDB)
		require.NoError(t, db.Close())
	})

	program := ssadb.IrProgram{ProgramName: "demo(2026-09-19 10:00:00)"}
	require.NoError(t, db.Create(&program).Error)

	deleted, absent, _, err := deleteSSAIRProgramForLifecycle(
		context.Background(),
		buildSSAIRStoreIdentity("sqlite", "/wrong/store.db"),
		program.ProgramName,
	)
	require.ErrorContains(t, err, "IR 存储身份不匹配")
	require.False(t, deleted)
	require.False(t, absent)

	var remaining int
	require.NoError(t, db.Model(&ssadb.IrProgram{}).Where("program_name = ?", program.ProgramName).Count(&remaining).Error)
	require.Equal(t, 1, remaining)
}

func TestDeleteSSAIRProgramForLifecycleAlreadyAbsentIsIdempotent(t *testing.T) {
	oldDB := ssadb.GetDB()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "ssa-ir.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(ssadb.SSAProjectTables...).Error)
	ssadb.SetDB(db)
	t.Cleanup(func() {
		ssadb.SetDB(oldDB)
		require.NoError(t, db.Close())
	})
	storeIdentity, err := ssaIRStoreIdentity(db)
	require.NoError(t, err)

	deleted, absent, reason, err := deleteSSAIRProgramForLifecycle(
		context.Background(),
		storeIdentity,
		"missing-program",
	)
	require.NoError(t, err)
	require.False(t, deleted)
	require.True(t, absent)
	require.Contains(t, reason, "已不存在")
}

func TestBuildSSAIRStoreIdentityProtocolFixture(t *testing.T) {
	require.Equal(
		t,
		"ir-store-v1:99ee4529de7d535032633b46c96f699105626112df20b15a3d6516e6b8983fd1",
		buildSSAIRStoreIdentity("postgres", "7670784354635026475", "ssa_ir"),
	)
}

func TestCommandConsumerLimitsSSAIRMaintenanceToOneWorker(t *testing.T) {
	consumer := &commandConsumer{
		irMaintenanceSlots: make(chan struct{}, maxConcurrentSSAIRMaintenances),
	}
	require.True(t, consumer.tryAcquireSSAIRMaintenance())
	require.False(t, consumer.tryAcquireSSAIRMaintenance())
	consumer.releaseSSAIRMaintenance()
	require.True(t, consumer.tryAcquireSSAIRMaintenance())
	consumer.releaseSSAIRMaintenance()
}
