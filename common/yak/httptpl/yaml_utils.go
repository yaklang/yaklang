package httptpl

import (
	"strconv"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v3"
)

func nodeGetRaw(node *yaml.Node, key string) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode {
		node = node.Content[0]
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func nodeGetFirstRaw(node *yaml.Node, keys ...string) *yaml.Node {
	for _, key := range keys {
		if ret := nodeGetRaw(node, key); ret != nil {
			return ret
		}
	}
	return nil
}

func sequenceNodeForEach(node *yaml.Node, fn func(value *yaml.Node) error) error {
	if node == nil {
		return utils.Error("node is nil")
	}
	if node.Kind != yaml.SequenceNode {
		return utils.Error("node is not node")
	}
	for _, i := range node.Content {
		if err := fn(i); err != nil {
			return err
		}
	}
	return nil
}

func mappingNodeForEach(node *yaml.Node, fn func(key string, subNode *yaml.Node) error) error {
	if node == nil {
		return utils.Error("node is nil")
	}
	if node.Kind != yaml.MappingNode {
		return utils.Error("node is not node")
	}
	for i := 0; i < len(node.Content); i += 2 {
		if err := fn(node.Content[i].Value, node.Content[i+1]); err != nil {
			return err
		}
	}
	return nil
}

func nodeGetString(node *yaml.Node, key string) string {
	innerNode := nodeGetRaw(node, key)
	if innerNode == nil {
		return ""
	}
	return innerNode.Value
}

func nodeGetBool(node *yaml.Node, key string) bool {
	value := nodeGetString(node, key)
	b, _ := strconv.ParseBool(value)
	return b
}

func nodeGetInt64(node *yaml.Node, key string) int64 {
	value := nodeGetString(node, key)
	i, _ := strconv.ParseInt(value, 10, 64)
	return i
}

func nodeGetFloat64(node *yaml.Node, key string) float64 {
	value := nodeGetString(node, key)
	i, _ := strconv.ParseFloat(value, 64)
	return i
}

func nodeGetStringSlice(node *yaml.Node, key string) []string {
	innerNode := nodeGetRaw(node, key)
	if innerNode == nil {
		return nil
	}
	contents := lo.Map(innerNode.Content, func(item *yaml.Node, index int) string {
		return item.Value
	})
	return contents
}

func nodeToStringSlice(node *yaml.Node) []string {
	if node == nil {
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil
	}
	var ret []string
	for _, i := range node.Content {
		ret = append(ret, i.Value)
	}
	return ret
}

func nodeToInt64(node *yaml.Node) int64 {
	if node == nil {
		return 0
	}
	i, _ := strconv.ParseInt(node.Value, 10, 64)
	return i
}

func nodeToBool(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	b, _ := strconv.ParseBool(node.Value)
	return b
}

func nodeGetStringSliceFallback(node *yaml.Node, keys ...string) []string {
	subNode := nodeGetFirstRaw(node, keys...)
	if subNode == nil {
		return nil
	}
	groups := nodeToStringSlice(subNode)
	if len(groups) == 0 {
		// fallback to string
		groups = append(groups, subNode.Value)
	}
	return groups
}
