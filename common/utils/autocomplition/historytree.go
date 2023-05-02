package autocomplition

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/c-bata/go-prompt"
	"github.com/google/shlex"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"sort"
	"strings"
	"yaklang.io/yaklang/common/log"
	utils2 "yaklang.io/yaklang/common/utils"
)

var (
	blackList = []string{
		"ls", "cd", "cat", "tree",
	}
)

func getBashHistoryRawLines(raw []byte) []string {
	scanner := bufio.NewScanner(bytes.NewBuffer(raw))
	scanner.Split(bufio.ScanLines)

	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func getZshHistoryRawLines(raw []byte) []string {
	scanner := bufio.NewScanner(bytes.NewBuffer(raw))
	scanner.Split(bufio.ScanLines)

	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		rets := strings.SplitN(line, ";", 2)
		if len(rets) < 2 {
			continue
		}

		line = rets[1]
		lines = append(lines, line)
	}
	return lines
}

func GetSystemHistoryTreeRawLines(cmds ...string) []string {
	var rawCommandHistoryLines = cmds

	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		usr, err := user.Current()
		if err != nil {
			log.Errorf("get os user failed: %s", err)
		} else {
			homeDir = usr.HomeDir
		}
	}

	bashHistoryFileContent, err := ioutil.ReadFile(path.Join(homeDir, ".bash_history"))
	if err != nil {
		log.Debugf("get bash history file failed: %s", err)
	} else {
		rawCommandHistoryLines = append(rawCommandHistoryLines, getBashHistoryRawLines(bashHistoryFileContent)...)
	}

	zshHistoryFileContent, err := ioutil.ReadFile(path.Join(homeDir, ".zsh_history"))
	if err != nil {
		log.Debugf("get bash history file failed: %s", err)
	} else {
		rawCommandHistoryLines = append(rawCommandHistoryLines, getZshHistoryRawLines(zshHistoryFileContent)...)
	}
	return rawCommandHistoryLines
}

type ComplimentNode struct {
	// 补全内容
	Data   string
	Origin string

	// 使用次数（决定优先级）
	UseCount int

	Help string

	// 补全节点
	Children map[string]*ComplimentNode

	// 回溯父节点
	Parent *ComplimentNode
}

func (c *ComplimentNode) GetSuggessByArgs(args []string) []prompt.Suggest {
	var sugs []prompt.Suggest

	// 按照使用频率排序
	var sortedNodes []*ComplimentNode
	for _, n := range c.Children {
		sortedNodes = append(sortedNodes, n)
	}
	sort.SliceStable(sortedNodes, func(i, j int) bool {
		return sortedNodes[i].Data > sortedNodes[j].Data
	})
	sort.SliceStable(sortedNodes, func(i, j int) bool {
		return sortedNodes[i].UseCount > sortedNodes[j].UseCount
	})

	// 排序后的节点输出出来
	for _, node := range sortedNodes {
		var desc = node.Help

		if desc == "" {
			shortAfterCmd := node.Origin
			if len(shortAfterCmd) > 30 {
				shortAfterCmd = shortAfterCmd[:27] + "..."
			}

			if shortAfterCmd != "" {
				desc = fmt.Sprintf("-> %s", shortAfterCmd)
			}
		}
		sugs = append(sugs, prompt.Suggest{
			Text:        node.Data,
			Description: desc,
		})
	}
	return sugs
}

func (c *ComplimentNode) getOrCreateChild(raw string, origin string) *ComplimentNode {
	node, ok := c.Children[raw]
	if ok {
		node.UseCount++
		return node
	}

	//log.Infof("create node[%s] and add[%v]", raw, origin)
	node = &ComplimentNode{
		Data:     raw,
		Origin:   origin,
		Children: make(map[string]*ComplimentNode),
		Parent:   c,
	}
	c.Children[raw] = node
	return c
}

func (c *ComplimentNode) addArgs(rets []string) {
	//var child = c
	//
	//for index, value := range rets {
	//	if index+1 >= len(rets) {
	//		continue
	//	}
	//
	//	log.Infof("create index-%v node[%s] and add[%v]", index, value, rets)
	//
	//	child = child.getOrCreateChild(value, strings.Join(rets[index+1:], " "))
	//	child.addArgs(rets[index+1:])
	//}
	if len(rets) >= 2 {
		child := c.getOrCreateChild(rets[0], strings.Join(rets[1:], " "))
		child.addArgs(rets[1:])
		return
	}

	if len(rets) == 1 {
		_ = c.getOrCreateChild(rets[0], "")
		return
	}
}

