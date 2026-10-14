package ssaapi

import (
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestResultSavePersistsPrebuiltRisksWithoutRuntime(t *testing.T) {
	name := uuid.NewString()
	prog, err := Parse(`println(123)`, WithLanguage(ssaconfig.Yak), WithProgramName(name))
	require.NoError(t, err)
	t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), name) })
	res, err := prog.SyntaxFlowWithError(`desc(title: "prebuilt risk")
println(* as $target)
alert $target`, QueryWithMemory(), QueryWithTaskID(uuid.NewString()))
	require.NoError(t, err)
	require.Len(t, res.GetRisks(), 1)
	before := res.GetRisks()[0]
	hash := before.Hash
	// The row timestamp belongs to the database insert, not to construction.
	require.True(t, before.CreatedAt.IsZero())
	var n int64
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).Where("hash = ?", hash).Count(&n).Error)
	require.Zero(t, n)

	id, err := res.Save(schema.SFResultKindScan, res.TaskID)
	require.NoError(t, err)
	var stored schema.SSARisk
	require.NoError(t, ssadb.GetDB().Where("hash = ?", hash).First(&stored).Error)
	require.False(t, stored.CreatedAt.IsZero())
	require.Greater(t, res.GetGRPCModelRisk()[0].CreatedAt, int64(0))
	require.Equal(t, uint64(id), stored.ResultID)
	require.Equal(t, res.GetResultUUID(), stored.ResultUUID)
	require.Equal(t, hash, res.GetRiskHash("target", 0))
	require.Equal(t, uint64(id), res.GetGRPCModelRisk()[0].ResultID)
	loaded, err := LoadResultByID(id, true)
	require.NoError(t, err)
	require.Equal(t, res.GetResultUUID(), loaded.GetResultUUID())

	// A second direct save still binds the current numeric result id, without
	// creating a second risk row or depending on a collector's cover decision.
	id, err = res.Save(schema.SFResultKindScan, res.TaskID)
	require.NoError(t, err)
	require.NoError(t, ssadb.GetDB().Where("hash = ?", hash).First(&stored).Error)
	require.Equal(t, uint64(id), stored.ResultID)
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).Where("hash = ?", hash).Count(&n).Error)
	require.Equal(t, int64(1), n)
}

func TestRiskDBSaverConcurrentCoversKeepOneRow(t *testing.T) {
	runtimeID := uuid.NewString()
	db := ssadb.GetDB()
	t.Cleanup(func() { db.Unscoped().Where("runtime_id = ?", runtimeID).Delete(&schema.SSARisk{}) })
	rt := NewScanRuntime()
	saver := NewRiskDBSaver()
	rt.RegisterRiskUpdateHandler(saver)
	t.Cleanup(func() { _ = rt.Close() })

	const count = riskBatchSize + 25
	risks := make([]*schema.SSARisk, count)
	for i := range risks {
		risks[i] = batchRisk(fmt.Sprintf("source-%d", i), fmt.Sprintf("feature-%d", i), "source", "source")
		risks[i].RuntimeId = runtimeID
		risks[i].PrepareForSave()
		rt.SubmitRisk(risks[i])
	}
	require.NoError(t, rt.Flush())
	var first schema.SSARisk
	require.NoError(t, db.Where("hash = ?", risks[0].Hash).First(&first).Error)

	var wg sync.WaitGroup
	for i := range risks {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			structRisk := batchRisk(fmt.Sprintf("struct-%d", i), fmt.Sprintf("feature-%d", i), "struct", "struct")
			structRisk.RuntimeId = runtimeID
			structRisk.PrepareForSave()
			rt.SubmitRisk(structRisk)
			ssaRisk := batchRisk(fmt.Sprintf("ssa-%d", i), fmt.Sprintf("feature-%d", i), "ssa", "ssa")
			ssaRisk.RuntimeId = runtimeID
			ssaRisk.PrepareForSave()
			rt.SubmitRisk(ssaRisk)
		}(i)
	}
	wg.Wait()
	require.NoError(t, rt.Close())
	require.NoError(t, rt.Close(), "close is idempotent")
	require.NoError(t, rt.Flush(), "flush after close waits without blocking")
	var n int64
	require.NoError(t, db.Model(&schema.SSARisk{}).Where("runtime_id = ?", runtimeID).Count(&n).Error)
	require.Equal(t, int64(count), n)
	require.NoError(t, db.Model(&schema.SSARisk{}).Where("runtime_id = ? AND scan_mode != ?", runtimeID, "ssa").Count(&n).Error)
	require.Zero(t, n)
	var covered schema.SSARisk
	require.NoError(t, db.Where("runtime_id = ? AND risk_feature_hash = ?", runtimeID, risks[0].RiskFeatureHash).First(&covered).Error)
	require.Equal(t, "ssa", covered.ScanMode)
	require.NoError(t, db.Model(&schema.SSARisk{}).Where("hash = ?", first.Hash).Count(&n).Error)
	require.Zero(t, n, "the replaced hash does not leave its row behind")
	// The asynchronous saver never mutates objects already visible to streams.
	require.Zero(t, risks[0].ID)
}

