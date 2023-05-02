// Package newcrawlerx
// @Author bcy2007  2023/3/23 10:40
package newcrawlerx

import (
	"sync"
	"yaklang/common/log"
)

type UrlNode struct {
	url   string
	son   []*UrlNode
	level int
	next  *UrlNode
}

func CreateNode(url string, level int) *UrlNode {
	return &UrlNode{
		url:   url,
		son:   make([]*UrlNode, 0),
		level: level,
	}
}

func (node *UrlNode) Add(url string) *UrlNode {
	son := CreateNode(url, node.level+1)
	node.son = append(node.son, son)
	return son
}

func (node *UrlNode) Next(nextNode *UrlNode) {
	node.next = nextNode
}

type UrlTree struct {
	sync.Mutex

	root     *UrlNode
	last     *UrlNode
	maxLevel int
	count    int
}

func CreateTree(url string) *UrlTree {
	rootNode := CreateNode(url, 1)
	return &UrlTree{
		root:     rootNode,
		last:     rootNode,
		maxLevel: 1,
		count:    1,
	}
}

func (tree *UrlTree) Find(url string) *UrlNode {
	for node := tree.root; node != nil; node = node.next {
		if node.url == url {
			return node
		}
	}
	//log.Infof("url %s not found.", url)
	return nil
}

func (tree *UrlTree) Count() int {
	return tree.count
}

func (tree *UrlTree) Level() int {
	return tree.maxLevel
}

func (tree *UrlTree) Add(parent string, sons ...string) {
	tree.Lock()
	defer tree.Unlock()
	if parent == "" {
		log.Infof("parent url %s invalid", parent)
		return
	}
	upper := tree.Find(parent)
	if upper == nil {
		log.Infof("parent %s not found.", parent)
		return
	}
	for _, son := range sons {
		if son == "" {
			log.Infof("son url %s invalid", son)
			continue
		}
		temp := tree.Find(son)
		if temp != nil {
			continue
		}
		sonNode := upper.Add(son)
		tree.last.Next(sonNode)
		tree.last = sonNode
	}
	if tree.maxLevel == upper.level {
		tree.maxLevel += 1
	}
	tree.count += len(sons)
}

func (tree *UrlTree) Show() string {
	temp := ""
	for node := tree.root; node != nil; node = node.next {
		for _, son := range node.son {
			temp += node.url + " -> " + son.url + "\n"
		}
	}
	return temp
}
