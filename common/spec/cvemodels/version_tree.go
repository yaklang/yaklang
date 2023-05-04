package cvemodels

import (
	"fmt"
	"github.com/pkg/errors"
	"sort"
)

type VersionTreeNodeIf interface {
	GetChild(a byte) (*VersionCharNode, error)
	AddChild(a *VersionCharNode)
	GetChildren() []*VersionCharNode
	GetParent() VersionTreeNodeIf
	IsRoot() bool
}

type VersionTree struct {
	VersionTreeNodeIf

	//nodes []*VersionCharNode
	prefix   string
	origin   []string
	children []*VersionCharNode
}

func (v *VersionTree) IsRoot() bool {
	return true
}

func (v *VersionTree) GetParent() VersionTreeNodeIf {
	return nil
}

func (v *VersionTree) GetChild(a byte) (*VersionCharNode, error) {
	for _, c := range v.children {
		if c.value == a {
			return c, nil
		}
	}
	return nil, errors.Errorf("no existed child for %#v", a)
}

func (v *VersionTree) AddChild(a *VersionCharNode) {
	v.children = append(v.children, a)
	a.parent = nil
}

func (v *VersionTree) GetChildren() []*VersionCharNode {
	return v.children[:]
}

type VersionCharNode struct {
	VersionTreeNodeIf
	value byte

	parent   VersionTreeNodeIf
	children []*VersionCharNode

	showAsNode bool
}

func (v *VersionCharNode) GetChild(a byte) (*VersionCharNode, error) {
	for _, c := range v.children {
		if c.value == a {
			return c, nil
		}
	}
	return nil, errors.Errorf("no existed child for %#v", a)
}

func (v *VersionCharNode) AddChild(a *VersionCharNode) {
	v.children = append(v.children, a)
	a.parent = v
}

func (v *VersionCharNode) IsRoot() bool {
	return false
}

func (v *VersionCharNode) GetChildren() []*VersionCharNode {
	return v.children[:]
}

func (v *VersionCharNode) GetParent() VersionTreeNodeIf {
	return v.parent
}

func (v *VersionCharNode) IsLeaf() bool {
	return len(v.children) <= 0
}

func (v *VersionCharNode) NextString() []string {
	if v.IsLeaf() {
		return []string{fmt.Sprintf("%v", string(v.value))}
	}

	var results []string

	var sub []string
	for _, c := range v.children {
		if c.IsLeaf() {
			sub = append(sub, string(c.value))
		}

		if !c.IsLeaf() && c.showAsNode {
			sub = append(sub, string(c.value))
		}
	}

	compactChars := func(sl []string) string {
		sort.Strings(sl)
		if len(sl) > 1 {
			return fmt.Sprintf("%v-%v", sl[0], sl[len(sl)-1])
		} else if len(sl) == 1 {
			return fmt.Sprintf("%v", sl[0])
		} else {
			return "*"
		}
	}

	var buf string
	if len(sub) > 1 {
		buf = fmt.Sprintf("%v[%v]", string(v.value), compactChars(sub))
	} else {
		buf = fmt.Sprintf("%v%v", string(v.value), compactChars(sub))
	}

	results = append(results, buf)
	return results
}

func (v *VersionCharNode) PathString() string {
	var buf string

	var current VersionTreeNodeIf = v
	for {
		parent, ok := current.GetParent().(*VersionCharNode)
		if !ok {
			break
		}

		buf = string(parent.value) + buf
		current = parent
	}

	return buf
}

func (v *VersionCharNode) HaveLeaf() bool {
	for _, i := range v.children {
		if i.IsLeaf() {
			return true
		}
	}
	return false
}

func (v *VersionCharNode) Versions() []string {
	var (
		haveLeaf []*VersionCharNode
	)
	v.walk(func(n *VersionCharNode) {
		if n.HaveLeaf() {
			haveLeaf = append(haveLeaf, n)
			return
		}
	})

	var s []string
	for _, y := range haveLeaf {
		for _, nextVersion := range y.NextString() {
			s = append(s, fmt.Sprintf("%v%v", y.PathString(), nextVersion))
		}
	}
	return s
}

func (v *VersionCharNode) walk(h func(n *VersionCharNode)) {
	if v.IsLeaf() {
		h(v)
		return
	} else {
		for _, c := range v.children {
			h(c)
			c.walk(h)
		}
	}
}

func (v *VersionTree) init() {
	var parent VersionTreeNodeIf
	var current *VersionCharNode
	for _, ver := range v.origin {
		parent = v
		for _, c := range []byte(ver) {
			if node, err := parent.GetChild(c); err != nil {
				p := &VersionCharNode{
					value:  c,
					parent: parent,
				}
				parent.AddChild(p)
				current = p
				//v.nodes = append(v.nodes, p)
				parent = p
			} else {
				parent = node
			}
		}
		if current != nil {
			current.showAsNode = true
		}
	}
}

func NewVersionTree(prefix string, version ...string) *VersionTree {
	tree := &VersionTree{
		origin: version,
		prefix: prefix,
	}
	tree.init()

	return tree
}

func (v *VersionTree) Strings() []string {
	var s []string
	for _, c := range v.children {
		if c.IsLeaf() {
			s = append(s, fmt.Sprintf("%v%v", v.prefix, c.PathString()+string(c.value)))
		} else {
			for _, version := range c.Versions() {
				s = append(s, fmt.Sprintf("%v%v", v.prefix, version))
			}
		}
	}
	return s
}