type lifecycleRiskHandler struct {
	flushes int
	closes  int
}

func (*lifecycleRiskHandler) ApplyRiskUpdate(schema.RiskUpdateItem) error { return nil }
func (h *lifecycleRiskHandler) Flush() error                              { h.flushes++; return nil }
func (h *lifecycleRiskHandler) Close() error                              { h.closes++; return nil }

func TestScanRuntimeHandlerLifecycle(t *testing.T) {
	rt := NewScanRuntime()
	var nilHandler *lifecycleRiskHandler
	rt.RegisterRiskUpdateHandler(nilHandler)
	h := &lifecycleRiskHandler{}
	rt.RegisterRiskUpdateHandler(h)
	require.NoError(t, rt.Flush())
	require.NoError(t, rt.Close())
	require.NoError(t, rt.Close())
	require.Equal(t, 1, h.flushes)
	require.Equal(t, 1, h.closes)
}

func TestRiskSaverDropsOnlyCoveredAuditEdges(t *testing.T) {
	db := ssadb.GetDB()
	name := uuid.NewString()
	risk := batchRisk(name, name, "source", "source")
	risk.RuntimeId = name
	risk.PrepareForSave()
	require.NoError(t, SaveRiskUpdates(db, schema.RiskUpdateItem{Risk: risk}))
	node := &ssadb.AuditNode{NodeID: uuid.NewString(), AuditNodeStatus: ssadb.AuditNodeStatus{RiskHash: risk.Hash, ProgramName: name}}
	other := &ssadb.AuditNode{NodeID: uuid.NewString(), AuditNodeStatus: ssadb.AuditNodeStatus{RiskHash: "other", ProgramName: name}}
	require.NoError(t, db.Create(node).Error)
	require.NoError(t, db.Create(other).Error)
	edge := &ssadb.AuditEdge{FromNode: node.NodeID, ToNode: other.NodeID, ProgramName: name}
	require.NoError(t, db.Create(edge).Error)
	t.Cleanup(func() {
		db.Unscoped().Where("program_name = ?", name).Delete(&ssadb.AuditNode{})
		db.Unscoped().Where("program_name = ?", name).Delete(&ssadb.AuditEdge{})
		db.Unscoped().Where("runtime_id = ?", name).Delete(&schema.SSARisk{})
	})
	covered := batchRisk(uuid.NewString(), name, "ssa", "covered")
	covered.RuntimeId = name
	require.NoError(t, SaveRiskUpdates(db, schema.RiskUpdateItem{Risk: covered, OldID: risk.ID, OldHash: risk.Hash}))
	var n int64
	require.NoError(t, db.Model(&ssadb.AuditEdge{}).Where("id = ?", edge.ID).Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, db.Model(&ssadb.AuditNode{}).Where("id = ?", other.ID).Count(&n).Error)
	require.Equal(t, int64(1), n)
}
