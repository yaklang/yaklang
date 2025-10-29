package ssadb_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestDeleteResult(t *testing.T) {
	code := `
a = 1
b = a + 2
c = b + 3
`
	// Parse and create program with proper name and language
	progName := uuid.NewString()
	prog, err := ssaapi.Parse(code,
		ssaapi.WithProgramName(progName),
		ssaapi.WithLanguage(ssaconfig.Yak),
	)
	require.NoError(t, err)
	defer ssadb.DeleteProgram(ssadb.GetDB(), prog.GetProgramName())

	// Run syntaxflow analysis
	flowResult, err := prog.SyntaxFlowWithError("c as $c; c #-> as $top")
	require.NoError(t, err)
	flowResult.Show()

	// Save the result
	id, err := flowResult.Save(schema.SFResultKindDebug)
	require.NoError(t, err)
	require.NotZero(t, id)

	// Verify records exist before deletion
	var nodeCount, edgeCount int
	db := ssadb.GetDB()
	db.Model(&ssadb.AuditNode{}).Where("result_id = ?", id).Count(&nodeCount)
	require.Greater(t, nodeCount, 0, "should have audit nodes")

	db.Model(&ssadb.AuditEdge{}).Where("result_id = ?", id).Count(&edgeCount)
	require.Greater(t, edgeCount, 0, "should have audit edges")

	// Delete the result
	_, err = ssadb.DeleteResultByID(id)
	require.NoError(t, err)

	// Verify all records are deleted
	var auditResult ssadb.AuditResult
	err = db.First(&auditResult, id).Error
	require.Error(t, err, "result should be deleted")

	db.Model(&ssadb.AuditNode{}).Where("result_id = ?", id).Count(&nodeCount)
	require.Equal(t, 0, nodeCount, "all nodes should be deleted")

	db.Model(&ssadb.AuditEdge{}).Where("result_id = ?", id).Count(&edgeCount)
	require.Equal(t, 0, edgeCount, "all edges should be deleted")
}
