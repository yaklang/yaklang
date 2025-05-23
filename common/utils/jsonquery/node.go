package jsonquery

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"sort"
	"strings"
)

// A NodeType is the type of a Node.
type NodeType uint

const (
	// DocumentNode is a document object that, as the root of the document tree,
	// provides access to the entire XML document.
	DocumentNode NodeType = iota
	// ElementNode is an element.
	ElementNode
	// TextNode is the text content of a node.
	TextNode
)

// A Node consists of a NodeType and some Data (tag name for
// element nodes, content for text) and are part of a tree of Nodes.
type Node struct {
	Parent, PrevSibling, NextSibling, FirstChild, LastChild *Node

	Type NodeType
	Data string

	level int
	value interface{}
}

// Gets the JSON object value.
func (n *Node) Value() interface{} {
	return n.value
}

// ChildNodes gets all child nodes of the node.
func (n *Node) ChildNodes() []*Node {
	var a []*Node
	for nn := n.FirstChild; nn != nil; nn = nn.NextSibling {
		a = append(a, nn)
	}
	return a
}

// InnerText will gets the value of the node and all its child nodes.
//
// Deprecated: Use Value() to get JSON object value.
func (n *Node) InnerText() string {
	var output func(*strings.Builder, *Node)
	output = func(b *strings.Builder, n *Node) {
		if n.Type == TextNode {
			b.WriteString(fmt.Sprintf("%v", n.value))
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			output(b, child)
		}
	}
	var b strings.Builder
	output(&b, n)
	return b.String()
}

func outputXML(b *strings.Builder, n *Node, level int, skip bool) {
	level++
	if n.Type == TextNode {
		b.WriteString(fmt.Sprintf("%v", n.value))
		return
	}
	if v := reflect.ValueOf(n.value); v.Kind() == reflect.Slice {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			b.WriteString("<" + n.Data + ">")
			outputXML(b, child, level, true)
			b.WriteString("</" + n.Data + ">")
		}
	} else {
		d := n.Data
		if !skip {
			if d == "" {
				if v := reflect.ValueOf(n.value); v.Kind() == reflect.Map {
					d = "element"
				} else {
					d = fmt.Sprintf("%v", n.value)
				}
			}
			b.WriteString("<" + d + ">")
		}
		if reflect.TypeOf(n.value) != nil {
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				outputXML(b, child, level, false)
			}
		}
		if !skip {
			b.WriteString("</" + d + ">")
		}
	}
}

// OutputXML prints the XML string.
func (n *Node) OutputXML() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	b.WriteString("<root>")
	level := 0
	for n := n.FirstChild; n != nil; n = n.NextSibling {
		outputXML(&b, n, level, false)
	}
	b.WriteString("</root>")
	return b.String()
}

// SelectElement finds the first of child elements with the
// specified name.
func (n *Node) SelectElement(name string) *Node {
	for nn := n.FirstChild; nn != nil; nn = nn.NextSibling {
		if nn.Data == name {
			return nn
		}
	}
	return nil
}

// LoadURL loads the JSON document from the specified URL.
func LoadURL(url string) (*Node, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return Parse(resp.Body)
}

func parseValue(x interface{}, top *Node, level int) {
	addNode := func(n *Node) {
		if n.level == top.level {
			top.NextSibling = n
			n.PrevSibling = top
			n.Parent = top.Parent
			if top.Parent != nil {
				top.Parent.LastChild = n
			}
		} else if n.level > top.level {
			n.Parent = top
			if top.FirstChild == nil {
				top.FirstChild = n
				top.LastChild = n
			} else {
				t := top.LastChild
				t.NextSibling = n
				n.PrevSibling = t
				top.LastChild = n
			}
		}
	}
	switch v := x.(type) {
	case []interface{}:
		// JSON array
		for _, vv := range v {
			n := &Node{Type: ElementNode, level: level, value: vv}
			addNode(n)
			parseValue(vv, n, level+1)
		}
	case map[string]interface{}:
		// JSON object
		// The Go’s map iteration order is random.
		// (https://blog.golang.org/go-maps-in-action#Iteration-order)
		var keys []string
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			n := &Node{Data: key, Type: ElementNode, level: level, value: v[key]}
			addNode(n)
			parseValue(v[key], n, level+1)
		}
	default:
		// JSON types: string, number, boolean
		n := &Node{Data: fmt.Sprintf("%v", v), Type: TextNode, level: level, value: fmt.Sprintf("%v", v)}
		addNode(n)
	}
}

func parse(b []byte) (*Node, error) {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	doc := &Node{Type: DocumentNode}
	parseValue(v, doc, 1)
	return doc, nil
}

// Parse JSON document.
func Parse(r io.Reader) (*Node, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return parse(b)
}
