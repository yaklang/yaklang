// Package orderedyaml provides the small ordered-map surface that yaml.v2's
// MapSlice offered, backed by yaml.v3 nodes.
package orderedyaml

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

type MapItem struct {
	Key   any
	Value any
}

type MapSlice []MapItem

func Marshal(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func Unmarshal(data []byte, value any) error {
	return yaml.Unmarshal(data, value)
}

func (m *MapSlice) UnmarshalYAML(node *yaml.Node) error {
	value, err := decodeNode(node)
	if err != nil {
		return err
	}
	ordered, ok := value.(MapSlice)
	if !ok {
		return fmt.Errorf("expected YAML mapping, got %s", node.ShortTag())
	}
	*m = ordered
	return nil
}

func (m MapSlice) MarshalYAML() (any, error) {
	return encodeNode(m)
}

func decodeNode(node *yaml.Node) (any, error) {
	if node == nil {
		return nil, nil
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil, nil
		}
		return decodeNode(node.Content[0])
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return nil, fmt.Errorf("invalid YAML mapping with %d nodes", len(node.Content))
		}
		result := make(MapSlice, 0, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key, err := decodeNode(node.Content[i])
			if err != nil {
				return nil, err
			}
			value, err := decodeNode(node.Content[i+1])
			if err != nil {
				return nil, err
			}
			result = append(result, MapItem{Key: key, Value: value})
		}
		return result, nil
	case yaml.SequenceNode:
		result := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := decodeNode(child)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		return result, nil
	case yaml.AliasNode:
		return decodeNode(node.Alias)
	default:
		var value any
		if err := node.Decode(&value); err != nil {
			return nil, err
		}
		return value, nil
	}
}

func encodeNode(value any) (*yaml.Node, error) {
	switch ordered := value.(type) {
	case MapSlice:
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, item := range ordered {
			key, err := encodeNode(item.Key)
			if err != nil {
				return nil, err
			}
			child, err := encodeNode(item.Value)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, key, child)
		}
		return node, nil
	case *MapSlice:
		if ordered == nil {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null"}, nil
		}
		return encodeNode(*ordered)
	default:
		node := &yaml.Node{}
		if err := node.Encode(value); err != nil {
			return nil, err
		}
		return node, nil
	}
}
