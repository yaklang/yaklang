package bin_parser

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/binx"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v2"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Node struct {
	root          *Node
	nodeMap       map[string]*Node
	sliceMapIndex map[string]int
	sortKey       []string
	data          any

	Name string

	terminal bool
	cfg      Config
	reader   io.Reader
	result   any
	cbs      []func()
}

// NewEmptyNode 默认初始化一个空节点
func NewEmptyNode(name string, d any) *Node {
	node := &Node{
		nodeMap:       map[string]*Node{},
		sliceMapIndex: map[string]int{},
		Name:          name,
		cfg: Config{
			endian: binx.BigEndianByteOrder,
		},
		data: d,
	}
	return node
}
func NewNodeTree(reader io.Reader, d yaml.MapSlice) (*Node, error) {
	var emptyNodeP *Node
	node := emptyNodeP.NewSubEmptyNode("root", d)
	node.root = node
	node.data = d
	node.reader = reader
	return node, node.NewSubNodeByData()
}

// NewSubEmptyNode 自动继承父节点的一些信息，如果是SliceMap，则自动建立索引、生成配置
func (n *Node) NewSubEmptyNode(name string, data any) *Node {
	node := NewEmptyNode(name, data)
	node.Name = name
	if n != nil {
		node.root = n.root
		node.reader = n.reader
		node.cfg = n.cfg
	}
	if node.data != nil {
		switch ret := node.data.(type) {
		case yaml.MapSlice:
			opts, d := splitConfigAndData(ret)
			node.data = d
			switch ret := d.(type) {
			case yaml.MapSlice:
				for i, item := range ret {
					node.sortKey = append(node.sortKey, utils.InterfaceToString(item.Key))
					node.sliceMapIndex[utils.InterfaceToString(item.Key)] = i
				}
				for _, opt := range opts {
					opt(&node.cfg)
				}
			}
		}
	}
	return node
}
func (n *Node) NewSubNodeByData() error {
	switch ret := n.data.(type) {
	case yaml.MapSlice:
		for index, item := range ret {
			var resNode *Node
			if v, ok := n.nodeMap[utils.InterfaceToString(item.Key)]; ok {
				resNode = v
			} else {
				node := n.NewSubEmptyNode(utils.InterfaceToString(item.Key), item.Value)
				err := node.NewSubNodeByData()
				if err != nil {
					return err
				}
				n.nodeMap[utils.InterfaceToString(item.Key)] = node
				resNode = node
			}

			if resNode.cfg.autoLength {
				//var elementNode *Node
				//if resNode.cfg.isList {
				//	if len(resNode.nodeMap) != 1 {
				//		return errors.New("list node must have only one element")
				//	}
				//	elementNode = resNode.nodeMap[resNode.sortKey[0]]
				//} else {
				//	elementNode = n
				//}
				if index != len(ret)-1 {
					return errors.New("auto length node must be the last node")
				} else {
					var total uint64
					var totalNodeName string
					switch resNode.cfg.total.(type) {
					case int:
						total = uint64(resNode.cfg.total.(int))
					default:
						totalNodeName = utils.InterfaceToString(resNode.cfg.total)
						if n.nodeMap[totalNodeName] == nil {
							return fmt.Errorf("total node `%s` not found", totalNodeName)
						}
					}
					resNode.cfg.getAutoLength = func() (uint64, error) {
						if totalNodeName != "" {
							if v, ok := n.nodeMap[totalNodeName]; ok {
								v1, err := ToUint64(v.result)
								if err != nil {
									return 0, fmt.Errorf("total node `%s` type must be uint", totalNodeName)
								}
								total = v1
							} else {
								return 0, fmt.Errorf("total node `%s` not found", totalNodeName)
							}
						}
						var getLength func(node *Node) float64
						getLength = func(node *Node) float64 {
							var res float64
							if !node.IsTerminal() {
								node.ForEachNode(func(node *Node) bool {
									res += getLength(node)
									return true
								})
							} else {
								res += float64(node.cfg.length)
								if node.cfg.hasHalf {
									res += 0.5
								}
								return res
							}
							return res
						}
						var otherLength float64
						for _, node := range n.nodeMap {
							if node == resNode {
								continue
							}
							otherLength += getLength(node)
						}
						return total*4 - uint64(otherLength), nil
					}
				}
			}
		}
		return nil
	case string:
		// 类型,长度
		parse := func(params [2]string) error { // list, type, length
			options := []ConfigFunc{}
			if strings.HasSuffix(params[0], "...") {
				options = append(options, WithList(true))
				params[0] = params[0][:len(params[0])-3]
			}
			options = append(options, WithDataType(binx.BinaryTypeVerbose(params[0])))
			if params[1] == "auto" {
				options = append(options, WithAutoLength(true))
			} else {
				uintN, err := strconv.ParseUint(params[1], 10, 64)
				if err != nil {
					n, err := strconv.ParseFloat(params[1], 64)
					if err != nil {
						return errors.New("invalid length: " + params[1])
					}
					integerN := n - 0.5
					if float64(uint64(integerN)) != integerN {
						return errors.New("invalid length: " + params[1])
					}
					options = append(options, WithHasHalf(true))
					options = append(options, WithLength(uint64(integerN)))
				} else {
					options = append(options, WithLength(uintN))
				}
			}
			for _, opt := range options {
				opt(&n.cfg)
			}
			if !utils.StringArrayContains([]string{"int", "uint", "int8", "uint8", "int16", "uint16", "int32", "uint32", "int64", "uint64", "raw", "string"}, params[0]) {
				if n.root != nil {
					v, err := n.root.Get(params[0])
					if err != nil {
						return fmt.Errorf("invalid type: %w", err)
					}
					newNode := *v
					for _, opt := range options {
						opt(&newNode.cfg)
					}
					n.nodeMap[params[0]] = &newNode
					n.sortKey = append(n.sortKey, params[0])
					return nil
				}
				return errors.New("invalid type: " + params[1])
			}
			n.terminal = true
			return nil
		}
		splits := strings.Split(ret, ",")
		if len(splits) == 0 {
			return errors.New("invalid type: " + ret)
		}
		switch len(splits) {
		case 2:
			return parse([2]string{splits[0], splits[1]})
		case 1:
			var length uint64 = 0
			switch splits[0] {
			case "int":
				length = 4
			case "uint":
				length = 4
			case "int8":
				length = 1
			case "uint8":
				length = 1
			case "int16":
				length = 2
			case "uint16":
				length = 2
			case "int32":
				length = 4
			case "uint32":
				length = 4
			case "int64":
				length = 8
			case "uint64":
				length = 8
			default:
				return parse([2]string{splits[0], "auto"})
			}
			return parse([2]string{splits[0], strconv.FormatUint(length, 10)})
		default:
			return errors.New("invalid terminal node: " + ret)
		}
	default:
		return errors.New("invalid node: " + utils.InterfaceToString(n.data))
	}
}
func (n *Node) Get(key string) (*Node, error) {
	if node, ok := n.nodeMap[key]; ok {
		return node, nil
	}
	if n.data != nil {
		if index, ok := n.sliceMapIndex[key]; ok {
			newNode := n.NewSubEmptyNode(key, n.data.(yaml.MapSlice)[index].Value)
			err := newNode.NewSubNodeByData()
			if err != nil {
				return nil, fmt.Errorf("parse `%s` error: %w", key, err)
			}
			n.nodeMap[key] = newNode
			return newNode, nil
		} else {
			return nil, errors.New("key `" + key + "` not found")
		}
	}
	return nil, errors.New("key `" + key + "` not found")
}
func (n *Node) ForEachNode(f func(node *Node) bool) {
	if n.IsMap() {
		for _, v := range n.sortKey {
			if n.nodeMap[v] == nil {
				continue
			}
			if !f(n.nodeMap[v]) {
				return
			}
		}
	}
	return
}
func (n *Node) IsTerminal() bool {
	return n.terminal
}
func (n *Node) IsMap() bool {
	_, ok := n.data.(yaml.MapSlice)
	return ok
}
func (n *Node) Parse() (map[string]any, error) {
	res := map[string]any{}
	var forEachErr error
	var preByt []byte
	n.ForEachNode(func(node *Node) bool {
		if node.IsMap() {
			v, err1 := node.Parse()
			if err1 != nil {
				forEachErr = fmt.Errorf("parse `%s` error: %w", node.Name, err1)
				return false
			}
			res[node.Name] = v
		} else if node.cfg.isList {
			cfg := node.cfg
			var length uint64 = 0
			if cfg.autoLength {
				l, err := cfg.getAutoLength()
				if err != nil {
					forEachErr = fmt.Errorf("parse `%s` error: %w", node.Name, err)
					return false
				}
				length = l
			} else {
				length = cfg.length
			}
			var i uint64
			result := []any{}
			for ; i < length; i++ {
				v, err1 := node.Parse()
				if err1 != nil {
					forEachErr = fmt.Errorf("parse `%s` error: %w", node.Name, err1)
					return false
				}
				//res[node.Name] = append(res[node.Name].([]map[string]any), v)
				result = append(result, v)
			}
			node.result = result
		} else if node.IsTerminal() {
			cfg := node.cfg
			var length uint64 = 0
			if cfg.autoLength {
				l, err := cfg.getAutoLength()
				if err != nil {
					forEachErr = fmt.Errorf("parse `%s` error: %w", node.Name, err)
					return false
				}
				length = l
			} else {
				length = cfg.length
			}
			newHalf := false
			appendHalf := false
			if cfg.hasHalf {
				if len(preByt) == 0 {
					newHalf = true
				} else {
					appendHalf = true
				}
			} else {
				if len(preByt) != 0 {
					forEachErr = fmt.Errorf("parse `%s` error: %w", node.Name, errors.New(fmt.Sprintf("before %s node has a half byte, %s node must have a half byte", node.Name, node.Name)))
					return false
				}
			}
			if newHalf {
				length++
			}
			desc := binx.NewPartDescriptor(node.cfg.dataType, length)
			desc.Identifier = node.Name
			desc.ByteOrder = node.cfg.endian
			resultIf, err := binx.BinaryRead(node.reader, desc)
			if err != nil {
				forEachErr = fmt.Errorf("parse `%s` error: %w", node.Name, err)
				return false
			}
			if len(resultIf) != 1 {
				forEachErr = errors.New("result length is not 1")
				return false
			}
			resBytes := resultIf[0].GetBytes()
			if newHalf {
				if cfg.endian == binx.BigEndianByteOrder {
					b := resBytes[len(resBytes)-1]
					resBytes[len(resBytes)-1] = b >> 4
					preByt = []byte{b - resBytes[len(resBytes)-1]<<4}
				} else {
					b := resBytes[0]
					resBytes[0] = b >> 4
					preByt = []byte{b - resBytes[0]}
				}
			}
			if appendHalf {
				resBytes = append(preByt, resBytes...)
				preByt = nil
			}
			r := binx.NewResult(resBytes)
			r.Type = cfg.dataType
			res[node.Name] = r.Value()
			node.result = r.Value()
		}
		return true
	})
	if forEachErr != nil {
		return nil, forEachErr
	}
	return res, nil
}

