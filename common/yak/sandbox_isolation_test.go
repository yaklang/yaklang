package yak

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSandboxIsolatedExecutionDiscardsPreviousVariables(t *testing.T) {
	box := NewSandbox(WithSandbox_IsolatedExecution())
	_, err := box.ExecuteAsExpression(`remembered = 42; event.marker`, map[string]any{"event": map[string]any{"marker": "first"}})
	require.NoError(t, err)
	value, err := box.ExecuteAsExpression(`count = 0; for key, value = range event { count++ }; count`, map[string]any{"event": map[string]any{}})
	require.NoError(t, err)
	require.EqualValues(t, 0, value)
	value, err = box.ExecuteAsExpression(`remembered`)
	require.NoError(t, err)
	require.Nil(t, value)
	_, retained := box.engine.GetVar("event")
	require.False(t, retained, "the reusable sandbox must not retain expression dependencies")
}

func TestSandboxSessionKeepsConditionStateAndIsolatesHelpers(t *testing.T) {
	box := NewSandbox(WithSandbox_ExternalLib(map[string]any{"helper": map[string]any{"value": 7}}), WithSandbox_IsolatedExecution())
	session := box.NewSession()
	_, err := session.ExecuteAsExpression(`condition_value = 42; helper.value = 99; true`)
	require.NoError(t, err)
	value, err := session.ExecuteAsExpression(`condition_value`)
	require.NoError(t, err)
	require.EqualValues(t, 42, value)
	value, err = box.NewSession().ExecuteAsExpression(`helper.value`)
	require.NoError(t, err)
	require.EqualValues(t, 7, value)
	value, err = box.NewSession().ExecuteAsExpression(`condition_value`)
	require.NoError(t, err)
	require.Nil(t, value)

	// Existing Sandbox users keep their explicitly stateful behavior.
	stateful := NewSandbox()
	_, err = stateful.ExecuteAsExpression(`remembered = 42`)
	require.NoError(t, err)
	value, err = stateful.ExecuteAsExpression(`remembered`)
	require.NoError(t, err)
	require.EqualValues(t, 42, value)
}

func TestSandboxBooleanContextStopsExpression(t *testing.T) {
	box := NewSandbox(WithSandbox_IsolatedExecution())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := box.ExecuteAsBooleanContext(ctx, `for { }; true`)
		done <- err
	}()
	select {
	case err := <-done:
		require.True(t, errors.Is(err, context.DeadlineExceeded), "got %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("sandbox expression ignored the caller deadline")
	}
	matched, err := box.ExecuteAsBoolean(`true`)
	require.NoError(t, err)
	require.True(t, matched, "cancellation poisoned the reusable sandbox")
}
