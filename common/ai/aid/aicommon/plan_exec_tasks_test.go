package aicommon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldExposePlanExecTaskRecord(t *testing.T) {
	assert.True(t, ShouldExposePlanExecTaskRecord(`{"phase":"NotCompleted"}`))
	assert.True(t, ShouldExposePlanExecTaskRecord(`{"phase":"executing"}`))
	assert.True(t, ShouldExposePlanExecTaskRecord(""))
	assert.False(t, ShouldExposePlanExecTaskRecord(`{"phase":"plan_pending_approval","react_task_id":"t1"}`))
}
