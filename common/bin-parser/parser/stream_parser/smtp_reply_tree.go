package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func parseSMTPReply(node *base.Node, process func(*base.Node) (func(bool), error)) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits == 0 || bits%8 != 0 || bits > smtpReplyMaxBytes*8 {
		return fmt.Errorf("smtp-reply: explicit 1..524288 byte reply boundary required")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "SMTP reply staged bytes", "raw", node.Ctx)
	if err != nil {
		return err
	}
	raw.Cfg.SetItem(CfgParent, node)
	raw.Cfg.SetItem(CfgLength, bits)
	finish, err := process(raw)
	committed := false
	if finish != nil {
		defer func() { finish(!committed) }()
	}
	if err != nil {
		return err
	}
	value, err := getNodeResult(raw, true)
	if err != nil {
		return err
	}
	wire, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("smtp-reply: staged value is not raw")
	}
	lines, info, err := decodeSMTPReply(wire)
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	list := &base.Node{Name: "Reply Lines", Origin: yaml.MapSlice{}, Cfg: base.NewConfig(staged.Cfg), Ctx: node.Ctx}
	list.Cfg.SetItem(CfgParent, staged)
	list.Cfg.SetItem(CfgLength, bits)
	list.Cfg.SetItem(CfgIsList, true)
	for index, line := range lines {
		entry := &base.Node{Name: "Reply Line", Origin: yaml.MapSlice{}, Cfg: base.NewConfig(list.Cfg), Ctx: node.Ctx}
		entry.Cfg.SetItem(CfgParent, list)
		entry.Cfg.SetItem(CfgLength, uint64(line.End-line.Start)*8)
		entry.Cfg.SetItem(CfgIsList, false)
		entry.Cfg.SetItem(CfgElementIndex, index)
		fields := []struct {
			name, typ string
			from, to  int
		}{{"Code", "string", line.Start, line.Start + 3}}
		if line.Separator != 0 {
			fields = append(fields, struct {
				name, typ string
				from, to  int
			}{"Separator", "uint8", line.Start + 3, line.Start + 4})
		}
		fields = append(fields, struct {
			name, typ string
			from, to  int
		}{"Message", "string", line.TextStart, line.TextEnd}, struct {
			name, typ string
			from, to  int
		}{"CRLF", "raw", line.End - 2, line.End})
		for _, f := range fields {
			child, err := base.NewNodeTreeWithConfig(entry.Cfg, f.name, f.typ, node.Ctx)
			if err != nil {
				return err
			}
			child.Cfg.SetItem(CfgParent, entry)
			child.Cfg.SetItem(CfgLength, uint64(f.to-f.from)*8)
			child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(f.from)*8, start + uint64(f.to)*8})
			entry.Children = append(entry.Children, child)
		}
		list.Children = append(list.Children, entry)
	}
	staged.Children = []*base.Node{list}
	if err = InitNode(staged); err != nil {
		return err
	}
	list.Cfg.SetItem(CfgParent, node)
	node.Children = staged.Children
	node.Cfg.SetItem("additionInfo", info)
	committed = true
	return nil
}
