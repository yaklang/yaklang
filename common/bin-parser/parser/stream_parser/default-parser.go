package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/utils"
	"path"
	"reflect"
)

const (
	CfgIsTerminal    = "isTerminal"
	CfgIsList        = "isList"
	CfgIsTempRoot    = "temp root"
	CfgLength        = "length"
	CfgType          = "type"
	CfgGetResult     = "get result"
	CfgRawResult     = "raw result"
	CfgRootMap       = "rootNodeMap"
	CfgEndian        = "endian"
	CfgOperator      = "operator"
	CfgInList        = "inList"
	CfgParent        = "parent"
	CfgDel           = "del"
	CfgDelimiter     = "delimiter"
	CfgImport        = "import"
	CfgNodeResult    = "node result"
	CfgLastNode      = "last node"
	CfgElementIndex  = "element index"
	CfgExceptionPlan = "exception-plan"
	CtxGenReaders    = "readers in generator"
)

var baseType = []string{"int", "uint", "int8", "uint8", "int16", "uint16", "int32", "uint32", "int64", "uint64", "string", "bool", "raw"}

const (
	ParserMode    = "parser"
	GeneratorMode = "geneartor"
)

type DefParser struct {
	base.BaseParser
	write func([]byte, uint64) ([2]uint64, error)
	ctx   *base.NodeContext
	mode  string
}
type Operator struct {
	ParseStruct   func(node *base.Node) (bool, error)
	ParseTerminal func(node *base.Node) error
	NodeParse     func(node *base.Node) error
	Mode          string
	Backup        func() error
	Recovery      func() error
	PopBackup     func() error
}

