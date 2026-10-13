package consts

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestSSAProjectDatabasePublishedWithProfileStylePointer(t *testing.T) {
	first := GetGormSSAProjectDataBase()
	require.NotNil(t, first)
	require.Same(t, first, GetGormSSAProjectDataBase())
	require.Same(t, first, schema.GetDefaultSSADatabase())

	SetGormSSAProjectDatabase(first)
	require.Same(t, first, GetGormSSAProjectDataBase())
	require.Same(t, first, schema.GetDefaultSSADatabase())
}
