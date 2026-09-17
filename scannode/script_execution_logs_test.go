package scannode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScanDetailStage(t *testing.T) {
	assert.Equal(t, "compile", scanDetailStage(`{"phase":"compile","total_files":10}`))
	assert.Equal(t, "scan", scanDetailStage(`{"phase":"scan","total_rules":3}`))
	assert.Equal(t, "source-scan", scanDetailStage(`{"phase":"source-scan","total_rules":668}`))
	assert.Equal(t, "inspect", scanDetailStage(`{"phase":"inspect","total_rules":10}`))
	assert.Equal(t, "review", scanDetailStage(`{"phase":"review","total_files":12}`))
	assert.Equal(t, "analyze", scanDetailStage(`{"phase":"analyze","total_rules":3}`))
	assert.Equal(t, "collect", scanDetailStage(`{"phase":"collect","total_bytes":4096}`))
	assert.Equal(t, "rule-detail", scanDetailStage(`{"completed_rules":1}`))
	assert.Equal(t, "rule-detail", scanDetailStage(`not-json`))
}