func InitNode(node *base.Node) error {
	var walkNode func(node *base.Node) error
	walkNode = func(node *base.Node) error {
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
			node.Cfg.SetItem("isRefType", true)
			utils.GetLastElement[*base.Node](node.Children).Cfg.SetItem(CfgParent, node)
			node.Cfg.SetItem(CfgIsTerminal, false)
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
	node.Ctx.SetItem("def_writer", d.write)
	err := InitNode(node)
	if err != nil {
		return err
	}
	return nil
}
func (d *DefParser) Operate(operator *Operator, node *base.Node) error {
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
			return operator.NodeParse(v)
		}
	}
	if node.Cfg.Has(CfgImport) {
		ruleName := node.Cfg.GetString(CfgImport)
		rulePath := path.Join(node.Ctx.GetString("path"), ruleName)
		targetNode, err := ParseRule(rulePath)
		if err != nil {
			return err
		}
		rootChildMap := make(map[string]*base.Node)
		targetNode.Ctx.SetItem(CfgRootMap, rootChildMap)
		for _, child := range targetNode.Children {
			rootChildMap[child.Name] = child
		}
		err = InitNode(targetNode)
		if err != nil {
			return fmt.Errorf("on root error: %w", err)
		}

		rootNode := getNodeByPath(targetNode, node.Cfg.GetString("node"))
		if rootNode == nil {
			return fmt.Errorf("not found node %s from rule: %s ", node.Cfg.GetString("node"), ruleName)
		}
		//rootNode, err = base.NewChildNodeTree(node, node.Name, rootNode.Origin, node.Ctx)
		//if err != nil {
		//	return err
		//}
		//*rootNode.Ctx = *node.Ctx

		rootNode.Ctx.SetItem("writer", node.Ctx.GetItem("writer"))
		rootNode.Ctx.SetItem("buffer", node.Ctx.GetItem("buffer"))
		// 补充runtime cfg
		rootNode.Cfg = base.AppendConfig(node.Cfg, rootNode.Cfg)
		rootNode.Cfg.DeleteItem(CfgImport)
		rootNode.Cfg.DeleteItem(CfgIsTerminal)
		//rootNode.Cfg.SetItem(CfgNodeResult, nodeResult)
		*node = *rootNode
		//InitNode(node)
		return operator.NodeParse(node)
	}
	if v := node.Cfg.GetItem(CfgOperator); v != nil {
		err := ExecOperator(node, utils.InterfaceToString(v), func(node *base.Node) error {
			return operator.NodeParse(node)
		})
		if err != nil {
			return fmt.Errorf("eval operator error: %w", err)
		}
		return nil
	}
	if node.Cfg.GetBool(CfgIsList) {
		if operator.ParseStruct != nil {
			ok, err := operator.ParseStruct(node)
			if ok {
				return err
			}
		}
		node.Ctx.SetItem(CfgInList, true)
		if len(node.Children) == 0 {
			return errors.New("get node element type error")
		}
		elementTemplate := node.Children[0]
		node.Children = nil
		err := func() error {
			var listLength uint64
			hasLength := false
			if node.Cfg.Has("list-length") {
				listLength = node.Cfg.GetUint64("list-length")
				hasLength = true
			}
			if node.Cfg.Has("list-length-from-field") {
				field := node.Cfg.GetString("list-length-from-field")
				fieldNode := base.GetNodeByPath(node, field)
				if fieldNode == nil {
					return fmt.Errorf("read field %s error: not found", field)
				}
				if !NodeHasResult(fieldNode) {
					return fmt.Errorf("read field %s error: not set result", field)
				}
				res := GetNodeResult(fieldNode)
				if IsNumber(res) {
					listLength = AnyToUint64(res)
				}
				hasLength = true
			}
			index := 0
			for {
				if hasLength && uint64(index) >= listLength {
					break
				}
				err := operator.Backup()
				if err != nil {
					return fmt.Errorf("backup error: %w", err)
				}
				element := elementTemplate.Copy()
				element.Cfg.SetItem(CfgElementIndex, index)
				element.Cfg.SetItem(CfgParent, node)
				node.Children = append(node.Children, element)
				//node.Cfg.GetItem("exception-plan")
				l, err := getRemainingSpace(element)
				if err != nil {
					return fmt.Errorf("get remaining space error: %w", err)
				}
				if l == 0 {
					break
				}
				//cfgDeleteItem(element, CfgNodeResult)
				if !node.Ctx.GetBool(CfgInList) {
					break
				}
				err = operator.NodeParse(element)
				if err != nil {
					switch node.Cfg.GetString(CfgExceptionPlan) {
					case "stopList":
						node.Children = node.Children[:len(node.Children)-1]
						err := operator.Recovery()
						if err != nil {
							return fmt.Errorf("recovery error: %w", err)
						}
						return nil
					default:
						return fmt.Errorf("parse list node index %d error: %w", index, err)
					}
				}
				index++
			}
			return nil
		}()
		if err != nil {
			return fmt.Errorf("parse list node error: %w", err)
		}
		operator.PopBackup()
		node.Ctx.DeleteItem(CfgInList)
		return nil
	}
	if node.Cfg.GetBool(CfgIsTerminal) {
		err := operator.ParseTerminal(node)
		if err != nil {
			return err
		}
		return nil
	} else {
		if operator.ParseStruct != nil {
			ok, err := operator.ParseStruct(node)
			if ok {
				return err
			}
		}
		err := operator.Backup()
		if err != nil {
			return fmt.Errorf("backup error: %w", err)
		}
		for _, child := range node.Children {
			err := operator.NodeParse(child)
			if err != nil {
				if node.Cfg.GetString(CfgExceptionPlan) == "skip" {
					err = operator.Recovery()
					if err != nil {
						return fmt.Errorf("pop backup error: %w", err)
					}
					return nil
				}
				return fmt.Errorf("parse child node error: %w", err)
			}
		}
		err = operator.PopBackup()
		if err != nil {
			return fmt.Errorf("pop backup error: %w", err)
		}
		return nil
	}
}

