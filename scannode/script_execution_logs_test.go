package scannode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScanDetailStage(t *testing.T) {
	assert.Equal(t, "compile", scanDetailStage(`{"phase":"compile","total_files":10}`))
	assert.Equal(t, "scan", scanDetailStage(`{"phase":"scan","total_rules":3}`))
	assert.Equal(t, "rule-detail", scanDetailStage(`{"completed_rules":1}`))
	assert.Equal(t, "rule-detail", scanDetailStage(`not-json`))
}
