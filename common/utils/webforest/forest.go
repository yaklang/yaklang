package webforest

import (
	"bufio"
	"bytes"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/utils"
	"net/url"
	"strings"
)

type WebsiteNode struct {
	Parent *WebsiteNode `json:"-"`

	// 如果是根结点的话，
	NodeName       string   `json:"node_name"`
	HTTPRequestIDs []uint   `json:"http_request_ids"` // 暂时弃用
	Urls           []string `json:"urls"`

	// path == / 或者 "" 都为根结点
	Path string `json:"path"`

	Children map[string]*WebsiteNode `json:"children"`

	Uuid string `json:"uuid"`
}

func (n *WebsiteNode) GetRootName() string {
	var current = n
	for {
		if current.Parent == nil {
			return n.NodeName
		}

		current = current.Parent
	}
}

func (n *WebsiteNode) IsRoot() bool {
	return n.Parent == nil
}

func (n *WebsiteNode) IsLeaf() bool {
	return len(n.Children) <= 0
}

type WebsiteForest struct {
	MaxSize int `json:"max_size"`

	// schema + hostname + port
	Roots map[string]*WebsiteNode `json:"Roots"`

	Uuid string `json:"uuid"`
}

func NewWebsiteForest(size int) *WebsiteForest {
	return &WebsiteForest{
		MaxSize: size,
		Roots:   make(map[string]*WebsiteNode),
		Uuid:    uuid.NewV4().String(),
	}
}

type treeItem struct {
	WebsiteName string   `json:"website_name"`
	Path        string   `json:"path"`
	NodeName    string   `json:"node_name"`
	Urls        []string `json:"urls"`
	RequestIDs  []uint   `json:"request_ids"`
	Uuid        string   `json:"uuid"`

	Children []*treeItem `json:"children"`
}

type WebsiteForestOutputBasic struct {
	Size  int
	Trees []*treeItem
	Uuid  string
}

func (w *WebsiteNode) toTreeItem() *treeItem {
	tree := &treeItem{
		WebsiteName: w.GetRootName(),
		Path:        w.Path,
		NodeName:    w.NodeName,
		Urls:        w.Urls,
		RequestIDs:  w.HTTPRequestIDs,
		Uuid:        w.Uuid,
	}
	if tree.Uuid == "" {
		tree.Uuid = uuid.NewV4().String()
	}

	for _, i := range w.Children {
		tree.Children = append(tree.Children, i.toTreeItem())
	}

	return tree
}

func (w *WebsiteForest) ToBasicOutput() *WebsiteForestOutputBasic {
	var trees []*treeItem
	for _, root := range w.Roots {
		trees = append(trees, root.toTreeItem())
	}

	return &WebsiteForestOutputBasic{
		Size:  len(trees),
		Trees: trees,
		Uuid:  w.Uuid,
	}
}

func (w *WebsiteForest) AddNode(u string) error {
	urlobj, err := url.Parse(u)
	if err != nil {
		return utils.Errorf("url parse failed; %s", err)
	}

	var rootName string
	if urlobj.Port() != "" {
		rootName = fmt.Sprintf("%v://%v:%v", urlobj.Scheme, urlobj.Hostname(), urlobj.Port())
	} else {
		rootName = fmt.Sprintf("%v://%v", urlobj.Scheme, urlobj.Hostname())
	}

	rootNode, err := w.getOrCreateRootNode(rootName)
	if err != nil {
		return utils.Errorf("add reqeust failed: %s", err)
	}
	if rootNode.Uuid == "" {
		rootNode.Uuid = uuid.NewV4().String()
	}
	websiteNode, err := rootNode.getOrCreateNode(urlobj.Path)
	if err != nil {
		return utils.Errorf("create or get website node failed: %s", urlobj.Path)
	}
	if websiteNode.Uuid == "" {
		websiteNode.Uuid = uuid.NewV4().String()
	}
	websiteNode.Urls = utils.RemoveRepeatStringSlice(append(websiteNode.Urls, u))
	//websiteNode.HTTPRequestIDs = utils.RemoveRepeatUintSlice(append(websiteNode.HTTPRequestIDs, req.ID))
	return nil
}

func (w *WebsiteForest) getOrCreateRootNode(rootName string) (*WebsiteNode, error) {
	node, ok := w.Roots[rootName]
	if ok {
		return node, nil
	}

	if len(w.Roots) >= w.MaxSize {
		return nil, utils.Errorf("forest size limited, cannot create: %s", rootName)
	}

	root := &WebsiteNode{
		NodeName: rootName,
		Path:     "/",
		Children: make(map[string]*WebsiteNode),
	}
	w.Roots[rootName] = root
	return root, nil
}

func (w *WebsiteNode) getOrCreateNode(path string) (*WebsiteNode, error) {
	if !w.IsRoot() {
		return nil, utils.Errorf("root node can get or create nodes")
	}

	if !strings.HasPrefix(path, "/") {
		return nil, utils.Errorf("invalid path: %s", path)
	}

	blocks := pathToBlocks(path)

	var (
		buf      string
		lastNode *WebsiteNode = w
	)
	for index, b := range blocks {
		buf += b
		if index <= 0 {
			continue
		}
		lastNode = lastNode.getOrCreateChildByNodeName(b, buf)
	}
	return lastNode, nil
}

func (w *WebsiteNode) getOrCreateChildByNodeName(nodeName string, path string) *WebsiteNode {
	node, ok := w.Children[nodeName]
	if ok {
		return node
	}

	node = &WebsiteNode{
		Parent:   w,
		NodeName: nodeName,
		Path:     path,
		Children: map[string]*WebsiteNode{},
	}
	w.Children[nodeName] = node
	return node
}

func pathToBlocks(path string) []string {
	scanner := bufio.NewScanner(bytes.NewBufferString(path))
	scanner.Split(bufio.ScanBytes)

	var blocks []string
	var buf string
	for scanner.Scan() {
		if scanner.Text() == "/" {
			if buf != "" {
				blocks = append(blocks, buf)
				buf = ""
			}
			blocks = append(blocks, "/")
			continue
		} else {
			buf += scanner.Text()
		}
	}
	if buf != "" {
		blocks = append(blocks, buf)
	}
	return blocks
}
