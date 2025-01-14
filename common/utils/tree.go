package utils

import (
	"path/filepath"
	"strings"
)

type PathForest struct {
	roots    map[string]*PathNode
	readOnly bool
}

type PathNodes []*PathNode

func (p *PathForest) Output() []*PathNode {
	var nodes []*PathNode
	for _, n := range p.roots {
		nodes = append(nodes, n)
	}
	return nodes
}

func (p *PathForest) Recursive(f func(node2 *PathNode)) {
	for _, n := range p.roots {
		f(n)
		n.recursive(f)
	}
}

type PathNode struct {
	Parent        *PathNode `json:"-"`
	Path          string    `json:"path"`
	RelativePaths []string  `json:"relative_paths"`
	Name          string    `json:"name"`
	childrenMap   map[string]*PathNode
	Children      []*PathNode `json:"children"`
	Depth         int         `json:"depth"`
	Value         any         `json:"-"`
	ReadOnly      bool        `json:"-"`
}

func (p *PathNode) recursive(f func(node2 *PathNode)) {
	for _, n := range p.Children {
		f(n)
		n.recursive(f)
	}
}

func (p *PathForest) addPath(path string, v ...any) error {
	l := StringArrayFilterEmpty(strings.Split(path, "/"))
	if len(l) <= 0 {
		_, err := p.getRootNodeOrCreate("")
		if err != nil {
			return err
		}
		return nil
	}

	var val any
	if len(v) >= 1 {
		val = v[0]
	}

	rootNode, err := p.getRootNodeOrCreate(l[0])
	if err != nil {
		return err
	}

	if len(l) == 1 {
		rootNode.Value = val
		rootNode.RelativePaths = append(rootNode.RelativePaths, path)
		return nil
	}

	node, err := rootNode.getNodeOrCreate(path)
	if err != nil {
		return err
	}
	node.Value = val
	node.RelativePaths = append(node.RelativePaths, path)
	return nil
}

func (w *PathForest) ReadOnly() {
	w.Recursive(func(node2 *PathNode) {
		node2.ReadOnly = true
	})
}

func (w *PathForest) getRootNodeOrCreate(s string) (*PathNode, error) {
	n, ok := w.roots[s]
	if ok {
		return n, nil
	}

	var path string
	if strings.HasPrefix(s, "/") {
		path = s
	} else {
		path = filepath.Join("/", s)
	}

	if w.readOnly {
		return nil, Errorf("path forest is read only")
	}

	root := &PathNode{
		Path:        path,
		Name:        s,
		childrenMap: make(map[string]*PathNode),
	}
	w.roots[s] = root
	return root, nil
}

func (p *PathNode) Existed(i string) bool {

	_, ok := p.childrenMap[i]
	return ok
}

func (p *PathNode) getNodeOrCreate(path string) (*PathNode, error) {
	blocks := StringArrayFilterEmpty(strings.Split(path, "/"))
	if len(blocks) <= 1 {
		return nil, Errorf("this is in a root path: %s", path)
	}

	var (
		buf      []string
		lastNode *PathNode = p
	)
	for i, b := range blocks {
		buf = append(buf, b)
		if i <= 0 {
			continue
		}

		lastNode = lastNode.getOrCreateChildByNodeName(b, strings.Join(buf, "/"))
		if lastNode == nil {
			return nil, Errorf("create child node failed: %s", path)
		}
	}

	return lastNode, nil
}

func (p *PathNode) getOrCreateChildByNodeName(nodeName, path string) *PathNode {
	n, ok := p.childrenMap[nodeName]
	if ok {
		return n
	}

	if p.ReadOnly {
		return nil
	}
	n = &PathNode{
		Parent:      p,
		Path:        path,
		Name:        nodeName,
		childrenMap: make(map[string]*PathNode),
	}
	p.childrenMap[nodeName] = n
	p.Children = append(p.Children, n)
	return n
}

func (p *PathNode) AllChildren() []*PathNode {
	var nodes []*PathNode
	for _, n := range p.Children {
		nodes = append(nodes, n)
		nodes = append(nodes, n.AllChildren()...)
	}
	return nodes
}

func GeneratePathTrees(l ...string) (*PathForest, error) {
	forest := &PathForest{
		roots: make(map[string]*PathNode),
	}

	for _, p := range l {
		if strings.Contains(p, "\\") {
			return nil, Errorf("error path split by '\\': %v ", p)
		}
		err := forest.addPath(p)
		if err != nil {
			return nil, Errorf("add path to path forest failed: %s", err)
		}
	}
	return forest, nil
}

func (p *PathForest) AddPath(path string, f any) error {
	return p.addPath(path, f)
}

func (p *PathForest) Get(path string) (*PathNode, error) {
	node, err := p.getRootNodeOrCreate(p.getRoot(path))
	if err != nil {
		return nil, err
	}
	_, after, _ := strings.Cut(path, "/")
	if after != "" {
		return node.getNodeOrCreate(path)
	}
	return node, nil
}

func (n *PathNode) GetDepth() int {
	if n.Parent == nil {
		return 0
	}
	return n.Parent.GetDepth() + 1
}

func (p *PathForest) getRoot(path string) string {
	before, _, _ := strings.Cut(path, "/")
	return before
}
