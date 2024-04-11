package utils

import (
	"path/filepath"
	"strings"
)

type PathForest struct {
	roots map[string]*PathNode
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
}

func (p *PathNode) recursive(f func(node2 *PathNode)) {
	for _, n := range p.Children {
		f(n)
		n.recursive(f)
	}
}

func (p *PathForest) addPath(path string) error {
	l := StringArrayFilterEmpty(strings.Split(path, "/"))
	if len(l) <= 0 {
		_, err := p.getRootNodeOrCreate("")
		if err != nil {
			return err
		}
		return nil
	}

	rootNode, err := p.getRootNodeOrCreate(l[0])
	if err != nil {
		return err
	}

	if len(l) == 1 {
		rootNode.RelativePaths = append(rootNode.RelativePaths, path)
		return nil
	}

	node, err := rootNode.getNodeOrCreate(path)
	if err != nil {
		return err
	}
	node.RelativePaths = append(node.RelativePaths, path)
	return nil
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
	root := &PathNode{
		Path:        path,
		Name:        s,
		childrenMap: make(map[string]*PathNode),
	}
	w.roots[s] = root
	return root, nil
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
	}

	return lastNode, nil
}

func (p *PathNode) getOrCreateChildByNodeName(nodeName, path string) *PathNode {
	n, ok := p.childrenMap[nodeName]
	if ok {
		return n
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

func (n *PathNode) GetDepth() int {
	if n.Parent == nil {
		return 0
	}
	return n.Parent.GetDepth() + 1
}
