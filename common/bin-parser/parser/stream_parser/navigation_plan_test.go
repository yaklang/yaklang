package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"math"
	"strconv"
	"strings"
	"testing"
)

// Independent pre-optimization implementations are retained only in tests.
func TestNavigationPathMatchesLegacy(t *testing.T) {
	root := giopBridgeInlineRoot(t, "Package:\n  Group:\n    A: uint8\n    Wrapped:\n      unpack: true\n      B: uint8\n    C: uint8\n")
	require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader([]byte{1, 2, 3}))))
	group := root.Children[0].Children[0]
	group.Children[2].Name = "B"
	require.Same(t, group.Children[2], getNodeByPath(group, "B"))
	var nodes []*base.Node
	walkNode(root, func(n *base.Node) bool { nodes = append(nodes, n); return true })
	nodes = append(nodes, nil)
	for _, n := range nodes {
		for _, path := range []string{"", "/", "..", "../..", "A", "B", "C", "Wrapped/B", "../B", "@Group/B", "@Package/Group", "@", "@/", "../missing/..", "Group//B", "."} {
			var want, got *base.Node
			before := discardedOutcome(func() error { want = legacyNavigationPath(n, path); return nil })
			after := discardedOutcome(func() error { got = getNodeByPath(n, path); return nil })
			require.Equal(t, before, after, "path=%q", path)
			require.Same(t, want, got, "path=%q", path)
		}
		if n != nil {
			require.Equal(t, legacyNavigationChildren(n), GetSubNodes(n))
		}
	}
	// These public fields remain live; do not memoize resolved nodes.
	group.Children[2].Name = "Changed"
	require.Same(t, group.Children[1].Children[0], getNodeByPath(group, "B"))
	group.Children[1].Cfg.SetItem("unpack", false)
	require.Nil(t, getNodeByPath(group, "B"))
	group.Children[1].Cfg.SetItem(CfgIsRefType, true)
	require.Same(t, group.Children[1].Children[0], getNodeByPath(group, "B"))
	// A malformed later sibling must not be skipped after an earlier match.
	group.Children = append(group.Children, nil)
	want := discardedOutcome(func() error { legacyNavigationPath(group, "A"); return nil })
	got := discardedOutcome(func() error { getNodeByPath(group, "A"); return nil })
	require.Equal(t, want, got)
}

func TestNavigationLengthMatchesLegacy(t *testing.T) {
	root := giopBridgeInlineRoot(t, "Package:\n  Frame:\n    Size: uint8\n    Payload: uint8\n")
	require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader([]byte{1, 2}))))
	frame := root.Children[0].Children[0]
	node := frame.Children[1]
	check := func(n *base.Node) {
		t.Helper()
		var a, b uint64
		var aok, bok bool
		before := discardedOutcome(func() (err error) { a, aok, err = legacyNavigationLength(n); return })
		after := discardedOutcome(func() (err error) { b, bok, err = parseLengthByLengthConfig(n); return })
		require.Equal(t, before, after)
		require.Equal(t, a, b)
		require.Equal(t, aok, bok)
	}
	for _, limit := range []any{nil, uint64(0), uint64(8), uint64(24), uint64(64), uint64(math.MaxUint64)} {
		root.Cfg.SetItem(CfgLength, limit)
		for _, typ := range []any{"uint8", "int16", "raw", "missing", "string", nil} {
			node.Cfg.SetItem(CfgType, typ)
			for _, length := range []any{nil, uint64(0), uint64(8), uint64(128), int(-1), "8", false} {
				for _, cache := range []any{nil, map[string]uint64{}, map[string]uint64{node.Name: 0}, map[string]uint64{node.Name: 16}, "bad"} {
					frame.Cfg.SetItem(CfgLengthCacheMap, cache)
					if cache == nil {
						frame.Cfg.DeleteItem(CfgLengthCacheMap)
					}
					node.Cfg.SetItem(CfgLength, length)
					check(node)
					node.Cfg.DeleteItem(CfgLength)
					check(node)
				}
			}
		}
	}
	frame.Cfg.DeleteItem(CfgLengthCacheMap)
	root.Cfg.SetItem(CfgLength, uint64(128))
	node.Cfg.SetItem(CfgType, "raw")
	for _, path := range []any{"../Size", "../missing", "", nil} {
		node.Cfg.SetItem(CfgLengthFromField, path)
		for _, unit := range []any{"byte", "bit", "invalid", 12, nil} {
			node.Cfg.SetItem(CfgUnit, unit)
			for _, factor := range []any{"2", "-1", "bad", uint64(2), false, nil} {
				node.Cfg.SetItem("length-from-field-multiply", factor)
				check(node)
			}
		}
	}
	for _, parent := range []any{nil, "wrong", (*base.Node)(nil)} {
		node.Cfg.SetItem(CfgParent, parent)
		check(node)
	}
	check(root)
	root.Cfg.DeleteItem(CfgLength)
	check(root)
}

func legacyNavigationChildren(node *base.Node) []*base.Node {
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
func legacyNavigationPath(node *base.Node, key string) *base.Node {
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
			for _, child := range legacyNavigationChildren(node) {
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

func legacyNavigationLengthCache(parentNode *base.Node, childName string) (uint64, bool) {
	if parentNode.Cfg.Has(CfgLengthCacheMap) {
		return parentNode.Cfg.GetItem(CfgLengthCacheMap).(map[string]uint64)[childName], true
	}
	return 0, false
}
func legacyNavigationLength(node *base.Node) (uint64, bool, error) {
	if node.Name == "root" {
		if node.Cfg.Has(CfgLength) {
			return node.Cfg.GetUint64(CfgLength), true, nil
		}
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
	var parentLength uint64
	var parentLengthOK bool
	if v, ok := legacyNavigationLengthCache(parentNode, node.Name); ok {
		parentLength = v
		parentLengthOK = true
	} else {
		var err error
		parentLength, parentLengthOK, err = legacyNavigationLength(parentNode)
		if err != nil {
			return 0, false, fmt.Errorf("parse parent length error: %v", err)
		}
	}

	//parentRemaininigLength := uint64(0)
	var length uint64
	getLengthOK := false
	if node.Cfg.Has(CfgLength) {
		length = node.Cfg.GetUint64(CfgLength)
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
			if node.Cfg.Has(CfgLengthFromField) {
				// 从field 读取length
				if node.Cfg.Has(CfgLengthFromField) {
					fieldName := node.Cfg.GetString(CfgLengthFromField)
					target := legacyNavigationPath(node, fieldName)
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
	consumedLength := calcNodeConsumedLength
	if node.Ctx != nil && node.Ctx.GetBool("parseConsumedLengthLegacy") {
		consumedLength = calcNodeConsumedLengthLegacy
	}
	startField := parentNode.Cfg.GetString(CfgLengthForStartField)
	var startN = -1
	for i, sub := range parentNode.Children {
		if sub.Name == startField {
			startN = i
			break
		}
	}
	for i, sub := range parentNode.Children {
		if sub == node {
			break
		}
		if i < startN {
			continue
		}
		currentNodeLength += consumedLength(sub)
	}
	if currentNodeLength > parentLength {
		return 0, false, fmt.Errorf("consumed length %d exceeds parent boundary %d", currentNodeLength, parentLength)
	}
	remainingLength := parentLength - currentNodeLength
	if getLengthOK {
		if length > remainingLength {
			return 0, false, fmt.Errorf("node type %s,length %d over max size %d", node.Cfg.GetString(CfgType), length, remainingLength)
		}
		return length, true, nil
	} else {
		return remainingLength, parentLengthOK, nil
	}
}
