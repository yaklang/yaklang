package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/binx"
	"github.com/yaklang/yaklang/common/utils"
	"path"
	"reflect"
	"strconv"
	"strings"
)

const (
	CfgIsTerminal = "isTerminal"
	CfgIsList     = "isList"
	CfgLength     = "length"
	CfgType       = "type"
	CfgGetResult  = "get result"
	CfgRawResult  = "raw result"
	CfgRootMap    = "rootNodeMap"
	CfgEndian     = "endian"
	CfgOperator   = "operator"
	CfgInList     = "inList"
	CfgParent     = "parent"
	CfgDel        = "del"
	CfgDelimiter  = "delimiter"
	CfgImport     = "import"
	CfgNodeResult = "node result"
	CfgLastNode   = "last node"
)

var baseType = []string{"int", "uint", "int8", "uint8", "int16", "uint16", "int32", "uint32", "int64", "uint64", "string", "bool", "raw"}

type DefParser struct {
	base.BaseParser
	write func([]byte, uint64) ([2]uint64, error)
	ctx   *base.NodeContext
}
type NodeResult struct {
	Pos  [2]uint64
	Sub  []*NodeResult
	Node *base.Node
}

func (n *NodeResult) Result() (any, error) {
	node := n.Node
	var endian binx.ByteOrderEnum
	iendian := node.Cfg.GetItem(CfgEndian)
	if iendian == nil {
		endian = binx.BigEndianByteOrder
	}
	switch utils.InterfaceToString(iendian) {
	case "big":
		endian = binx.BigEndianByteOrder
	case "little":
		endian = binx.LittleEndianByteOrder
	default:
		return nil, fmt.Errorf("endian type error: %v", iendian)
	}
	resPoint := n.Pos
	buffer := node.Ctx.GetItem("buffer").(*bytes.Buffer)
	byts := buffer.Bytes()
	writer := node.Ctx.GetItem("writer").(*base.BitWriter)
	if writer.PreIsByte {
		byts = append(byts, writer.PreByte<<(8-writer.PreByteLen))
	}
	reader := base.NewBitReader(bytes.NewBuffer(byts))
	reader.ReadBits(resPoint[0])
	buf, err := reader.ReadBits(resPoint[1] - resPoint[0])
	if err != nil {
		return nil, fmt.Errorf("read bits error: %w", err)
	}
	res := binx.NewResult(buf)
	res.Identifier = node.Name
	res.ByteOrder = endian
	res.Type = binx.BinaryTypeVerbose(node.Cfg.GetString(CfgType))
	return res.Value(), nil
}
func getSubData(d any, key string) (any, bool) {
	refV := reflect.ValueOf(d)
	if refV.Kind() == reflect.Map {
		v := refV.MapIndex(reflect.ValueOf(key))
		if v.IsValid() {
			return v.Interface(), true
		}
	}
	return nil, false
}
func InitNode(node *base.Node) error {
	var walkNode func(node *base.Node) error
	walkNode = func(node *base.Node) error {
		if _, ok := node.Origin.(string); ok {
			node.Cfg.SetItem(CfgIsTerminal, true)
			nodeData := utils.InterfaceToString(node.Origin)
			options := strings.Split(nodeData, ";")
			for _, option := range options {
				kvs := strings.Split(option, ":")
				if len(kvs) == 1 {
					splits := strings.Split(nodeData, ",")
					var typeName string
					if len(splits) == 0 {
						return errors.New("not set type")
					} else if len(splits) == 1 {
						typeName = splits[0]
					} else if len(splits) == 2 {
						typeName = splits[0]
						lstr := ""
						isBit := false
						if strings.HasSuffix(splits[1], "bit") {
							lstr = splits[1][:len(splits[1])-3]
							isBit = true
						} else {
							lstr = splits[1]
						}
						l, err := strconv.ParseUint(lstr, 10, 64)
						if err != nil {
							return fmt.Errorf("parse length error: %w", err)
						}
						if isBit {
							node.Cfg.SetItem(CfgLength, l)
						} else {
							node.Cfg.SetItem(CfgLength, l*8)
						}
					} else {
						return errors.New("terminal node too many params")
					}
					cfgTypeName := ""
					if strings.HasSuffix(typeName, "...") {
						cfgTypeName = strings.TrimSuffix(typeName, "...")
						node.Cfg.SetItem(CfgIsList, true)
					} else {
						cfgTypeName = typeName
					}
					node.Cfg.SetItem(CfgType, cfgTypeName)

				} else if len(kvs) == 2 {
					switch kvs[0] {
					case CfgDel:
						node.Cfg.SetItem(CfgDelimiter, kvs[1])
					default:
						node.Cfg.SetItem(kvs[0], kvs[1])
					}
				} else {
					return errors.New("error option: " + option)
				}
			}
		} else {
			if len(node.Children) == 0 {
				node.Cfg.SetItem(CfgIsTerminal, true)
			}
		}
		typeName := node.Cfg.GetString(CfgType)
		if node.Cfg.GetBool(CfgIsTerminal) && typeName != "" && !utils.StringArrayContains(baseType, typeName) {
			irootNodeMap := node.Ctx.GetItem(CfgRootMap)
			if irootNodeMap == nil {
				return errors.New("not set rootNodeMap")
			}
			rootNodeMap, ok := irootNodeMap.(map[string]*base.Node)
			if !ok {
				return errors.New("rootNodeMap type error")
			}
			v, ok := rootNodeMap[typeName]
			if !ok {
				return fmt.Errorf("type `%s` not found", typeName)
			}
			err := appendNode(node, v)
			utils.GetLastElement[*base.Node](node.Children).Cfg.SetItem(CfgParent, node)
			if err != nil {
				return err
			}
		}
		for i, child := range node.Children {
			err := walkNode(child)
			if err != nil {
				return err
			}
			if i == len(node.Children)-1 {
				child.Cfg.SetItem(CfgLastNode, true)
			}
			child.Cfg.SetItem(CfgParent, node)
		}

		return nil
	}
	return walkNode(node)
}