func (d *DefParser) Generate(data any, node *base.Node) error {
	rootData := data
	var operator *Operator
	operator = &Operator{
		Mode: "generator",
		NodeParse: func(n *base.Node) error {
			return n.Generate(data)
		},
		ParseStruct: func(node *base.Node) (bool, error) {
			if GetNodePath(node) == "" {
				return false, nil
			}
			data, ok := getSubData(rootData, GetNodePath(node))
			if ok {
				switch ret := data.(type) {
				case []byte, string:
					err := d.Parse(base.NewBitReader(bytes.NewBuffer(utils.InterfaceToBytes(ret))), node)
					return true, err
				default:
					return false, nil
				}
			}
			return false, nil
		},
		ParseTerminal: func(node *base.Node) error {
			if !NodeIsTerminal(node) {
				return fmt.Errorf("node %s is not terminal", node.Name)
			}
			p := GetNodePath(node)
			data, ok := getSubData(rootData, p)
			if !ok {
				return fmt.Errorf("data %s not found", p)
			}
			if node.Cfg.Has(CfgElementIndex) {
				refD := reflect.ValueOf(data)
				if refD.Kind() != reflect.Slice || refD.Kind() != reflect.Array {
					return fmt.Errorf("data %s is not slice or array", p)
				}
				index := node.Cfg.GetUint64(CfgElementIndex)
				if index >= uint64(refD.Len()) {
					return fmt.Errorf("index %d out of range", index)
				}
				data = refD.Index(int(index)).Interface()
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
					buf := ConvertToBytes(data, length)
					rawRes, err := d.write(buf, length)
					if err != nil {
						return fmt.Errorf("write error: %w", err)
					}
					node.Cfg.SetItem(CfgNodeResult, rawRes)
					return nil
				} else {
					return errors.New("not support type")
				}
			} else {
				var raw []byte
				switch ret := data.(type) {
				case string:
					raw = []byte(ret)
				case []byte:
					raw = ret
				}
				raw = append(raw, node.Cfg.GetString(CfgDelimiter)...)
				rawRes, err := d.write(raw, uint64(len(raw)))
				if err != nil {
					return fmt.Errorf("write error: %w", err)
				}
				node.Cfg.SetItem(CfgNodeResult, rawRes)
				return nil
			}
		},
		Backup: func() error {
			return nil
		},
		Recovery: func() error {
			return nil
		},
		PopBackup: func() error {
			return nil
		},
	}
	return d.Operate(operator, node)
}
func (d *DefParser) Parse(data *base.BitReader, node *base.Node) error {
	var operator *Operator
	operator = &Operator{
		Mode: "parser",
		NodeParse: func(n *base.Node) error {
			return n.Parse(data)
		},
		ParseTerminal: func(node *base.Node) error {
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
					return nil
				} else {
					return errors.New("not support type")
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
					b, err := data.ReadBits(8)
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
				node.Cfg.SetItem(CfgNodeResult, res)
				return nil
			}
		},
		Backup: func() error {
			return data.Backup()
		},
		Recovery: func() error {
			return data.Recovery()
		},
		PopBackup: func() error {
			return data.PopBackup()
		},
	}
	return d.Operate(operator, node)
}

var noResultError = errors.New("no result")

func (d *DefParser) Result(node *base.Node) (any, error) {
	if NodeHasResult(node) {
		return GetResultByNode(node), nil
	}
	if node.Cfg.GetBool(CfgIsList) {
		res := []any{}
		for _, sub := range node.Children {
			d, err := sub.Result()
			if err != nil {
				if errors.Is(err, noResultError) {
					continue
				}
				return nil, err
			}
			res = append(res, d)
		}
		if len(res) == 0 {
			return nil, noResultError
		}
		return res, nil
	} else {
		res := map[string]any{}
		for _, sub := range node.Children {
			d, err := sub.Result()
			if err != nil {
				if errors.Is(err, noResultError) {
					continue
				}
				return nil, err
			}
			res[sub.Name] = d
		}
		if len(res) == 0 {
			return nil, noResultError
		}
		return res, nil
	}
}
