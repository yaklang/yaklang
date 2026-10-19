package ssa

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssalog"
)

func TestExternLibMarshalKeepsEmptyExtraInfoWithoutWarning(t *testing.T) {
	var buf bytes.Buffer
	previousLevel := ssalog.Log.Level
	ssalog.Log.SetLevel("warn")
	ssalog.Log.SetOutput(&buf)
	t.Cleanup(func() {
		ssalog.Log.Level = previousLevel
		ssalog.Log.SetOutput(os.Stdout)
	})

	original := &ExternLib{anValue: NewValue(), LibraryName: "reflect"}
	original.SetName("alias")
	buf.Reset()

	first := marshalExtraInformation(original)
	second := marshalExtraInformation(original)
	require.Empty(t, first)
	require.Empty(t, second)
	require.NotContains(t, buf.String(), "marshal ExternLib")

	record := &ssadb.IrCode{}
	record.SetExtraInfo(first)
	reloaded := &ExternLib{anValue: NewValue()}
	unmarshalExtraInformation(nil, reloaded, record)
	require.Empty(t, reloaded.GetName())
	require.Empty(t, reloaded.LibraryName)
	require.Equal(t, "alias", original.GetName())
	require.Equal(t, "reflect", original.LibraryName)
}
