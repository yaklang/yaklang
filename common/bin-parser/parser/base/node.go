package base

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v2"
	"math"
	"unicode"
)

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

func NewConfig(config *Config) *Config {
	res := &Config{
		BaseKV: BaseKV{
			make(map[string]any),
		},
	}
	copeFields := []string{"endian", "parser"}
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
type NodeResult struct {
	Length         uint64
	IsTerminalData bool
	TerminalData   any
	Struct         *Node
	Children       []*NodeResult
	Buffer         *bytes.Buffer
	writer         *BitWriter
}

func NewNodeResultByNode(node *Node) *NodeResult {
	buf := bytes.NewBuffer(nil)
	return &NodeResult{
		Struct:         node,
		Buffer:         buf,
		IsTerminalData: node.Cfg.GetBool("isTerminal"),
		writer:         NewBitWriter(buf),
	}
}
func (n *NodeResult) Bytes() []byte {
	res := n.Buffer.Bytes()
	length := n.Struct.Cfg.GetUint64("length")
	if len(res) < int(length/8) {
		res = append(res, make([]byte, int(length/8)-len(res))...)
	}
	return res
}
func (n *NodeResult) AppendChildren(children ...*NodeResult) error {
	for _, child := range children {
		err := n.AppendChild(child)
		if err != nil {
			return err
		}
	}
	return nil
}
func (n *NodeResult) Write(data []byte, length uint64) error {
	n1 := int(math.Ceil(float64(length) / 8))
	if len(data) < n1 {
		data = append(data, make([]byte, n1-len(data))...)
	}
	err := n.writer.WriteBits(data, length)
	if err != nil {
		return fmt.Errorf("append child error: %w", err)
	}
	return nil
}
func (n *NodeResult) AppendChild(child *NodeResult) error {
	n.Children = append(n.Children, child)
	n.Length += child.Length
	err := n.Write(child.Bytes(), child.Length)
	if err != nil {
		return fmt.Errorf("append child error: %w", err)
	}
	return nil
}
func (n *Node) Generate(data any) (*NodeResult, error) {
	parserName := n.Cfg.GetItem("parser")
	if parserName == nil {
		return nil, errors.New("not set parser")
	}
	parser, ok := parseMap[utils.InterfaceToString(parserName)]
	if !ok {
		return nil, fmt.Errorf("parser %s not found", parserName)
	}
	if n.Cfg.GetItem("isRoot") != nil {
		err := parser.OnRoot(n)
		if err != nil {
			return nil, fmt.Errorf("on root error: %w", err)
		}
	}
	res, err := parser.Generate(data, n)
	if err != nil {
		return nil, fmt.Errorf("parse node %s error: %w", n.Name, err)
	}
	return res, nil
}
func (n *Node) Parse(reader *BitReader) (*NodeResult, error) {
	parserName := n.Cfg.GetItem("parser")
	if parserName == nil {
		return nil, errors.New("not set parser")
	}
	parser, ok := parseMap[utils.InterfaceToString(parserName)]
	if !ok {
		return nil, fmt.Errorf("parser %s not found", parserName)
	}
	if n.Cfg.GetItem("isRoot") != nil {
		err := parser.OnRoot(n)
		if err != nil {
			return nil, fmt.Errorf("on root error: %w", err)
		}
	}
	res, err := parser.Parse(reader, n)
	if err != nil {
		return nil, fmt.Errorf("parse node %s error: %w", n.Name, err)
	}
	return res, nil
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
		}
		return node, nil
	case string:
		node := NewEmptyNode(name, data, cfg, ctx)
		node.Cfg = cfg
		return node, nil
	default:
		return nil, errors.New("invalid data")
	}
}
