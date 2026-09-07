package stream_parser_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestRuleCanDistinguishDeclaredBoundsFromAnOpenStream(t *testing.T) {
	for _, bounded := range []bool{false, true} {
		t.Run(fmt.Sprintf("bounded=%t", bounded), func(t *testing.T) {
			source := fmt.Sprintf(`
Package:
  Message:
    operator: |
      if this.HasMaxLength() != %t { panic("incorrect initial bound") }
      this.ProcessSubNode("Marker")
      if this.GetSubNode("Inherited").HasMaxLength() != %t { panic("incorrect inherited bound") }
      this.GetSubNode("Empty").SetMaxLength(0)
      if !this.GetSubNode("Empty").HasMaxLength() || this.GetSubNode("Empty").GetMaxLength() != 0 { panic("empty is not unbounded") }
      this.GetSubNode("Inherited").SetMaxLength(1)
      if !this.GetSubNode("Inherited").HasMaxLength() { panic("explicit bound lost") }
      this.ProcessSubNode("Inherited")
    Marker: uint8
    Empty: raw
    Inherited:
      Byte: uint8
`, bounded, bounded)
			var document yaml.MapSlice
			require.NoError(t, yaml.Unmarshal([]byte(source), &document))
			root, err := base.NewNodeTree(document)
			require.NoError(t, err)
			if bounded {
				root.Cfg.SetItem(base.CfgLength, uint64(16))
			}
			reader := bytes.NewReader([]byte{0x11, 0x22})
			require.NoError(t, root.Parse(base.NewBitReader(reader)))
			require.Zero(t, reader.Len())
		})
	}
}
