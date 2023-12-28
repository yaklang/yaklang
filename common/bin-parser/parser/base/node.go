package base

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v2"
	"strconv"
	"strings"
	"unicode"
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
	CfgOptionFuns = "options functions"
)

type NodeValue struct {
	Origin *Node
	Name      string
	Value     any
	ListValue bool
	AppendSub func(value *NodeValue) error
}

func (n *NodeValue) IsList() bool {
	return n.ListValue
}
func (n *NodeValue) IsStruct() bool {
	_, ok := n.Value.([]*NodeValue)
	return !n.ListValue && ok
}
func (n *NodeValue) IsValue() bool {
	return !n.ListValue && !n.IsStruct()
}
func (n *NodeValue) Child(name string) *NodeValue {
	for _, child := range n.Children() {
		if child.Name == name {
			return child
		}
	}
	return nil
}
func (n *NodeValue) Children() []*NodeValue {
	v, ok := n.Value.([]*NodeValue)
	if ok {
		return v
	}
	return nil
}

type BaseKV struct {
	data map[string]any
}

func (c *BaseKV) DeleteItem(k string) {
	delete(c.data, k)
}

func (c *BaseKV) SetItem(k string, v any) {
	c.data[k] = v
}
func (c *BaseKV) GetString(k string) string {
	v, ok := c.data[k]
	if ok {
		if v1, ok := v.(string); ok {
			return v1
		}
	}
	return ""
}
func (c *BaseKV) GetItem(k string) any {
	return c.data[k]
}
func (c *BaseKV) Has(k string) bool {
	_, ok := c.data[k]
	return ok
}
func (c *BaseKV) ConvertUint64(k string) uint64 {
	v, ok := c.data[k]
	if ok {
		v, err := strconv.ParseUint(utils.InterfaceToString(v), 10, 64)
		if err != nil {
			return 0
		}
		return v
	}
	return 0
}
func (c *BaseKV) GetUint64(k string) uint64 {
	v, ok := c.data[k]
	if ok {
		v1, ok := InterfaceToUint64(v)
		if ok {
			return v1
		}
	}
	return 0
}
func (c *BaseKV) GetBool(k string) bool {
	v, ok := c.data[k]
	if ok {
		if v1, ok := v.(bool); ok {
			return v1
		}
	}
	return false
}

type Config struct {
	BaseKV
}

func NewEmptyConfig() *Config {
	return &Config{
		BaseKV: BaseKV{
			make(map[string]any),
		},
	}
}
func (c *Config) SetItem(k string, v any) {
	c.BaseKV.SetItem(k, v)
	if c.Has(CfgOptionFuns) {
		newOptions := append(c.GetItem(CfgOptionFuns).([]NodeConfigFun), func(config *Config) {
			config.SetItem(k, v)
		})
		c.data[CfgOptionFuns] = newOptions
	} else {
		c.data[CfgOptionFuns] = []NodeConfigFun{func(config *Config) {
			config.SetItem(k, v)
		}}
	}
}
func AppendConfig(parent, config *Config) *Config {
	res := CopyConfig(parent)
	if config.Has(CfgOptionFuns) {
		for _, opt := range config.GetItem(CfgOptionFuns).([]NodeConfigFun) {
			opt(res)
		}
	}
	return res
}
func CopyConfig(config *Config) *Config {
	res := NewEmptyConfig()
	for k, v := range config.data {
		res.SetItem(k, v)
	}
	return res
}
func NewConfig(config *Config) *Config {
	res := &Config{
		BaseKV: BaseKV{
			make(map[string]any),
		},
	}
	copeFields := []string{"endian", "parser", "unit"}
	for _, field := range copeFields {
		if config.Has(field) {
			res.SetItem(field, config.GetItem(field))
		}
	}
	return res
}

type NodeContext struct {
	BaseKV
}

type Node struct {
	Name     string
	Origin   any
	Children []*Node
	Cfg      *Config
	Ctx      *NodeContext
}

func (n *Node) Result() (*NodeValue, error) {
	parser, err := n.getParser()
	if err != nil {
		return nil, err
	}
	return parser.Result(n)
}