// OnRoot 设置了Ctx: root、rootNodeMap; Cfg：parent、lastNode,writer,buffer、isTerminal
func (d *DefParser) OnRoot(node *base.Node) error {
	rootChildMap := make(map[string]*base.Node)
	node.Ctx.SetItem(CfgRootMap, rootChildMap)
	for _, child := range node.Children {
		rootChildMap[child.Name] = child
	}
	node.Ctx.SetItem("root", node)
	buffer := &bytes.Buffer{}
	node.Ctx.SetItem("buffer", buffer)
	node.Ctx.SetItem("writer", base.NewBitWriter(buffer))
	d.ctx = node.Ctx
	if d.ctx.Has("writer") {
		d.write = func(bytes []byte, u uint64) ([2]uint64, error) {
			writer := d.ctx.GetItem("writer").(*base.BitWriter)
			err := writer.WriteBits(bytes, u)
			if err != nil {
				return [2]uint64{}, err
			}
			start := d.ctx.GetUint64("pointer")
			d.ctx.SetItem("pointer", start+u)
			return [2]uint64{start, d.ctx.GetUint64("pointer")}, nil
		}
	}
	err := InitNode(node)
	if err != nil {
		return err
	}
	return nil
}
func (d *DefParser) Generate(data any, node *base.Node) error {
	return nil
}
func (d *DefParser) GenerateTerminal(data any, node *base.Node) error {
	return nil
}
func (d *DefParser) Parse(data *base.BitReader, node *base.Node) error {
	if node.Name == "Destination" {
		print()
	}
	nodeResult := &NodeResult{Node: node}
	node.Cfg.SetItem(CfgNodeResult, nodeResult)
	if node.Name == "root" {
		irootNodeMap := node.Ctx.GetItem(CfgRootMap)
		if irootNodeMap == nil {
			return errors.New("not set rootNodeMap")
		}
		rootNodeMap, ok := irootNodeMap.(map[string]*base.Node)
		if !ok {
			return errors.New("rootNodeMap type error")
		}
		if v, ok := rootNodeMap["Package"]; !ok {
			return errors.New("package node not found")
		} else {
			return v.Parse(data)
		}
	}
	if node.Cfg.Has(CfgImport) {
		ruleName := node.Cfg.GetString(CfgImport)
		rulePath := path.Join(node.Ctx.GetString("path"), ruleName)
		targetNode, err := ParseRule(rulePath)
		if err != nil {
			return err
		}
		err = d.OnRoot(targetNode)
		if err != nil {
			return fmt.Errorf("on root error: %w", err)
		}
		rootNode := getNodeByPath(targetNode, node.Cfg.GetString("node"))
		if rootNode == nil {
			return fmt.Errorf("not found node %s from rule: %s ", node.Cfg.GetString("node"), ruleName)
		}
		rootNode, err = base.NewChildNodeTree(node, node.Name, rootNode.Origin, rootNode.Ctx)
		if err != nil {
			return err
		}

		// 补充runtime cfg
		rootNode.Cfg.SetItem(CfgParent, node.Cfg.GetItem(CfgParent))

		*node = *rootNode
		InitNode(node)
		return node.Parse(data)
	}
	if v := node.Cfg.GetItem(CfgOperator); v != nil {
		//err := ExecOperator(data, nil, node, utils.InterfaceToString(v), "parse")
		err := ExecOperator(node, utils.InterfaceToString(v), func(node *base.Node) error {
			err := node.Parse(data)
			if err != nil {
				return err
			}
			iparent := node.Cfg.GetItem(CfgParent)
			if iparent != nil {
				parent := iparent.(*base.Node)
				nodeRes := parent.Cfg.GetItem(CfgNodeResult).(*NodeResult)
				nodeRes.Sub = append(nodeRes.Sub, node.Cfg.GetItem(CfgNodeResult).(*NodeResult))
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("eval operator error: %w", err)
		}
		return nil
	}
	if node.Cfg.GetBool(CfgIsTerminal) {
		isDelmiter := node.Cfg.Has(CfgDelimiter)
		if !isDelmiter {
			isList := node.Cfg.GetBool(CfgIsList)
			if isList {
				node.Ctx.SetItem(CfgInList, true)
				res := node.Cfg.GetItem(CfgNodeResult).(*NodeResult)
				element := node.Children[0]
				var total uint64
				for {
					if !node.Ctx.GetBool(CfgInList) {
						break
					}
					err := d.Parse(data, element)
					if err != nil {
						return fmt.Errorf("parse list error: %w", err)
					}
					sub := element.Cfg.GetItem(CfgNodeResult).(*NodeResult)
					res.Sub = append(res.Sub, sub)
					total += (sub.Pos[1] - sub.Pos[0])
				}
				node.Ctx.DeleteItem(CfgInList)
				return nil
			} else {
				err := d.ParseTerminal(data, node)
				if err != nil {
					return err
				}
				return nil
			}
		} else {
			delimiter := utils.InterfaceToString(node.Cfg.GetItem(CfgDelimiter))
			if len(delimiter) == 0 {
				return errors.New("delimiter length must be greater than 0")
			}
			delimitern := 0
			byts := []byte{}
			// 循环读取数据，直到遇到delimiter结束
			for {
				b := make([]byte, 1)
				_, err := data.Read(b)
				if err != nil {
					return err
				}
				if delimiter[delimitern] == b[0] {
					delimitern++
				} else {
					delimitern = 0
				}
				if delimitern == len(delimiter) {
					byts = byts[:len(byts)+1-delimitern]
					break
				}
				byts = append(byts, b...)
			}
			res, err := d.write(byts, uint64(len(byts)*8))
			if err != nil {
				return err
			}
			nodeRes := node.Cfg.GetItem(CfgNodeResult).(*NodeResult)
			nodeRes.Pos = res
			return nil
		}
	} else {
		res := node.Cfg.GetItem(CfgNodeResult).(*NodeResult)
		for _, child := range node.Children {
			err := child.Parse(data)
			if err != nil {
				return fmt.Errorf("parse child node error: %w", err)
			}
			res.Sub = append(res.Sub, child.Cfg.GetItem(CfgNodeResult).(*NodeResult))
		}
		return nil
	}
}
func (d *DefParser) ParseTerminal(data *base.BitReader, node *base.Node) error {
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
		res := node.Cfg.GetItem(CfgNodeResult).(*NodeResult)
		res.Pos = rawRes
		return nil
	} else {
		irootNodeMap := node.Ctx.GetItem(CfgRootMap)
		if irootNodeMap == nil {
			return errors.New("not set rootNodeMap")
		}
		rootNodeMap, ok := irootNodeMap.(map[string]*base.Node)
		if !ok {
			return errors.New("rootNodeMap type error")
		}
		v, ok := rootNodeMap[typeName]
		if !ok {
			return fmt.Errorf("type `%s` not found", typeName)
		}
		return v.Parse(data)
	}
}