// 定义补全森林
type AutoComplitionForest struct {
	trees map[string]*ComplimentNode
}

func (a *AutoComplitionForest) ApplyHistories(raw ...string) {
	for _, line := range raw {
		results, err := shlex.Split(line)
		if err != nil {
			continue
		}
		a.applyHistory(results)
	}
}

func (a *AutoComplitionForest) applyHistory(raw []string) {
	if len(raw) <= 0 {
		return
	}

	cmd, args := raw[0], raw[1:]
	if utils2.StringArrayContains(blackList, cmd) {
		return
	}

	node := a.getRootNodeOrCreateRootNode(cmd, strings.Join(raw, " "))
	node.addArgs(args)
}

func (a *AutoComplitionForest) getRootNodeOrCreateRootNode(raw string, origin string) *ComplimentNode {
	node, ok := a.trees[raw]
	if ok {
		return node
	}

	node = &ComplimentNode{
		Data:     raw,
		Origin:   origin,
		Children: make(map[string]*ComplimentNode),
	}
	a.trees[raw] = node

	return node
}

func (a *AutoComplitionForest) getRootNode(cmd string) (*ComplimentNode, error) {
	node, ok := a.trees[cmd]
	if !ok {
		return nil, errors.Errorf("cmd[%s] don't have any suggestions", cmd)
	}
	return node, nil
}

func (a *AutoComplitionForest) GetTreeRootsSuggestions() []prompt.Suggest {
	// 按照使用频率排序
	var (
		sortedNodes []*ComplimentNode
		suggestions []prompt.Suggest
	)
	for _, n := range a.trees {
		sortedNodes = append(sortedNodes, n)
	}
	sort.SliceStable(sortedNodes, func(i, j int) bool {
		return sortedNodes[i].Data > sortedNodes[j].Data
	})
	sort.SliceStable(sortedNodes, func(i, j int) bool {
		return sortedNodes[i].UseCount > sortedNodes[j].UseCount
	})

	for _, node := range sortedNodes {
		suggestions = append(suggestions, prompt.Suggest{
			Text: node.Data,
		})
	}

	return suggestions
}

func (a *AutoComplitionForest) GetSuggest(cmd string) (sugs []prompt.Suggest) {
	parseResults, err := shlex.Split(cmd)
	if err != nil {
		return
	}

	if len(parseResults) <= 0 {
		return a.GetTreeRootsSuggestions()
	}

	if len(parseResults) == 1 && !strings.HasSuffix(cmd, " ") {
		return a.GetTreeRootsSuggestions()
	}

	var (
		name string
		args []string
	)
	if len(parseResults) > 1 {
		name, args = parseResults[0], parseResults[1:]
	} else {
		name, args = parseResults[0], nil
	}

	tree, err := a.getRootNode(name)
	if err != nil {
		return
	}

	sugs = tree.GetSuggessByArgs(args)
	return sugs
}

func GetDefaultSystemHistoryAutoComplitionForest(cmds ...string) *AutoComplitionForest {
	forest := &AutoComplitionForest{trees: make(map[string]*ComplimentNode)}

	var rawCmds = GetSystemHistoryTreeRawLines(cmds...)

	rawCmds = utils2.RemoveRepeatStringSlice(rawCmds)
	if len(rawCmds) > 1000 {
		rawCmds = rawCmds[len(rawCmds)-1000:]
	}
	forest.ApplyHistories(rawCmds...)

	return forest
}

func GetHistoryAutoComplitionForest(cmds ...string) *AutoComplitionForest {
	forest := &AutoComplitionForest{trees: make(map[string]*ComplimentNode)}

	var rawCmds = cmds
	rawCmds = utils2.RemoveRepeatStringSlice(rawCmds)
	//if len(rawCmds) > 1000 {
	//	rawCmds = rawCmds[len(rawCmds)-1000:]
	//}

	forest.ApplyHistories(rawCmds...)

	return forest
}