func (n *Node) Copy() *Node {
	res := &Node{
		Name:     n.Name,
		Origin:   n.Origin,
		Children: make([]*Node, 0),
		Cfg:      CopyConfig(n.Cfg),
		Ctx:      n.Ctx,
	}
	for _, child := range n.Children {
		child.Cfg.SetItem(CfgParent, res)
		res.Children = append(res.Children, child.Copy())
	}
	return res
}

func (n *Node) getParser() (Parser, error) {
	parserName := n.Cfg.GetItem("parser")
	if parserName == nil {
		return nil, errors.New("not set parser")
	}
	parser, ok := parseMap[utils.InterfaceToString(parserName)]
	if !ok {
		return nil, fmt.Errorf("parser %s not found", parserName)
	}
	return parser, nil
}
func (n *Node) GenerateSubNode(data any, path string) error {
	parser, err := n.getParser()
	if err != nil {
		return err
	}
	if n.Cfg.GetItem("isRoot") != nil {
		err := parser.OnRoot(n)
		if err != nil {
			return fmt.Errorf("on root error: %w", err)
		}
	}
	irootNodeMap := n.Ctx.GetItem(CfgRootMap)
	if irootNodeMap == nil {
		return errors.New("not set rootNodeMap")
	}
	rootNodeMap, ok := irootNodeMap.(map[string]*Node)
	if !ok {
		return errors.New("rootNodeMap type error")
	}
	if packageNode, ok := rootNodeMap["Package"]; !ok {
		return errors.New("package node not found")
	} else {
		node := GetNodeByPath(packageNode, path)
		if node == nil {
			return fmt.Errorf("node %s not found", path)
		}
		node.Cfg.SetItem("temp root", true)
		return node.Generate(data)
	}
}
func (n *Node) Generate(data any) error {
	parser, err := n.getParser()
	if err != nil {
		return err
	}
	if n.Cfg.GetItem("isRoot") != nil {
		err := parser.OnRoot(n)
		if err != nil {
			return fmt.Errorf("on root error: %w", err)
		}
	}
	err = parser.Generate(data, n)
	if err != nil {
		return fmt.Errorf("parse node %s error: %w", n.Name, err)
	}
	return nil
}
func (n *Node) ParseSubNode(data *BitReader, path string) error {
	parser, err := n.getParser()
	if err != nil {
		return err
	}
	if n.Cfg.GetItem("isRoot") != nil {
		err := parser.OnRoot(n)
		if err != nil {
			return fmt.Errorf("on root error: %w", err)
		}
	} else {
		return errors.New("not root node")
	}

	irootNodeMap := n.Ctx.GetItem(CfgRootMap)
	if irootNodeMap == nil {
		return errors.New("not set rootNodeMap")
	}
	rootNodeMap, ok := irootNodeMap.(map[string]*Node)
	if !ok {
		return errors.New("rootNodeMap type error")
	}
	if packageNode, ok := rootNodeMap["Package"]; !ok {
		return errors.New("package node not found")
	} else {
		node := GetNodeByPath(packageNode, path)
		if node == nil {
			return fmt.Errorf("node %s not found", path)
		}
		return node.Parse(data)
	}
}
func (n *Node) Parse(reader *BitReader) error {
	parser, err := n.getParser()
	if err != nil {
		return err
	}
	if n.Cfg.GetItem("isRoot") != nil {
		err := parser.OnRoot(n)
		if err != nil {
			return fmt.Errorf("on root error: %w", err)
		}
	}
	err = parser.Parse(reader, n)
	if err != nil {
		return fmt.Errorf("parse node %s error: %w", n.Name, err)
	}
	return nil
}

// NewEmptyNode 默认初始化一个空节点
func NewEmptyNode(name string, d any, cfg *Config, ctx *NodeContext) *Node {
	node := &Node{
		Cfg:    cfg,
		Name:   name,
		Origin: d,
		Ctx:    ctx,
	}
	return node
}

