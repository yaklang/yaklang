package stream_parser

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestPublicOperatorCallbackOrder(t *testing.T) {
	for _, failure := range []string{"", "handled", "backup", "child", "pop"} {
		n := &base.Node{Name: "Parent", Cfg: base.NewEmptyConfig()}
		n.Children = []*base.Node{{Name: "A"}, {Name: "B"}}
		var calls []string
		marker := errors.New("callback failed")
		op := &Operator{
			Mode: "custom",
			ParseStruct: func(*base.Node) (bool, error) {
				calls = append(calls, "struct")
				return failure == "handled", marker
			},
			Backup: func() error {
				calls = append(calls, "backup")
				if failure == "backup" {
					return marker
				}
				return nil
			},
			NodeParse: func(n *base.Node) error {
				calls = append(calls, n.Name)
				if failure == "child" && n.Name == "A" {
					return marker
				}
				return nil
			},
			Recovery: func() error { calls = append(calls, "recovery"); return nil },
			PopBackup: func() error {
				calls = append(calls, "pop")
				if failure == "pop" {
					return marker
				}
				return nil
			},
		}
		err := (&DefParser{}).Operate(op, n)
		expected := []string{"struct", "backup", "A", "B", "pop"}
		switch failure {
		case "handled":
			expected = []string{"struct"}
		case "backup":
			expected = []string{"struct", "backup"}
		case "child":
			expected = []string{"struct", "backup", "A", "recovery"}
		}
		require.Equal(t, expected, calls)
		if failure == "" {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, marker)
		}
	}
}

func TestParseExecutionRetainedCallbacksKeepReaderAndOrigin(t *testing.T) {
	root := giopBridgeInlineRoot(t, "Package:\n  A: uint8\n")
	d := &DefParser{}
	require.NoError(t, d.OnRoot(root))
	first := &parseExecution{parser: d, reader: base.NewBitReader(bytes.NewReader([]byte{0x11, 0x12})), origin: root}
	save, restore := first.backup, first.recovery
	otherOrigin := positionedResultNode([]byte{0x99, 0x98, 0x97}, 0)
	second := &parseExecution{parser: d, reader: base.NewBitReader(bytes.NewReader([]byte{0x22, 0x23})), origin: otherOrigin}
	require.NoError(t, second.backup())
	b, err := second.reader.ReadByte()
	require.NoError(t, err)
	_, err = d.write([]byte{b}, 8)
	require.NoError(t, err)
	require.NoError(t, second.popBackup())
	require.NoError(t, save())
	b, err = first.reader.ReadByte()
	require.NoError(t, err)
	_, err = d.write([]byte{b}, 8)
	require.NoError(t, err)
	require.NoError(t, restore())
	require.Equal(t, []byte{0x22}, root.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes())
	require.Equal(t, []byte{0x99, 0x98, 0x97}, otherOrigin.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes())
	b, err = first.reader.ReadByte()
	require.NoError(t, err)
	require.Equal(t, byte(0x11), b)
	b, err = second.reader.ReadByte()
	require.NoError(t, err)
	require.Equal(t, byte(0x23), b)
}
