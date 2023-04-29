package utils

import (
	"path/filepath"
	"strings"
)

type PathForest struct {
	roots map[string]*pathNode
}

type PathNodes []*pathNode

func (p *PathForest) Output() []*pathNode {
	var nodes []*pathNode
	for _, n := range p.roots {
		nodes = append(nodes, n)
	}
	return nodes
}

type pathNode struct {
	Parent        *pathNode `json:"-"`
	Path          string    `json:"path"`
	RelativePaths []string  `json:"relative_paths"`
	Name          string    `json:"name"`
	childrenMap   map[string]*pathNode
	Children      []*pathNode `json:"children"`
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

func (w *PathForest) getRootNodeOrCreate(s string) (*pathNode, error) {
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
	root := &pathNode{
		Path:        path,
		Name:        s,
		childrenMap: make(map[string]*pathNode),
	}
	w.roots[s] = root
	return root, nil
}

func (p *pathNode) getNodeOrCreate(path string) (*pathNode, error) {
	blocks := StringArrayFilterEmpty(strings.Split(path, "/"))
	if len(blocks) <= 1 {
		return nil, Errorf("this is in a root path: %s", path)
	}

	var (
		buf      []string
		lastNode *pathNode = p
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

func (p *pathNode) getOrCreateChildByNodeName(nodeName, path string) *pathNode {
	n, ok := p.childrenMap[nodeName]
	if ok {
		return n
	}

	n = &pathNode{
		Parent:      p,
		Path:        path,
		Name:        nodeName,
		childrenMap: make(map[string]*pathNode),
	}
	p.childrenMap[nodeName] = n
	p.Children = append(p.Children, n)
	return n
}

func GeneratePathTrees(l ...string) (*PathForest, error) {
	forest := &PathForest{
		roots: make(map[string]*pathNode),
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
