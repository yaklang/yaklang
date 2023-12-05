package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/binx"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v2"
	"math"
	"reflect"
	"strconv"
	"strings"
)

func getSubData(d any, key string) (any, bool) {
	p := strings.Split(key, ".")
	for _, ele := range p {
		refV := reflect.ValueOf(d)
		if refV.Kind() == reflect.Map {
			v := refV.MapIndex(reflect.ValueOf(ele))
			if !v.IsValid() {
				return nil, false
			} else {
				d = v.Interface()
			}
		} else if refV.Kind() == reflect.Slice || refV.Kind() == reflect.Array {
			if !strings.HasPrefix(ele, "#") {
				return nil, false
			}
			index, err := strconv.Atoi(ele[1:])
			if err != nil {
				return nil, false
			}
			if index >= refV.Len() {
				return nil, false
			}
			d = refV.Index(index).Interface()
		} else {
			return nil, false
		}
	}
	return d, true
}
func GetNodePath(node *base.Node) string {
	p := ""
	for {
		if node.Name == "Package" {
			break
		}
		if node.Cfg.GetBool(CfgIsTempRoot) {
			break
		}
		parent := node.Cfg.GetItem(CfgParent).(*base.Node)
		if parent.Cfg.GetBool(CfgIsList) {
			index := 0
			for i, child := range parent.Children {
				if child == node {
					index = i
					break
				}
			}
			p = fmt.Sprintf("#%d.", index) + p
		} else {
			p = node.Name + "." + p
		}
		node = node.Cfg.GetItem(CfgParent).(*base.Node)
	}
	if len(p) > 0 {
		p = p[:len(p)-1]
	}
	return p
}
func ConvertToVar(v []byte, length uint64, typeName string) any {
	switch typeName {
	case "int", "int8", "int16", "int32", "int64":
		var n int64
		for i := 0; i < int(length); i++ {
			n <<= 8
			n += int64(v[i])
		}
		return n
	case "uint", "uint8", "uint16", "uint32", "uint64":
		var n uint64
		for i := 0; i < int(length); i++ {
			n <<= 8
			n += uint64(v[i])
		}
		return n
	case "bytes":
		return string(v)
	case "raw":
		return v
	default:
		return v
	}
}
func AnyToInt64(d any) int64 {
	switch ret := d.(type) {
	case int, int8, int16, int32, int64:
		return int64(reflect.ValueOf(ret).Int())
	case uint, uint8, uint16, uint32, uint64:
		return int64(reflect.ValueOf(ret).Uint())
	case string:
		return int64(len(ret))
	case []byte:
		return int64(len(ret))
	default:
		return 0
	}
}
func AnyToUint64(d any) uint64 {
	switch ret := d.(type) {
	case int, int8, int16, int32, int64:
		return uint64(reflect.ValueOf(ret).Int())
	case uint, uint8, uint16, uint32, uint64:
		return uint64(reflect.ValueOf(ret).Uint())
	case string:
		return uint64(len(ret))
	case []byte:
		return uint64(len(ret))
	default:
		return 0
	}
}
func ConvertToBytes(v any, length uint64) []byte {
	switch ret := v.(type) {
	case int, int8, int16, int32, int64:
		l := math.Ceil(float64(length) / 8)
		res := make([]byte, 0)
		for i := 0; i < int(l); i++ {
			res = append(res, byte(AnyToInt64(ret)>>uint(8*(int(l)-1-i))))
		}
		return res
	case uint, uint8, uint16, uint32, uint64:
		l := math.Ceil(float64(length) / 8)
		res := make([]byte, 0)
		for i := 0; i < int(l); i++ {
			res = append(res, byte(AnyToUint64(ret)>>uint(8*(int(l)-1-i))))
		}
		return res
	case string:
		return []byte(v.(string))
	case []byte:
		return v.([]byte)
	default:
		return []byte{}
	}
}
func appendNode(parent *base.Node, child *base.Node) error {
	err := parent.AppendNode(child)
	if err != nil {
		return err
	}
	return InitNode(utils.GetLastElement(parent.Children))
}
func getNodeByPath(node *base.Node, key string) *base.Node {
	splits := strings.Split(key, ".")
	var findChildByPath func(node *base.Node, path ...string) *base.Node
	findChildByPath = func(node *base.Node, path ...string) *base.Node {
		if len(path) == 0 {
			return node
		}
		var child1 *base.Node
		for _, child := range node.Children {
			if child.Name == path[0] {
				child1 = child
			}
		}
		if child1 == nil {
			return nil
		}
		return findChildByPath(child1, path[1:]...)
	}
	var targetNode *base.Node
	if strings.HasPrefix(splits[0], "@") {
		splits[0] = splits[0][1:]
		targetNode = findChildByPath(node.Ctx.GetItem("root").(*base.Node), append([]string{"Package"}, splits...)...)
	} else {
		targetNode = findChildByPath(node, splits...)
	}
	if targetNode == nil {
		return nil
	}
	return targetNode
}

