package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// Explicit invocation state replaces the per-node Operator plus five captured
// functions. Each invocation still owns its reader, parser and rollback node;
// saved callbacks never depend on a mutable shared current-node slot.
type parseExecution struct {
	parser *DefParser
	reader *base.BitReader
	origin *base.Node
}

func (p *parseExecution) mode() string                         { return ParserMode }
func (p *parseExecution) parseStruct(*base.Node) (bool, error) { return false, nil }
func (p *parseExecution) parseNode(n *base.Node) error         { return n.Parse(p.reader) }

func (p *parseExecution) parseTerminal(node *base.Node) error {
	d, data := p.parser, p.reader
	//if node.Cfg.Has("parser") {
	//	secondValue, err := ExecParser(node)
	//	if err != nil {
	//		return fmt.Errorf("exec parser error: %w", err)
	//	}
	//	node.Cfg.SetItem("secondValue", secondValue)
	//	return nil
	//}
	if !NodeIsTerminal(node) {
		return fmt.Errorf("node %s is not terminal", node.Name)
	}
	if !NodeIsDelimiter(node) {
		itypeName := node.Cfg.GetItem(CfgType)
		if itypeName == nil {
			return errors.New("not set type")
		}
		typeName := utils.InterfaceToString(itypeName)
		if utils.StringArrayContains(baseType, typeName) {
			length, err := getNodeLength(node)
			if err != nil {
				return fmt.Errorf("get node length error: %w", err)
			}
			if length == 0 {
				return nil
			}
			switch typeName {
			case "string":
				typeName = "bytes"
			case "bytes":
				typeName = "raw"
			}
			buf, err := data.ReadBits(length)
			if err != nil {
				return fmt.Errorf("read bits error: %w", err)
			}
			rawRes, err := d.write(buf, length)
			if err != nil {
				return fmt.Errorf("write error: %w", err)
			}
			node.Cfg.SetItem(CfgNodeResult, rawRes)
			if log.GetLevel() >= log.DebugLevel {
				log.Debugf("node %s result: %v", node.Name, codec.EncodeToHex(buf))
			}
			return nil
		} else {
			return errors.New("not support type")
		}
	} else {
		delimiter := utils.InterfaceToString(node.Cfg.GetItem(CfgDelimiter))
		if len(delimiter) == 0 {
			delimiter = utils.InterfaceToString(node.Cfg.GetItem(CfgDel))
			if len(delimiter) == 0 {
				return errors.New("delimiter length must be greater than 0")
			}
		}
		available, bounded, err := parseLengthByLengthConfig(node)
		if err != nil {
			return fmt.Errorf("delimiter boundary: %w", err)
		}
		// KMP preserves overlapping delimiter prefixes (e.g. aab in aaab).
		prefix := make([]int, len(delimiter))
		for i, matched := 1, 0; i < len(delimiter); i++ {
			for matched > 0 && delimiter[i] != delimiter[matched] {
				matched = prefix[matched-1]
			}
			if delimiter[i] == delimiter[matched] {
				matched++
			}
			prefix[i] = matched
		}
		delimitern := 0
		byts := []byte{}
		writeUnterminated := func() error {
			res, writeErr := d.write(byts, uint64(len(byts)*8))
			if writeErr != nil {
				return writeErr
			}
			node.Cfg.SetItem(CfgNodeResult, res)
			node.Cfg.SetItem(CfgConsumedBits, uint64(len(byts)*8))
			return nil
		}
		for {
			if bounded && (uint64(len(byts))+1)*8 > available {
				if node.Cfg.GetBool(CfgDelimiterOptional) && uint64(len(byts))*8 == available {
					return writeUnterminated()
				}
				return fmt.Errorf("delimiter not found within field boundary: %w", io.ErrUnexpectedEOF)
			}
			b, err := data.ReadByte()
			if err != nil {
				if (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) && node.Cfg.GetBool(CfgDelimiterOptional) {
					return writeUnterminated()
				}
				return err
			}
			byts = append(byts, b)
			for delimitern > 0 && delimiter[delimitern] != b {
				delimitern = prefix[delimitern-1]
			}
			if delimiter[delimitern] == b {
				delimitern++
			}
			if delimitern == len(delimiter) {
				byts = byts[:len(byts)-delimitern]
				break
			}
		}
		res, err := d.write(byts, uint64(len(byts)*8))
		if err != nil {
			return err
		}
		node.Cfg.SetItem(CfgNodeResult, res)
		res, err = d.write([]byte(delimiter), uint64(len(delimiter)*8))
		if err != nil {
			return err
		}
		node.Cfg.SetItem(CfgConsumedBits, uint64((len(byts)+len(delimiter))*8))
		if log.GetLevel() >= log.DebugLevel {
			log.Debugf("node %s result: %v", node.Name, codec.EncodeToHex(byts))
		}
		return nil
	}
}

func (p *parseExecution) backup() error {
	d, data := p.parser, p.reader
	d.bpPos = append(d.bpPos, d.ctx.GetUint64("pointer"))
	writer := d.ctx.GetItem("writer").(*base.BitWriter)
	d.bpOut = append(d.bpOut, writer.Snapshot())
	return data.Backup()
}

func (p *parseExecution) recovery() error {
	d, data, node := p.parser, p.reader, p.origin
	if len(d.bpPos) == 0 || len(d.bpOut) == 0 {
		return errors.New("no parser backup")
	}
	position := d.bpPos[len(d.bpPos)-1]
	writerState := d.bpOut[len(d.bpOut)-1]
	d.ctx.SetItem("pointer", position)
	buffer := node.Ctx.GetItem("buffer").(*bytes.Buffer)
	buffer.Truncate(int(position) / 8)
	writer := d.ctx.GetItem("writer").(*base.BitWriter)
	if err := writer.Restore(writerState); err != nil {
		return fmt.Errorf("restore output writer: %w", err)
	}
	d.bpPos = d.bpPos[:len(d.bpPos)-1]
	d.bpOut = d.bpOut[:len(d.bpOut)-1]
	return data.Recovery()
}

func (p *parseExecution) popBackup() error {
	d, data := p.parser, p.reader
	if len(d.bpPos) == 0 || len(d.bpOut) == 0 {
		return errors.New("no parser backup")
	}
	d.bpPos = d.bpPos[:len(d.bpPos)-1]
	d.bpOut = d.bpOut[:len(d.bpOut)-1]
	return data.PopBackup()
}
