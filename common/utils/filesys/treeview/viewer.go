package treeview

import (
	"fmt"
	"index/suffixarray"
	"sort"
	"strings"
)

// Node 表示树节点结构
type Node struct {
	name     string
	children map[string]*Node
	isFile   bool
}

// TreeView 表示树形视图结构
type TreeView struct {
	root  *Node
	index *suffixarray.Index
	data  []byte
}

// NewTreeView 创建新的树形视图实例
func NewTreeView(paths []string) *TreeView {
	if paths == nil {
		paths = []string{}
	}

	// 创建后缀数组索引
	data := []byte(strings.Join(paths, "\n"))
	index := suffixarray.New(data)

	return &TreeView{
		root:  buildTree(paths),
		index: index,
		data:  data,
	}
}

// NewTreeViewFromString 从字符串创建树形视图实例
func NewTreeViewFromString(pathsStr string) *TreeView {
	if pathsStr == "" {
		return NewTreeView(nil)
	}
	paths := strings.Split(strings.TrimSpace(pathsStr), "\n")
	return NewTreeView(paths)
}

// newNode 创建新节点
func newNode(name string) *Node {
	return &Node{
		name:     name,
		children: make(map[string]*Node),
		isFile:   false,
	}
}

// buildTree 构建树结构
func buildTree(paths []string) *Node {
	if paths == nil {
		return newNode("")
	}

	root := newNode("")

	for _, path := range paths {
		if path == "" {
			continue
		}

		parts := strings.Split(strings.TrimSpace(path), "/")
		current := root

		for i, part := range parts {
			if part == "" {
				continue
			}
			if _, exists := current.children[part]; !exists {
				current.children[part] = newNode(part)
			}
			current = current.children[part]
			if i == len(parts)-1 {
				current.isFile = true
			}
		}
	}

	return root
}

// Print 打印树形结构
func (tv *TreeView) Print() string {
	if tv == nil || tv.root == nil {
		return ""
	}

	var builder strings.Builder
	builder.WriteString(".\n")
	tv.printNode(tv.root, "", true, &builder)
	return builder.String()
}

// printNode 打印节点（内部方法）
func (tv *TreeView) printNode(node *Node, prefix string, isLast bool, builder *strings.Builder) {
	if node == nil || builder == nil {
		return
	}

	if node.name != "" {
		if isLast {
			builder.WriteString(fmt.Sprintf("%s└── %s\n", prefix, node.name))
			prefix += "    "
		} else {
			builder.WriteString(fmt.Sprintf("%s├── %s\n", prefix, node.name))
			prefix += "│   "
		}
	}

	if node.children == nil {
		return
	}

	var keys []string
	for k := range node.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i, key := range keys {
		isLastChild := i == len(keys)-1
		tv.printNode(node.children[key], prefix, isLastChild, builder)
	}
}

// Search 搜索包含指定模式的路径
func (tv *TreeView) Search(pattern string) []string {
	if tv == nil || tv.index == nil || tv.data == nil || pattern == "" {
		return nil
	}

	matches := tv.index.Lookup([]byte(pattern), -1)
	if matches == nil {
		return nil
	}

	var results []string
	for _, pos := range matches {
		if pos < 0 || pos >= len(tv.data) {
			continue
		}

		// 找到包含该位置的完整路径
		start := pos
		for start > 0 && tv.data[start-1] != '\n' {
			start--
		}
		end := pos
		for end < len(tv.data) && tv.data[end] != '\n' {
			end++
		}

		if start < end {
			results = append(results, string(tv.data[start:end]))
		}
	}

	return results
}

// Count 返回文件和目录的数量
func (tv *TreeView) Count() (files, dirs int) {
	if tv == nil || tv.root == nil {
		return 0, 0
	}
	return tv.countNode(tv.root)
}

// countNode 计算节点数量（内部方法）
func (tv *TreeView) countNode(node *Node) (files, dirs int) {
	if node == nil {
		return 0, 0
	}

	if node.name != "" {
		if node.isFile {
			files++
		} else {
			dirs++
		}
	}

	if node.children == nil {
		return files, dirs
	}

	for _, child := range node.children {
		f, d := tv.countNode(child)
		files += f
		dirs += d
	}

	return files, dirs
}