func Parse(data io.Reader, rule string) (*Node, error) {
	ruleContent, err := os.ReadFile("./rules/" + rule + ".yaml")
	if err != nil {
		return nil, err
	}
	var ruleMap yaml.MapSlice
	err = yaml.Unmarshal(ruleContent, &ruleMap)
	if err != nil {
		return nil, err
	}
	node, err := NewNodeTree(data, ruleMap)
	node.root = node
	if err != nil {
		return nil, fmt.Errorf("parse rule error: %w", err)
	}
	packageNode, err := node.Get("package")
	if err != nil {
		return nil, fmt.Errorf("parse rule error: %w", err)
	}
	_, err = packageNode.Parse()
	if err != nil {
		return nil, fmt.Errorf("parse rule error: %w", err)
	}
	return packageNode, nil
}
func ParseBinary(data io.Reader, rule string) (*base.Node, error) {
	splits := strings.Split(rule, ".")
	paths := []string{"./rules/"}
	splits[len(splits)-1] = splits[len(splits)-1] + ".yaml"
	p := filepath.Join(append(paths, splits...)...)
	rootNode, err := parser.ParseRule(p)
	if err != nil {
		return nil, err
	}
	err = rootNode.Parse(base.NewBitReader(data))
	if err != nil {
		return nil, err
	}

	return rootNode.Children[0], nil
}

func GenerateBinary(data any, rule string) (*base.Node, error) {
	splits := strings.Split(rule, ".")
	paths := []string{"./rules/"}
	splits[len(splits)-1] = splits[len(splits)-1] + ".yaml"
	p := filepath.Join(append(paths, splits...)...)
	rootNode, err := parser.ParseRule(p)
	if err != nil {
		return nil, err
	}
	return rootNode.Children[0], rootNode.Generate(data)
}
