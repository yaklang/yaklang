package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestCallNonVirtualMarshalRoundTrip(t *testing.T) {
	original := &Call{IsNonVirtual: true}
	record := &ssadb.IrCode{}
	record.SetExtraInfo(marshalExtraInformation(original))

	reloaded := &Call{}
	unmarshalExtraInformation(nil, reloaded, record)
	require.True(t, reloaded.IsNonVirtual)
}
