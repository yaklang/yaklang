package _default

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/binx"
	"github.com/yaklang/yaklang/common/utils"
	"reflect"
	"strconv"
	"strings"
)

type DefParser struct {
	base.BaseParser
}

func getSubData(d any, key string) (any, bool) {
	//if d, ok := d.(map[string]any); ok {
	//	v, ok := d[key]
	//	return v, ok
	//}
	refV := reflect.ValueOf(d)
	if refV.Kind() == reflect.Map {
		v := refV.MapIndex(reflect.ValueOf(key))
		if v.IsValid() {
			return v.Interface(), true
		}
	}
	return nil, false
}
func (d *DefParser) OnRoot(node *base.Node) error {
	node.Ctx.SetItem("root", node)
	rootChildMap := make(map[string]*base.Node)
	var walkNode func(node *base.Node) error
	walkNode = func(node *base.Node) error {
		if _, ok := node.Origin.(string); ok {
			node.Cfg.SetItem("isTerminal", true)
			nodeData := utils.InterfaceToString(node.Origin)
			if strings.HasPrefix(nodeData, "del:") {
				node.Cfg.SetItem("delimiter", nodeData[4:])
				return nil
			}
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
					node.Cfg.SetItem("length", l)
				} else {
					node.Cfg.SetItem("length", l*8)
				}
			} else {
				return errors.New("terminal node too many params")
			}
			if strings.HasSuffix(typeName, "...") {
				typeName = strings.TrimSuffix(typeName, "...")
				node.Cfg.SetItem("isList", true)
				node.Cfg.SetItem("type", typeName)
			} else {
				node.Cfg.SetItem("type", typeName)
			}
		} else {
			if len(node.Children) == 0 {
				node.Cfg.SetItem("isTerminal", true)
			}
			for i, child := range node.Children {
				err := walkNode(child)
				if err != nil {
					return err
				}
				if i == len(node.Children)-1 {
					child.Cfg.SetItem("lastNode", true)
				}
				child.Cfg.SetItem("parent", node)
			}
		}
		return nil
	}
	for _, child := range node.Children {
		rootChildMap[child.Name] = child
		err := walkNode(child)
		if err != nil {
			return fmt.Errorf("walk node error: %w", err)
		}
	}
	node.Ctx.SetItem("rootNodeMap", rootChildMap)
	return nil
}
func (d *DefParser) Generate(data any, node *base.Node) (*base.NodeResult, error) {
	if node.Name == "Header Length" {
		println()
	}
	if node.Name == "root" {
		irootNodeMap := node.Ctx.GetItem("rootNodeMap")
		if irootNodeMap == nil {
			return nil, errors.New("not set rootNodeMap")
		}
		rootNodeMap, ok := irootNodeMap.(map[string]*base.Node)
		if !ok {
			return nil, errors.New("rootNodeMap type error")
		}
		if v, ok := rootNodeMap["Package"]; !ok {
			return nil, errors.New("package node not found")
		} else {
			return v.Generate(data)
		}
	}
	if v := node.Cfg.GetItem("operator"); v != nil {
		result, err := ExecOperator(nil, data, node, utils.InterfaceToString(v), "generate")
		if err != nil {
			return nil, fmt.Errorf("eval operator error: %w", err)
		}
		return result, nil
	}
	if node.Cfg.GetItem("isTerminal") != nil {
		isDelmiter := node.Cfg.Has("delimiter")
		if !isDelmiter {
			isList := node.Cfg.GetBool("isList")
			if isList {
				result := base.NewNodeResultByNode(node)
				maxLength, err := getNodeLength(node)
				if err != nil {
					return nil, fmt.Errorf("get node length error: %w", err)
				}
				node.Ctx.SetItem("inList", true)
				refV := reflect.ValueOf(data)
				if refV.Kind() == reflect.Slice || refV.Kind() == reflect.Array {
					for i := 0; i < refV.Len(); i++ {
						data = refV.Index(i).Interface()
						if !node.Ctx.GetBool("inList") {
							break
						}
						element, err := d.GenerateTerminal(data, node)
						if err != nil {
							return nil, fmt.Errorf("parse list error: %w", err)
						}
						err = result.AppendChild(element)
						if err != nil {
							return nil, err
						}
						if result.Length >= maxLength {
							break
						}
					}
				} else {
					return nil, fmt.Errorf("data `%s` type error", node.Name)
				}
				result.Length = maxLength
				node.Ctx.DeleteItem("inList")
				return result, nil
			} else {
				result, err := d.GenerateTerminal(data, node)
				if err != nil {
					return nil, err
				}
				return result, nil
			}
		} else {
			delimiter := utils.InterfaceToString(node.Cfg.GetItem("delimiter"))
			if len(delimiter) == 0 {
				return nil, errors.New("delimiter length must be greater than 0")
			}
			var d []byte
			if v, ok := data.([]byte); ok {
				d = utils.InterfaceToBytes(v)
			}
			// 循环读取数据，直到遇到delimiter结束
			res := base.NewNodeResultByNode(node)
			res.IsTerminalData = true
			res.TerminalData = d
			res.Buffer.Write([]byte(delimiter))
			res.Length = uint64(len(d)) * 4
			return res, nil
		}
	} else {
		result := base.NewNodeResultByNode(node)
		for _, child := range node.Children {
			subData, ok := getSubData(data, child.Name)
			if !ok {
				return nil, fmt.Errorf("data `%s` not found", child.Name)
			}
			res, err := child.Generate(subData)
			if err != nil {
				return nil, fmt.Errorf("parse child node error: %w", err)
			}
			err = result.AppendChild(res)
			if err != nil {
				return nil, err
			}
			node.Cfg.SetItem("now length", result.Length)
		}
		node.Cfg.SetItem("length", result.Length)
		return result, nil
	}
}
func (d *DefParser) GenerateTerminal(data any, node *base.Node) (*base.NodeResult, error) {
	itypeName := node.Cfg.GetItem("type")
	if itypeName == nil {
		return nil, errors.New("not set type")
	}
	typeName := utils.InterfaceToString(itypeName)
	var endian binx.ByteOrderEnum
	iendian := node.Cfg.GetItem("endian")
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
	if utils.StringArrayContains([]string{"int", "uint", "int8", "uint8", "int16", "uint16", "int32", "uint32", "int64", "uint64", "string", "bool", "raw"}, typeName) {
		length, err := getNodeLength(node)
		if err != nil {
			return nil, fmt.Errorf("get node length error: %w", err)
		}
		if node.Name == "Header Length" {
			println()
		}
		if length == 0 {
			return nil, nil
		}
		switch typeName {
		case "string":
			typeName = "bytes"
		case "bytes":
			typeName = "raw"
		}
		var buf []byte
		number, ok := base.InterfaceToUint64(data)
		if ok {
			l := length / 8
			if length%8 != 0 {
				l++
			}
			if endian == binx.BigEndianByteOrder {
				var i uint64
				for ; i < l; i++ {
					buf = append(buf, byte(number>>(uint64(l-i-1)*8)))
				}
			} else {
				var i uint64
				for ; i < l; i++ {
					buf = append(buf, byte(number>>(i*8)))
				}
			}
		} else {
			switch ret := data.(type) {
			case string:
				buf = []byte(ret)
			case []byte:
				buf = ret
			default:
				return nil, fmt.Errorf("unknown type: %v", data)
			}
		}
		node.Cfg.SetItem("result", data)
		result := base.NewNodeResultByNode(node)
		result.Length = length
		result.TerminalData = data
		err = result.Write(buf, length)
		if err != nil {
			return nil, err
		}
		return result, nil
	} else {
		irootNodeMap := node.Ctx.GetItem("rootNodeMap")
		if irootNodeMap == nil {
			return nil, errors.New("not set rootNodeMap")
		}
		rootNodeMap, ok := irootNodeMap.(map[string]*base.Node)
		if !ok {
			return nil, errors.New("rootNodeMap type error")
		}
		v, ok := rootNodeMap[typeName]
		if !ok {
			return nil, fmt.Errorf("type `%s` not found", typeName)
		}
		return v.Generate(data)
	}
}
func (d *DefParser) Parse(data *base.BitReader, node *base.Node) (*base.NodeResult, error) {
	if node.Name == "root" {
		irootNodeMap := node.Ctx.GetItem("rootNodeMap")
		if irootNodeMap == nil {
			return nil, errors.New("not set rootNodeMap")
		}
		rootNodeMap, ok := irootNodeMap.(map[string]*base.Node)
		if !ok {
			return nil, errors.New("rootNodeMap type error")
		}
		if v, ok := rootNodeMap["Package"]; !ok {
			return nil, errors.New("package node not found")
		} else {
			return v.Parse(data)
		}
	}
	if v := node.Cfg.GetItem("operator"); v != nil {
		result, err := ExecOperator(data, nil, node, utils.InterfaceToString(v), "parse")
		if err != nil {
			return nil, fmt.Errorf("eval operator error: %w", err)
		}
		return result, nil
	}
	if node.Cfg.GetItem("isTerminal") != nil {
		isDelmiter := node.Cfg.Has("delimiter")
		if !isDelmiter {
			isList := node.Cfg.GetBool("isList")
			if isList {
				result := base.NewNodeResultByNode(node)
				maxLength, err := getNodeLength(node)
				if err != nil {
					return nil, fmt.Errorf("get node length error: %w", err)
				}
				node.Ctx.SetItem("inList", true)
				for {
					if !node.Ctx.GetBool("inList") {
						break
					}
					//element := base.NewNodeResultByNode(node)
					element, err := d.ParseTerminal(data, node)
					if err != nil {
						return nil, fmt.Errorf("parse list error: %w", err)
					}
					if element == nil {
						continue
					}
					err = result.AppendChild(element)
					if err != nil {
						return nil, err
					}
					if result.Length >= maxLength {
						break
					}
				}
				result.Length = maxLength
				node.Ctx.DeleteItem("inList")
				return result, nil
			} else {
				//result := base.NewNodeResultByNode(node)
				result, err := d.ParseTerminal(data, node)
				if err != nil {
					return nil, err
				}
				return result, nil
			}
		} else {
			delimiter := utils.InterfaceToString(node.Cfg.GetItem("delimiter"))
			if len(delimiter) == 0 {
				return nil, errors.New("delimiter length must be greater than 0")
			}
			delimitern := 0
			d := []byte{}
			// 循环读取数据，直到遇到delimiter结束
			for {
				b := make([]byte, 1)
				_, err := data.Read(b)
				if err != nil {
					return nil, err
				}
				if delimiter[delimitern] == b[0] {
					delimitern++
				} else {
					delimitern = 0
				}
				if delimitern == len(delimiter) {
					d = d[:len(d)+1-delimitern]
					break
				}
				d = append(d, b...)
			}
			res := base.NewNodeResultByNode(node)
			res.IsTerminalData = true
			res.TerminalData = d
			res.Buffer.Write([]byte(delimiter))
			res.Length = uint64(len(d)) * 4
			return res, nil
		}
	} else {
		result := base.NewNodeResultByNode(node)
		for _, child := range node.Children {
			res, err := child.Parse(data)
			if err != nil {
				return nil, fmt.Errorf("parse child node error: %w", err)
			}
			if res == nil {
				continue
			}
			err = result.AppendChild(res)
			if err != nil {
				return nil, err
			}
			node.Cfg.SetItem("now length", result.Length)
		}
		node.Cfg.SetItem("length", result.Length)
		return result, nil
	}
}
func (d *DefParser) ParseTerminal(data *base.BitReader, node *base.Node) (*base.NodeResult, error) {
	itypeName := node.Cfg.GetItem("type")
	if itypeName == nil {
		return nil, errors.New("not set type")
	}
	typeName := utils.InterfaceToString(itypeName)
	var endian binx.ByteOrderEnum
	iendian := node.Cfg.GetItem("endian")
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
	if utils.StringArrayContains([]string{"int", "uint", "int8", "uint8", "int16", "uint16", "int32", "uint32", "int64", "uint64", "string", "bool", "raw"}, typeName) {
		length, err := getNodeLength(node)
		if err != nil {
			return nil, fmt.Errorf("get node length error: %w", err)
		}
		if length == 0 {
			return nil, nil
		}
		switch typeName {
		case "string":
			typeName = "bytes"
		case "bytes":
			typeName = "raw"
		}
		buf, err := data.ReadBits(length)
		if err != nil {
			return nil, fmt.Errorf("read bits error: %w", err)
		}
		res := binx.NewResult(buf)
		res.Identifier = node.Name
		res.ByteOrder = endian
		res.Type = binx.BinaryTypeVerbose(typeName)
		node.Cfg.SetItem("result", res.Value())
		result := base.NewNodeResultByNode(node)
		result.Length = length
		result.TerminalData = res.Value()
		result.Buffer.Write(res.GetBytes())
		return result, nil
	} else {
		irootNodeMap := node.Ctx.GetItem("rootNodeMap")
		if irootNodeMap == nil {
			return nil, errors.New("not set rootNodeMap")
		}
		rootNodeMap, ok := irootNodeMap.(map[string]*base.Node)
		if !ok {
			return nil, errors.New("rootNodeMap type error")
		}
		v, ok := rootNodeMap[typeName]
		if !ok {
			return nil, fmt.Errorf("type `%s` not found", typeName)
		}
		return v.Parse(data)
	}
}