func getNodeAttrByPath(node *base.Node, key string) (*base.Node, string) {
	splits := strings.Split(key, ".")
	node = getNodeByPath(node, strings.Join(splits[:len(splits)-1], "."))
	return node, splits[len(splits)-1]
}
func getRemainingSpace(node *base.Node) (uint64, error) {
	log.Debugf("get remaining space for node %s", node.Name)
	if node.Name == "root" {
		return math.MaxUint64, nil
	}
	iparent := node.Cfg.GetItem(CfgParent)
	if iparent == nil {
		return 0, errors.New("not set parentCfg")
	}
	parentNode, ok := iparent.(*base.Node)
	if !ok {
		return 0, errors.New("get parent failed")
	}
	// 当前节点剩余长度 = 父节点剩余长度(或父节点配置的长度) - 当前节点之前的兄弟节点长度
	parentRemaininigLength, err := getRemainingSpace(parentNode)
	if err != nil {
		return 0, err
	}
	var fieldsInScope []string
	inScope := false

	if parentNode.Cfg.Has("length-from-field") {
		// 从field 读取length
		if parentNode.Cfg.Has("length-from-field") {
			fieldName := parentNode.Cfg.GetString("length-from-field")
			if node.Name != fieldName {
				for _, child := range parentNode.Children {
					if child.Name == fieldName {
						if !child.Cfg.Has(CfgNodeResult) {
							break
						}
						res := GetResultByNode(child)
						if v, ok := base.InterfaceToUint64(res); ok {
							total := v
							if parentNode.Cfg.Has("length-from-field-multiply") {
								mul, ok := base.InterfaceToUint64(parentNode.Cfg.GetItem("length-from-field-multiply"))
								if !ok {
									return 0, fmt.Errorf("length-from-field-multiply type error")
								}
								total *= mul
							}
							if total > parentRemaininigLength {
								return 0, fmt.Errorf("node %s length %d over max size %d", node.Name, total, parentRemaininigLength)
							}
							if parentNode.Cfg.Has("length-for-field") { // 当存在字段限制，且当前节点在限制范围内时，更新parentRemaininigLength
								fieldsStr := parentNode.Cfg.GetString("length-for-field")
								fieldsInScope = strings.Split(fieldsStr, ",")
								for _, field := range fieldsInScope {
									if field == node.Name {
										inScope = true
										parentRemaininigLength = total
										break
									}
								}
							} else {
								parentRemaininigLength = total
							}
						} else {
							return 0, fmt.Errorf("field %s type error", fieldName)
						}
						break
					}
				}
			}
		}
	}
	// 从config 读取
	if parentNode.Cfg.Has(CfgLength) {
		l := parentNode.Cfg.GetUint64(CfgLength)
		if l > parentRemaininigLength {
			return 0, fmt.Errorf("node %s length %d over max size %d", node.Name, l, parentRemaininigLength)
		}
		parentRemaininigLength = l
	}
	var nowLength uint64
	if inScope {
		for _, sub := range parentNode.Children {
			if sub == node {
				break
			}
			if utils.StringArrayContains(fieldsInScope, sub.Name) {
				nowLength += CalcNodeResultLength(sub)
			}
		}
		return parentRemaininigLength - nowLength, nil
	} else {
		for _, sub := range parentNode.Children {
			if sub == node {
				break
			}
			nowLength += CalcNodeResultLength(sub)
		}
		return parentRemaininigLength - nowLength, nil
	}
}
func getNodeLength(node *base.Node) (uint64, error) {
	remainingLength, err := getRemainingSpace(node)
	if err != nil {
		return 0, err
	}
	var length uint64
	getLengthFaild := false
	if !node.Cfg.Has(CfgLength) && !node.Cfg.Has("length-from-field") {
		typeName := node.Cfg.GetString(CfgType)
		switch typeName {
		case "int":
			length = 32
		case "uint":
			length = 32
		case "int8":
			length = 8
		case "uint8":
			length = 8
		case "int16":
			length = 16
		case "uint16":
			length = 16
		case "int32":
			length = 32
		case "uint32":
			length = 32
		case "int64":
			length = 64
		case "uint64":
			length = 64
		default:
			getLengthFaild = true
		}
	} else {
		if node.Cfg.Has("length") {
			length = node.Cfg.GetUint64("length")
		} else if node.Cfg.Has("length-from-field") {
			fieldName := node.Cfg.GetString("length-from-field")
			iparent := node.Cfg.GetItem("parent")
			parent, ok := iparent.(*base.Node)
			if !ok {
				return 0, fmt.Errorf("get parent failed")
			}
			for _, child := range parent.Children {
				if child.Name == fieldName {
					if !child.Cfg.Has(CfgNodeResult) {
						break
					}
					res := GetResultByNode(child)
					if v, ok := base.InterfaceToUint64(res); ok {
						var mul uint64 = 1
						if node.Cfg.Has("length-from-field-multiply") {
							mul = node.Cfg.ConvertUint64("length-from-field-multiply")
						}
						length = v * mul
					} else {
						return 0, fmt.Errorf("field %s type error", fieldName)
					}
					break
				}
			}
		}

	}
	if !getLengthFaild {
		if length > remainingLength {
			return 0, fmt.Errorf("node type %s,length %d over max size %d", node.Cfg.GetString(CfgType), length, remainingLength)
		}
		return length, nil
	} else {
		return remainingLength, nil
	}
}
func walkNode(node *base.Node, handle func(node *base.Node) bool) {
	if !handle(node) {
		return
	}
	for _, child := range node.Children {
		walkNode(child, handle)
	}
}
func GetBytesByNode(node *base.Node) []byte {
	res, err := getNodeResult(node, true)
	if err != nil {
		log.Errorf("get node result error: %v", err)
	}
	return res.([]byte)
}
func GetResultByNode(node *base.Node) any {
	if NodeHasResult(node) {
		return GetNodeResult(node)
	} else {
		return GetBytesByNode(node)
	}
}
func getMapSliceElement(d yaml.MapSlice, path string) any {
	var findChildByPath func(d any, path ...string) any
	findChildByPath = func(d any, path ...string) any {
		if len(path) == 0 {
			return d
		}
		m, ok := d.(yaml.MapSlice)
		if !ok {
			return nil
		}
		var child1 any
		for _, child := range m {
			if child.Key == path[0] {
				child1 = child
			}
		}
		if child1 == nil {
			return nil
		}

		return findChildByPath(child1, path[1:]...)
	}
	return findChildByPath(d, strings.Split(path, ".")...)
}
func CalcNodeResultLength(node *base.Node) uint64 {
	var length uint64
	walkNode(node, func(node *base.Node) bool {
		if NodeHasResult(node) {
			pos := GetNodeResultPos(node)
			length += pos[1] - pos[0]
		}
		return true
	})
	return length
}
func cfgDeleteItem(node *base.Node, key string) {
	for _, child := range node.Children {
		cfgDeleteItem(child, key)
	}
	node.Cfg.DeleteItem(key)
}
func NodeIsTerminal(node *base.Node) bool {
	return node.Cfg.GetBool(CfgIsTerminal)
}
func NodeIsDelimiter(node *base.Node) bool {
	return node.Cfg.GetBool(CfgDelimiter)
}
func NodeHasResult(node *base.Node) bool {
	return node.Cfg.Has(CfgNodeResult)
}
func GetNodeResultPos(node *base.Node) [2]uint64 {
	return node.Cfg.GetItem(CfgNodeResult).([2]uint64)
}
func GetNodeResult(node *base.Node) any {
	res, err := getNodeValue(node)
	if err != nil {
		log.Errorf("get node result error: %v", err)
	}
	return res
}

func getNodeResult(node *base.Node, isByte bool) (any, error) {
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
	if !node.Cfg.Has(CfgNodeResult) {
		var startPos, endPos [2]uint64
		first := false
		walkNode(node, func(n *base.Node) bool {
			if NodeIsTerminal(n) {
				if first {
					first = false
					startPos = GetNodeResultPos(n)
				} else {
					endPos = GetNodeResultPos(n)
				}
			}
			return true
		})
		buffer := node.Ctx.GetItem("buffer").(*bytes.Buffer)
		byts := buffer.Bytes()
		return byts[startPos[0]/8 : endPos[1]/8], nil
	}
	resPoint := node.Cfg.GetItem(CfgNodeResult).([2]uint64)
	buffer := node.Ctx.GetItem("buffer").(*bytes.Buffer)
	byts := buffer.Bytes()
	writer := node.Ctx.GetItem("writer").(*base.BitWriter)
	if writer.PreIsBit {
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
	if isByte {
		return res.Bytes, nil
	} else {
		return res.Value(), nil
	}
}
func getNodeValue(node *base.Node) (any, error) {
	return getNodeResult(node, false)
}
