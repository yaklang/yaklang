package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v2"
	"math"
	"reflect"
	"strconv"
	"strings"
)

func newListNodeValue(name string, children ...*base.NodeValue) *base.NodeValue {
	var v *base.NodeValue
	v = &base.NodeValue{
		Name:      name,
		ListValue: true,
		Value:     children,
		AppendSub: func(value *base.NodeValue) error {
			val, ok := v.Value.([]*base.NodeValue)
			if !ok {
				return errors.New("current node is complex node")
			}
			v.Value = append(val, value)
			return nil
		},
	}
	return v
}
func newStructNodeValue(name string, children ...*base.NodeValue) *base.NodeValue {
	var v *base.NodeValue
	v = &base.NodeValue{
		Name:      name,
		ListValue: false,
		Value:     children,
		AppendSub: func(value *base.NodeValue) error {
			val, ok := v.Value.([]*base.NodeValue)
			if !ok {
				return errors.New("current node is complex node")
			}
			v.Value = append(val, value)
			return nil
		},
	}
	return v
}

func newNodeValue(name string, v any) *base.NodeValue {
	if v == (*[]*base.NodeValue)(nil) {
		println()
	}
	return &base.NodeValue{
		Name:      name,
		Value:     v,
		ListValue: false,
		AppendSub: func(value *base.NodeValue) error {
			return errors.New("current node is complex node")
		},
	}
}
func ListNodeNewElement(node *base.Node) (*base.Node, error) {
	if !node.Cfg.GetBool(CfgIsList) {
		return nil, errors.New("not list node")
	}
	if len(node.Children) == 0 {
		panic("not set template node")
	}
	if !node.Cfg.Has("template") {
		templateNode, err := ParseRefNode(node.Children[0])
		if err != nil {
			panic(fmt.Errorf("new node by type error: %w", err))
		}
		node.Cfg.SetItem("template", templateNode)
		node.Children = nil
	}
	elementTemplate := node.Cfg.GetItem("template").(*base.Node)
	element := elementTemplate.Copy()
	element.Cfg.SetItem(CfgParent, node)
	node.Children = append(node.Children, element)
	return element, nil
}
func ParseRefNode(node *base.Node) (*base.Node, error) {
	if !node.Cfg.Has(CfgRefType) {
		return node, nil
	}
	typeNode, err := NewNodeByType(node, node.Cfg.GetString(CfgRefType))
	if err != nil {
		return nil, fmt.Errorf("new node by type error: %w", err)
	}
	parentCfg := base.CopyConfig(node.Cfg)
	parentCfg.DeleteItem(CfgRefType)
	typeNode.Cfg = base.AppendConfig(parentCfg, typeNode.Cfg)
	typeNode.Name = node.Name
	return typeNode, nil
}
func NewNodeByType(node *base.Node, typeName string) (*base.Node, error) {
	irootNodeMap := node.Ctx.GetItem(CfgRootMap)
	if irootNodeMap == nil {
		return nil, errors.New("not set rootNodeMap")
	}
	rootNodeMap, ok := irootNodeMap.(map[string]*base.Node)
	if !ok {
		return nil, errors.New("rootNodeMap type error")
	}
	v, ok := rootNodeMap[typeName]
	if !ok {
		v = getNodeByPath(node, typeName)
		if v == nil {
			return nil, fmt.Errorf("type `%s` not found", typeName)
		}
	}
	return base.NewNodeTreeWithConfig(node.Cfg, node.Name, v.Origin, node.Ctx)
	//return v.Copy(), nil
}

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
		parent := GetParentNode(node)
		if parent == nil {
			break
		}
		if parent.Cfg.GetBool(CfgIsList) {
			index := 0
			for i, child := range GetSubNodes(parent) {
				if child == node {
					index = i
					break
				}
			}
			p = fmt.Sprintf("#%d.", index) + p
		} else {
			p = node.Name + "." + p
		}
		node = parent
	}
	if len(p) > 0 {
		p = p[:len(p)-1]
	}
	return p
}
func ConvertToVar(v []byte, length uint64, endian, typeName string) any {
	if endian == "big" {
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
	} else {
		switch typeName {
		case "int", "int8", "int16", "int32", "int64":
			var n int64
			for i := 0; i < int(length); i++ {
				n <<= 8
				n += int64(v[int(length)-1-i])
			}
			return n
		case "uint", "uint8", "uint16", "uint32", "uint64":
			var n uint64
			for i := 0; i < int(length); i++ {
				n <<= 8
				n += uint64(v[int(length)-1-i])
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
func IsNumber(d any) bool {
	switch d.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32, float64:
		return true
	default:
		return false
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
func GetParentNode(node *base.Node) *base.Node {
	isPackage := func(node *base.Node) bool {
		if node.Name == "Package" && node.Cfg.GetItem("parent") == node.Ctx.GetItem("root") {
			return true
		}
		return false
	}
	var getParent func(node *base.Node) *base.Node
	getParent = func(node *base.Node) *base.Node {
		var parent *base.Node
		iparent := node.Cfg.GetItem(CfgParent)
		if iparent == nil {
			return nil
		}
		parentNode, ok := iparent.(*base.Node)
		if !ok {
			return nil
		}
		if parentNode.Cfg.GetBool(CfgIsRefType) || parentNode.Cfg.GetBool("unpack") || isPackage(parentNode) {
			parent = getParent(parentNode)
		} else {
			parent = parentNode
		}
		return parent
	}
	return getParent(node)
}
func GetSubNodes(node *base.Node) []*base.Node {
	isPackage := func(node *base.Node) bool {
		if node.Name == "Package" && node.Cfg.GetItem("parent") == node.Ctx.GetItem("root") {
			return true
		}
		return false
	}
	var getSubs func(node *base.Node) []*base.Node
	getSubs = func(node *base.Node) []*base.Node {
		children := []*base.Node{}
		for _, sub := range node.Children {
			if sub.Cfg.GetBool(CfgIsRefType) || sub.Cfg.GetBool("unpack") || isPackage(sub) {
				children = append(children, getSubs(sub)...)
			} else {
				children = append(children, sub)
			}
		}
		return children
	}
	return getSubs(node)
}
func getNodeByPath(node *base.Node, key string) *base.Node {
	splits := strings.Split(key, "/")
	var findChildByPath func(node *base.Node, path ...string) *base.Node
	findChildByPath = func(node *base.Node, path ...string) *base.Node {
		if node == nil {
			return nil
		}
		if len(path) == 0 {
			return node
		}
		var child1 *base.Node
		if path[0] == ".." {
			child1 = node.Cfg.GetItem(CfgParent).(*base.Node)
		} else {
			for _, child := range GetSubNodes(node) {
				if child.Name == path[0] {
					child1 = child
				}
			}
		}
		return findChildByPath(child1, path[1:]...)
	}
	var targetNode *base.Node
	if strings.HasPrefix(splits[0], "@") {
		splits[0] = splits[0][1:]
		targetNode = findChildByPath(node.Ctx.GetItem("root").(*base.Node), splits...)
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
func parseLengthByLengthConfig(node *base.Node) (uint64, bool, error) {
	if node.Name == "root" {
		return math.MaxUint64, false, nil
	}
	iparent := node.Cfg.GetItem(CfgParent)
	if iparent == nil {
		return 0, false, errors.New("not set parentCfg")
	}
	parentNode, ok := iparent.(*base.Node)
	if !ok {
		return 0, false, errors.New("get parent failed")
	}
	parentLength, parentLengthOK, err := parseLengthByLengthConfig(parentNode)
	if err != nil {
		return 0, false, fmt.Errorf("parse parent length error: %v", err)
	}
	//parentRemaininigLength := uint64(0)
	var length uint64
	getLengthOK := false
	if node.Cfg.Has("length") {
		length = node.Cfg.GetUint64("length")
		getLengthOK = true
	} else {
		if node.Cfg.Has(CfgType) {
			typeName := node.Cfg.GetString(CfgType)
			ok := true
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
				ok = false
			}
			if ok {
				getLengthOK = true
			}
		}
		if !getLengthOK {
			if node.Cfg.Has("length-from-field") {
				// 从field 读取length
				if node.Cfg.Has("length-from-field") {
					fieldName := node.Cfg.GetString("length-from-field")
					target := getNodeByPath(node, fieldName)
					if target.Cfg.Has(CfgNodeResult) {
						res := GetResultByNode(target)
						if v, ok := base.InterfaceToUint64(res); ok {
							total := v
							total = total * getMulti(node)
							if node.Cfg.Has("length-from-field-multiply") {
								imulti := node.Cfg.GetItem("length-from-field-multiply")
								var multi uint64
								switch imulti.(type) {
								case string:
									n, err := strconv.Atoi(imulti.(string))
									if err != nil {
										return 0, false, fmt.Errorf("length-from-field-multiply type error")
									}
									multi = uint64(n)
								default:
									mul, ok := base.InterfaceToUint64(node.Cfg.GetItem("length-from-field-multiply"))
									if !ok {
										return 0, false, fmt.Errorf("length-from-field-multiply type error")
									}
									multi = mul
								}
								total *= multi
							}
							length = total
							getLengthOK = true
							//if node.Cfg.Has("length-for-field") { // 当存在字段限制，且当前节点在限制范围内时，更新parentRemaininigLength
							//	fieldsStr := node.Cfg.GetString("length-for-field")
							//	fieldsInScope = strings.Split(fieldsStr, ",")
							//	for _, field := range fieldsInScope {
							//		if field == node.Name {
							//			length = total
							//			break
							//		}
							//	}
							//} else {
							//	length = total
							//}
						} else {
							return 0, false, fmt.Errorf("field %s type error", fieldName)
						}
					}

				}
			}
		}
	}
	var currentNodeLength uint64
	for _, sub := range parentNode.Children {
		if sub == node {
			break
		}
		currentNodeLength += CalcNodeResultLength(sub)
	}
	remainingLength := parentLength - currentNodeLength
	if getLengthOK {
		if length > remainingLength {
			spew.Dump(GetNodePath(node))
			return 0, false, fmt.Errorf("node type %s,length %d over max size %d", node.Cfg.GetString(CfgType), length, remainingLength)
		}
		return length, true, nil
	} else {
		return remainingLength, parentLengthOK, nil
	}
}
func getNodeLength(node *base.Node) (uint64, error) {
	remainingLength, ok, err := parseLengthByLengthConfig(node)
	//if !ok {
	//	return 0, errors.New("parse length by length config error")
	//}
	_ = ok
	if err != nil {
		return 0, err
	}
	return remainingLength, nil
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
	return node.Cfg.Has(CfgDelimiter) || node.Cfg.Has(CfgDel)
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
	endian := node.Cfg.GetString(CfgEndian)
	if endian != "little" {
		endian = "big"
	}
	if !node.Cfg.Has(CfgNodeResult) {
		var start, end uint64
		first := true
		walkNode(node, func(n *base.Node) bool {
			if NodeHasResult(n) {
				if first {
					first = false
					p := GetNodeResultPos(n)
					start = p[0]
					end = p[1]
				} else {
					p := GetNodeResultPos(n)
					end = p[1]
				}
			}
			return true
		})
		buffer := node.Ctx.GetItem("buffer").(*bytes.Buffer)
		byts := buffer.Bytes()
		if start > end {
			return nil, nil
		}
		return byts[start/8 : end/8], nil
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
	if isByte {
		return buf, nil
	} else {
		if node.Cfg.GetString(CfgType) == "string" {
			return string(buf), nil
		}
		_ = endian
		return ConvertToVar(buf, uint64(len(buf)), endian, node.Cfg.GetString(CfgType)), nil
	}
}
func getNodeValue(node *base.Node) (any, error) {
	return getNodeResult(node, false)
}
