package stream_parser

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/utils"
)

// operatorNode binds a node once. Prepared operators resolve Go methods on
// demand, instead of allocating every YakNode function field on every visit.
// General/custom Yak operators retain the public YakNode representation.
type operatorNode struct {
	origin               *base.Node
	operator             func(*base.Node) (func(bool), error)
	Name                 string
	CalcNodeResultLength func() uint64
}

func convertOperatorNode(n *base.Node, process func(*base.Node) (func(bool), error)) *operatorNode {
	return &operatorNode{origin: n, operator: process, Name: n.Name}
}
func (yakNode *operatorNode) getRootNode(key string) *operatorNode {
	node := yakNode.origin
	operator := yakNode.operator

	rootMap := node.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)
	if v, ok := rootMap[key]; ok {
		return convertOperatorNode(v, operator)
	}
	panic("not found root node " + key)
}
func (yakNode *operatorNode) AddInfo(key string, v any) {
	node := yakNode.origin

	if node.Cfg.Has("additionInfo") {
		additionInfo := node.Cfg.GetItem("additionInfo").(map[string]any)
		additionInfo[key] = v
	} else {
		node.Cfg.SetItem("additionInfo", map[string]any{
			key: v,
		})
	}
}
func (yakNode *operatorNode) GetInfo(key string) any {
	node := yakNode.origin

	if node.Cfg.Has("additionInfo") {
		additionInfo := node.Cfg.GetItem("additionInfo").(map[string]any)
		return additionInfo[key]
	}
	return nil
}
func (yakNode *operatorNode) ProcessSubNode(name string) any {
	return yakNode.GetSubNode(name).Process()
}
func (yakNode *operatorNode) TryProcessSubNode(name string) (result any, response map[string]any) {
	operator := yakNode.operator

	typeNode := yakNode.GetSubNode(name)
	response = map[string]any{
		"OK":       false,
		"Message":  "",
		"Save":     func() {},
		"Recovery": func() {},
	}
	defer func() {
		if e := recover(); e != nil {
			response["Message"] = fmt.Sprintf("%v", e)
		}
	}()

	yakNode.AppendNode(typeNode)
	copyNode := yakNode.origin.Children[len(yakNode.origin.Children)-1]
	copyYakNode := convertOperatorNode(copyNode, operator)

	deferFun, err := operator(copyNode)
	response["Save"] = func() {
		deferFun(false)
	}
	response["GetNode"] = func() any {
		return copyYakNode
	}
	response["Recovery"] = func() {
		deferFun(true)
		yakNode.origin.Children = yakNode.origin.Children[:len(yakNode.origin.Children)-1]
	}
	if err != nil {
		response["Message"] = err.Error()
		return nil, response
	}

	result = copyYakNode.Result()
	response["Result"] = result
	response["OK"] = true
	return result, response
}
func (yakNode *operatorNode) GetMaxLength(uints ...string) uint64 {
	n := getMulti(yakNode.origin, uints...)
	l, err := getNodeLength(yakNode.origin)
	if err != nil {
		panic(err)
	}
	return l / n
}
func (yakNode *operatorNode) HasMaxLength() bool {
	_, bounded, err := parseLengthByLengthConfig(yakNode.origin)
	if err != nil {
		panic(err)
	}
	return bounded
}
func (yakNode *operatorNode) NewUnknownNode(datas ...string) *operatorNode {
	node := yakNode.origin
	operator := yakNode.operator

	name := utils.InterfaceToString(utils.GetLastElement(datas))
	unknownNode := convertOperatorNode(&base.Node{
		Name:   "Unknown",
		Origin: "raw",
		Cfg:    base.NewConfig(yakNode.origin.Cfg),
		Ctx:    yakNode.origin.Ctx,
	}, operator)
	err := appendNode(node, unknownNode.origin)
	if err != nil {
		panic(err)
	}
	if name != "" {
		utils.GetLastElement(node.Children).Name = name
	}
	return convertOperatorNode(utils.GetLastElement(node.Children), operator)
}
func (yakNode *operatorNode) NewEmptyNode(datas ...string) *operatorNode {
	node := yakNode.origin
	operator := yakNode.operator

	name := utils.InterfaceToString(utils.GetLastElement(datas))
	unknownNode := convertOperatorNode(&base.Node{
		Name:   "Empty",
		Origin: "raw",
		Cfg:    base.NewConfig(yakNode.origin.Cfg),
		Ctx:    yakNode.origin.Ctx,
	}, operator)
	err := appendNode(node, unknownNode.origin)
	if err != nil {
		panic(err)
	}
	if name != "" {
		utils.GetLastElement(node.Children).Name = name
	}
	return convertOperatorNode(utils.GetLastElement(node.Children), operator)
}
func (yakNode *operatorNode) SetMaxLength(l uint64, uints ...string) {
	node := yakNode.origin

	n := getMulti(yakNode.origin, uints...)
	node.Cfg.SetItem(CfgLength, l*uint64(n))
}
func (yakNode *operatorNode) ProcessByType(datas ...any) any {
	operator := yakNode.operator
	getRootNode := yakNode.getRootNode

	var typeName, nodeName string
	switch len(datas) {
	case 1:
		typeName = utils.InterfaceToString(datas[0])
		nodeName = typeName
	case 2:
		typeName = utils.InterfaceToString(datas[0])
		nodeName = utils.InterfaceToString(datas[1])
	default:
		panic("invalid args")
	}
	typeNode := getRootNode(typeName)
	yakNode.AppendNode(typeNode)
	target := utils.GetLastElement(yakNode.origin.Children)
	target.Name = nodeName
	return convertOperatorNode(target, operator).Process()
}
func (yakNode *operatorNode) SetChildren(nodes []*operatorNode) {
	yakNode.origin.Children = nil
	for _, node := range nodes {
		yakNode.origin.Children = append(yakNode.origin.Children, node.origin)
	}
}
func (yakNode *operatorNode) GetChildren() []*operatorNode {
	operator := yakNode.operator

	res := []*operatorNode{}
	for _, node := range yakNode.origin.Children {
		res = append(res, convertOperatorNode(node, operator))
	}
	return res
}
func (yakNode *operatorNode) GetCfg(k string) any {
	node := yakNode.origin

	return node.Cfg.GetItem(k)
}
func (yakNode *operatorNode) Result() any {
	node := yakNode.origin

	res, err := node.Result()
	if err != nil {
		panic(err)
	}
	return res
}
func (yakNode *operatorNode) NewElement() *operatorNode {
	node := yakNode.origin
	operator := yakNode.operator

	element, err := ListNodeNewElement(node)
	if err != nil {
		panic(err)
	}
	return convertOperatorNode(element, operator)
}
func (yakNode *operatorNode) TryProcessByType(datas ...string) (result any, response map[string]any) {
	operator := yakNode.operator
	getRootNode := yakNode.getRootNode

	var typeName, nodeName string
	switch len(datas) {
	case 1:
		typeName = utils.InterfaceToString(datas[0])
		nodeName = typeName
	case 2:
		typeName = utils.InterfaceToString(datas[0])
		nodeName = utils.InterfaceToString(datas[1])
	default:
		panic("invalid args")
	}
	typeNode := getRootNode(typeName)
	response = map[string]any{
		"OK":       false,
		"Message":  "",
		"Save":     func() {},
		"Recovery": func() {},
	}
	defer func() {
		if e := recover(); e != nil {
			response["Message"] = fmt.Sprintf("%v", e)
		}
	}()

	yakNode.AppendNode(typeNode)
	copyNode := yakNode.origin.Children[len(yakNode.origin.Children)-1]
	if nodeName != "" {
		copyNode.Name = nodeName
	}
	copyYakNode := convertOperatorNode(copyNode, operator)

	deferFun, err := operator(copyNode)
	response["Save"] = func() {
		deferFun(false)
	}
	response["GetNode"] = func() any {
		return copyYakNode
	}
	response["Recovery"] = func() {
		deferFun(true)
		yakNode.origin.Children = yakNode.origin.Children[:len(yakNode.origin.Children)-1]
	}
	if err != nil {
		response["Message"] = err.Error()
		return nil, response
	}

	result = copyYakNode.Result()
	response["Result"] = result
	response["OK"] = true
	return result, response
}
func (yakNode *operatorNode) Process() any {
	node := yakNode.origin
	operator := yakNode.operator

	deferFun, err := operator(node)
	if err != nil {
		if deferFun != nil {
			deferFun(true)
		}
		panic(err)
	}
	deferFun(false)
	return yakNode.Result()
}
func (yakNode *operatorNode) ForEachChild(f func(child *operatorNode)) {
	node := yakNode.origin
	operator := yakNode.operator

	for _, child := range node.Children {
		f(convertOperatorNode(child, operator))
	}
}
func (yakNode *operatorNode) GetParent() *operatorNode {
	node := yakNode.origin
	operator := yakNode.operator

	if node.Cfg.Has(CfgParent) {
		parent := node.Cfg.GetItem(CfgParent).(*base.Node)
		return convertOperatorNode(parent, operator)
	}
	return nil
}
func (yakNode *operatorNode) GetSubNode(name string) *operatorNode {
	node := yakNode.origin
	operator := yakNode.operator

	for _, child := range node.Children {
		if child.Name == name {
			return convertOperatorNode(child, operator)
		}
	}
	panic(spew.Sprintf("node %s not found", name))
}
func (yakNode *operatorNode) SetCfg(k string, v any) {
	node := yakNode.origin

	node.Cfg.SetItem(k, v)
}
func (yakNode *operatorNode) NewSubNode(datas ...any) *operatorNode {
	node := yakNode.origin
	operator := yakNode.operator
	getRootNode := yakNode.getRootNode

	var typeName, nodeName string
	switch len(datas) {
	case 1:
		typeName = utils.InterfaceToString(datas[0])
		nodeName = typeName
	case 2:
		typeName = utils.InterfaceToString(datas[0])
		nodeName = utils.InterfaceToString(datas[1])
	default:
		panic("invalid args")
	}
	typeNode := getRootNode(typeName)
	err := appendNode(node, typeNode.origin)
	if err != nil {
		panic(err)
	}
	utils.GetLastElement(node.Children).Name = nodeName
	return convertOperatorNode(utils.GetLastElement(node.Children), operator)
}
func (yakNode *operatorNode) AppendNode(d *operatorNode) {
	node := yakNode.origin

	err := appendNode(node, d.origin)
	if err != nil {
		panic(err)
	}
}
func (yakNode *operatorNode) GetRemainingSpace() uint64 {
	node := yakNode.origin

	res, err := getNodeLength(yakNode.origin)
	if err != nil {
		panic(err)
	}
	return res / getMulti(node)
}
func (yakNode *operatorNode) Length(uints ...string) uint64 {
	n := getMulti(yakNode.origin, uints...)
	return CalcNodeConsumedLength(yakNode.origin) / uint64(n)
}
