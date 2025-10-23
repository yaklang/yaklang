package treeview

import (
	"fmt"
	"index/suffixarray"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// Node 表示树节点结构
type Node struct {
	name     string
	children map[string]*Node
	isFile   bool
	fullPath string // 完整路径，用于获取文件信息
}

// TreeView 表示树形视图结构
type TreeView struct {
	root           *Node
	index          *suffixarray.Index
	data           []byte
	maxDepth       int                          // 最大深度，0表示无限制
	maxLines       int                          // 最大行数，0表示无限制
	collapseSingle bool                         // 是否合并单文件夹
	filesystem     filesys_interface.FileSystem // 文件系统引用，用于获取文件信息
	rootPath       string                       // 根路径
}

// NewTreeView 创建新的树形视图实例（无限制）
func NewTreeView(paths []string) *TreeView {
	return NewTreeViewWithOptions(paths, 0, 0, false)
}

// NewTreeViewWithLimits 创建带限制的树形视图实例
func NewTreeViewWithLimits(paths []string, maxDepth, maxLines int) *TreeView {
	return NewTreeViewWithOptions(paths, maxDepth, maxLines, false)
}

// NewTreeViewWithOptions 创建带完整选项的树形视图实例
func NewTreeViewWithOptions(paths []string, maxDepth, maxLines int, collapseSingle bool) *TreeView {
	if paths == nil {
		paths = []string{}
	}

	// 创建后缀数组索引
	data := []byte(strings.Join(paths, "\n"))
	index := suffixarray.New(data)

	root := buildTree(paths)
	if collapseSingle {
		root = collapseTree(root)
	}

	return &TreeView{
		root:           root,
		index:          index,
		data:           data,
		maxDepth:       maxDepth,
		maxLines:       maxLines,
		collapseSingle: collapseSingle,
		filesystem:     nil,
		rootPath:       "",
	}
}

// NewTreeViewFromString 从字符串创建树形视图实例（无限制）
func NewTreeViewFromString(pathsStr string) *TreeView {
	return NewTreeViewFromStringWithOptions(pathsStr, 0, 0, false)
}

// NewTreeViewFromStringWithLimits 从字符串创建带限制的树形视图实例
func NewTreeViewFromStringWithLimits(pathsStr string, maxDepth, maxLines int) *TreeView {
	return NewTreeViewFromStringWithOptions(pathsStr, maxDepth, maxLines, false)
}

// NewTreeViewFromStringWithOptions 从字符串创建带完整选项的树形视图实例
func NewTreeViewFromStringWithOptions(pathsStr string, maxDepth, maxLines int, collapseSingle bool) *TreeView {
	if pathsStr == "" {
		return NewTreeViewWithOptions(nil, maxDepth, maxLines, collapseSingle)
	}
	paths := strings.Split(strings.TrimSpace(pathsStr), "\n")
	return NewTreeViewWithOptions(paths, maxDepth, maxLines, collapseSingle)
}

// NewTreeViewFromFS 从 FileSystem 创建树形视图实例（无限制）
func NewTreeViewFromFS(filesystem filesys_interface.FileSystem, root string) *TreeView {
	return NewTreeViewFromFSWithOptions(filesystem, root, 0, 0, false)
}

// NewTreeViewFromFSWithLimits 从 FileSystem 创建带限制的树形视图实例
func NewTreeViewFromFSWithLimits(filesystem filesys_interface.FileSystem, root string, maxDepth, maxLines int) *TreeView {
	return NewTreeViewFromFSWithOptions(filesystem, root, maxDepth, maxLines, false)
}

// NewTreeViewFromFSWithOptions 从 FileSystem 创建带完整选项的树形视图实例
func NewTreeViewFromFSWithOptions(filesystem filesys_interface.FileSystem, root string, maxDepth, maxLines int, collapseSingle bool) *TreeView {
	if maxDepth <= 0 {
		maxDepth = 4
	}

	if maxLines <= 0 {
		maxLines = 100
	}

	if filesystem == nil {
		return NewTreeViewWithOptions(nil, maxDepth, maxLines, collapseSingle)
	}

	paths := collectPathsFromFS(filesystem, root, maxDepth, maxLines)
	// 为文件系统创建特殊的树结构，保存完整路径信息
	data := []byte(strings.Join(paths, "\n"))
	index := suffixarray.New(data)
	treeRoot := buildTreeWithPaths(paths)
	if collapseSingle {
		treeRoot = collapseTree(treeRoot)
	}

	tv := &TreeView{
		root:           treeRoot,
		index:          index,
		data:           data,
		maxDepth:       maxDepth,
		maxLines:       maxLines,
		collapseSingle: collapseSingle,
		filesystem:     filesystem,
		rootPath:       root,
	}
	return tv
}

// collectPathsFromFS 从 FileSystem 递归收集所有路径
func collectPathsFromFS(filesystem filesys_interface.FileSystem, root string, maxDepth int, maxCount int) []string {
	var paths []string
	var fileCount int

	// 应用5倍限制
	actualMaxDepth := maxDepth * 5
	actualMaxCount := maxCount * 5

	var walkFS func(string, int) error
	walkFS = func(path string, currentDepth int) error {
		// 检查深度限制
		if currentDepth > actualMaxDepth {
			return nil
		}

		// 检查文件数量限制
		if fileCount >= actualMaxCount {
			return nil
		}

		// 添加当前路径到结果（如果是目录）
		if path != "." && path != "" {
			paths = append(paths, path)
		}

		// 读取目录内容
		entries, err := filesystem.ReadDir(path)
		if err != nil {
			return err
		}

		// 遍历目录项
		for _, entry := range entries {
			// 检查文件数量限制
			if fileCount >= actualMaxCount {
				break
			}

			entryPath := path
			if path == "." || path == "" {
				entryPath = entry.Name()
			} else {
				entryPath = filepath.Join(path, entry.Name())
			}

			if entry.IsDir() {
				// 递归处理子目录，深度+1
				if err := walkFS(entryPath, currentDepth+1); err != nil {
					continue // 跳过错误目录继续处理其他目录
				}
			} else {
				// 添加文件路径并增加文件计数
				paths = append(paths, entryPath)
				fileCount++
			}
		}

		return nil
	}

	// 从根目录开始遍历，初始深度为0
	if err := walkFS(root, 0); err != nil {
		// 如果遍历失败，返回空路径
		return []string{}
	}

	return paths
}

// newNode 创建新节点
func newNode(name string) *Node {
	return &Node{
		name:     name,
		children: make(map[string]*Node),
		isFile:   false,
		fullPath: "",
	}
}

// newNodeWithPath 创建带路径的新节点
func newNodeWithPath(name, fullPath string) *Node {
	return &Node{
		name:     name,
		children: make(map[string]*Node),
		isFile:   false,
		fullPath: fullPath,
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

// buildTreeWithPaths 构建带路径信息的树结构
func buildTreeWithPaths(paths []string) *Node {
	if paths == nil {
		return newNode("")
	}

	root := newNode("")

	for _, fullPath := range paths {
		if fullPath == "" {
			continue
		}

		parts := strings.Split(strings.TrimSpace(fullPath), "/")
		current := root
		currentPath := ""

		for i, part := range parts {
			if part == "" {
				continue
			}

			// 构建当前路径
			if currentPath == "" {
				currentPath = part
			} else {
				currentPath = filepath.Join(currentPath, part)
			}

			if _, exists := current.children[part]; !exists {
				current.children[part] = newNodeWithPath(part, currentPath)
			}
			current = current.children[part]
			if i == len(parts)-1 {
				current.isFile = true
				current.fullPath = fullPath // 保存完整路径
			}
		}
	}

	return root
}

// collapseTree 合并单文件夹，将只有一个子节点的目录与子节点合并
func collapseTree(root *Node) *Node {
	if root == nil {
		return nil
	}

	// 递归处理所有子节点
	for name, child := range root.children {
		root.children[name] = collapseNode(child)
	}

	return root
}

// collapseNode 合并单个节点
func collapseNode(node *Node) *Node {
	if node == nil {
		return nil
	}

	// 如果是文件，直接返回
	if node.isFile {
		return node
	}

	// 首先递归处理所有子节点
	for name, child := range node.children {
		node.children[name] = collapseNode(child)
	}

	// 合并逻辑：如果当前节点只有一个子节点，且该子节点是目录，
	// 且子节点不是直接包含文件的目录，则进行合并
	if len(node.children) == 1 {
		for childName, child := range node.children {
			if !child.isFile && !hasDirectFiles(child) {
				// 合并名称：父节点/子节点
				if node.name == "" {
					node.name = childName
				} else {
					node.name = node.name + "/" + childName
				}
				// 继承子节点的属性
				node.children = child.children
				node.isFile = child.isFile
				// 递归继续合并，直到不能再合并为止
				return collapseNode(node)
			}
		}
	}

	return node
}

// hasDirectFiles 检查节点是否直接包含文件
func hasDirectFiles(node *Node) bool {
	if node == nil || node.children == nil {
		return false
	}

	for _, child := range node.children {
		if child.isFile {
			return true
		}
	}
	return false
}

// Print 打印树形结构
func (tv *TreeView) Print() string {
	if tv == nil || tv.root == nil {
		return ""
	}

	var builder strings.Builder
	builder.WriteString(".\n")
	lineCount := 1
	tv.printNodeWithPathAndRemaining(tv.root, "", true, &builder, 0, &lineCount, "", 0)
	return builder.String()
}

// getFileInfoFromNode 从节点获取文件信息字符串
func (tv *TreeView) getFileInfoFromNode(node *Node) string {
	if tv.filesystem == nil || node == nil || !node.isFile {
		return ""
	}

	// 使用节点中保存的完整路径
	filePath := node.fullPath
	if filePath == "" {
		return ""
	}

	// log.Debugf("trying to get file info for path: %s", filePath)
	info, err := tv.filesystem.Stat(filePath)
	if err != nil {
		// log.Debugf("failed to get file info for %s: %v", filePath, err)
		return ""
	}

	// 格式化文件大小
	size := info.Size()
	var sizeStr string
	if size < 1024 {
		sizeStr = fmt.Sprintf("%d bytes", size)
	} else if size < 1024*1024 {
		sizeStr = fmt.Sprintf("%.1f KB", float64(size)/1024)
	} else if size < 1024*1024*1024 {
		sizeStr = fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	} else {
		sizeStr = fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
	}

	return fmt.Sprintf(" (%s)", sizeStr)
}

// countRemainingItems 计算剩余项目数量
func (tv *TreeView) countRemainingItems(node *Node, currentDepth int) int {
	if node == nil || node.children == nil {
		return 0
	}

	count := 0
	for _, child := range node.children {
		count++
		if !child.isFile && (tv.maxDepth == 0 || currentDepth+1 < tv.maxDepth) {
			count += tv.countRemainingItems(child, currentDepth+1)
		}
	}

	return count
}

// printNode 打印节点（内部方法）
func (tv *TreeView) printNode(node *Node, prefix string, isLast bool, builder *strings.Builder, depth int, lineCount *int) {
	tv.printNodeWithPath(node, prefix, isLast, builder, depth, lineCount, "")
}

// printNodeWithPath 打印节点带路径（内部方法）
func (tv *TreeView) printNodeWithPath(node *Node, prefix string, isLast bool, builder *strings.Builder, depth int, lineCount *int, currentPath string) {
	tv.printNodeWithPathAndRemaining(node, prefix, isLast, builder, depth, lineCount, currentPath, 0)
}

// printNodeWithPathAndRemaining 打印节点带路径和剩余项目信息（内部方法）
func (tv *TreeView) printNodeWithPathAndRemaining(node *Node, prefix string, isLast bool, builder *strings.Builder, depth int, lineCount *int, currentPath string, remainingSiblings int) {
	if node == nil || builder == nil {
		return
	}

	// 检查行数限制
	if tv.maxLines > 0 && *lineCount >= tv.maxLines {
		if *lineCount == tv.maxLines {
			var ellipsisText string
			if remainingSiblings > 0 {
				ellipsisText = fmt.Sprintf("... (%d more items)", remainingSiblings)
			} else {
				ellipsisText = "..."
			}
			if isLast {
				builder.WriteString(fmt.Sprintf("%s└── %s\n", prefix, ellipsisText))
			} else {
				builder.WriteString(fmt.Sprintf("%s├── %s\n", prefix, ellipsisText))
			}
			*lineCount++
		}
		return
	}

	// 检查深度限制
	if tv.maxDepth > 0 && depth >= tv.maxDepth {
		if node.name != "" {
			var displayText string
			if node.isFile {
				// 对于文件，显示文件信息而不是 "..."
				fileInfo := tv.getFileInfoFromNode(node)
				displayText = node.name + fileInfo
			} else {
				// 对于目录，计算子项数量
				childCount := len(node.children)
				displayText = fmt.Sprintf("%s ... (%d items)", node.name, childCount)
			}
			if isLast {
				builder.WriteString(fmt.Sprintf("%s└── %s\n", prefix, displayText))
			} else {
				builder.WriteString(fmt.Sprintf("%s├── %s\n", prefix, displayText))
			}
			*lineCount++
		}
		return
	}

	if node.name != "" {
		nodePath := currentPath
		if currentPath == "" {
			nodePath = node.name
		} else {
			nodePath = filepath.Join(currentPath, node.name)
		}
		var displayText string
		if node.isFile {
			// 对于文件，显示文件信息
			fileInfo := tv.getFileInfoFromNode(node)
			displayText = node.name + fileInfo
		} else {
			// 对于目录，只显示名称
			displayText = node.name
		}
		if isLast {
			builder.WriteString(fmt.Sprintf("%s└── %s\n", prefix, displayText))
			prefix += "    "
		} else {
			builder.WriteString(fmt.Sprintf("%s├── %s\n", prefix, displayText))
			prefix += "│   "
		}
		*lineCount++
		currentPath = nodePath
	}

	if node.children == nil {
		return
	}

	var keys []string
	for k := range node.children {
		keys = append(keys, k)
	}
	// 自定义排序：隐藏文件（以.开头）排在后面
	sort.Slice(keys, func(i, j int) bool {
		keyI, keyJ := keys[i], keys[j]
		isDotI := strings.HasPrefix(keyI, ".")
		isDotJ := strings.HasPrefix(keyJ, ".")

		// 如果一个是隐藏文件，一个不是，非隐藏文件排在前面
		if isDotI && !isDotJ {
			return false
		}
		if !isDotI && isDotJ {
			return true
		}

		// 如果都是隐藏文件或都不是隐藏文件，按字母顺序排序
		return keyI < keyJ
	})

	for i, key := range keys {
		isLastChild := i == len(keys)-1
		childPath := currentPath
		if node.name != "" {
			if currentPath == "" {
				childPath = node.name
			} else {
				childPath = filepath.Join(currentPath, node.name)
			}
		}
		// 计算剩余的兄弟节点数量
		remainingSiblings := len(keys) - i - 1
		tv.printNodeWithPathAndRemaining(node.children[key], prefix, isLastChild, builder, depth+1, lineCount, childPath, remainingSiblings)
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