//	func (n *Node) SetParentResultWriterByNode(node *Node) {
//		nodeRes := node.Cfg.GetItem("resultWriter").(*NodeResult)
//		n.Cfg.SetItem("parentResultWriter", func(d []byte, length uint64) error {
//			return nodeRes.Write(d, length)
//		})
//	}
func (n *Node) AppendNode(node *Node) error {
	newNode, err := newNodeTree(n.Cfg, node.Name, node.Origin, n.Ctx)
	if err != nil {
		return err
	}
	n.Children = append(n.Children, newNode)
	var setParent func(parent, node *Node)
	setParent = func(parent, node *Node) {
		node.Cfg.SetItem(CfgParent, parent)
		for _, child := range node.Children {
			setParent(node, child)
		}
	}
	setParent(n, newNode)
	return nil
}
func NewNodeTree(d yaml.MapSlice) (*Node, error) {
	defaultConfig := &Config{
		BaseKV: BaseKV{
			make(map[string]any),
		},
	}
	defaultConfig.SetItem("endian", "big")
	defaultConfig.SetItem("parser", "default")
	ctx := &NodeContext{
		BaseKV: BaseKV{
			make(map[string]any),
		},
	}
	root, err := newNodeTree(defaultConfig, "root", d, ctx)
	if err != nil {
		return nil, err
	}
	ctx.SetItem("root", root)
	root.Cfg.SetItem("isRoot", true)
	return root, nil
}
func NewNodeTreeWithConfig(parentCfg *Config, name string, data any, ctx *NodeContext) (*Node, error) {
	return newNodeTree(parentCfg, name, data, ctx)
}
func NewChildNodeTree(parent *Node, name string, data any, ctx *NodeContext) (*Node, error) {
	return newNodeTree(parent.Cfg, name, data, ctx)
}
func newNodeTree(parentCfg *Config, name string, data any, ctx *NodeContext) (*Node, error) {
	cfg := NewConfig(parentCfg)
	switch ret := data.(type) {
	case yaml.MapSlice:
		node := NewEmptyNode(name, data, cfg, ctx)
		for _, item := range ret {
			keyStr := utils.InterfaceToString(item.Key)
			if len(keyStr) > 0 && unicode.IsLower(rune(keyStr[0])) { // 小写开头是 config字段
				cfg.SetItem(keyStr, item.Value)
				continue
			}
			childNode, err := newNodeTree(cfg, keyStr, item.Value, ctx)
			if err != nil {
				return nil, err
			}
			node.Children = append(node.Children, childNode)
			if name == "Package" {
				childNode.Cfg.SetItem("package-child", true)
			}
		}
		if len(node.Children) == 0 {
			node.Cfg.SetItem(CfgIsTerminal, true)
		}
		return node, nil
	case string:
		node := NewEmptyNode(name, data, cfg, ctx)
		node.Cfg = cfg
		node.Cfg.SetItem(CfgIsTerminal, true)
		nodeData := utils.InterfaceToString(node.Origin)
		options := strings.Split(nodeData, ";")
		for _, option := range options {
			kvs := strings.Split(option, ":")
			if len(kvs) == 1 {
				splits := strings.Split(nodeData, ",")
				var typeName string
				if len(splits) == 0 {
					return nil, utils.Errorf("terminal node %s has no type", node.Name)
				} else if len(splits) == 1 {
					typeName = splits[0]
				} else if len(splits) >= 2 {
					typeName = splits[0]
					splits[1] = strings.Join(splits[1:], ",")
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
						return nil, fmt.Errorf("terminal node %s parse length error: %w", node.Name, err)
					}
					if isBit {
						node.Cfg.SetItem(CfgLength, l)
					} else {
						node.Cfg.SetItem(CfgLength, l*8)
					}
				}
				cfgTypeName := ""
				if strings.HasSuffix(typeName, "...") {
					cfgTypeName = strings.TrimSuffix(typeName, "...")
					node.Cfg.SetItem(CfgIsList, true)
				} else {
					cfgTypeName = typeName
				}
				node.Cfg.SetItem(CfgType, cfgTypeName)

			} else {
				kvs[1] = strings.Join(kvs[1:], ":")
				switch kvs[0] {
				case CfgDel:
					node.Cfg.SetItem(CfgDelimiter, kvs[1])
				default:
					node.Cfg.SetItem(kvs[0], kvs[1])
				}
			}
		}
		return node, nil
	default:
		return nil, fmt.Errorf("unknown type %T", data)
	}
}
