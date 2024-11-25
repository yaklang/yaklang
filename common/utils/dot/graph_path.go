package dot

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
)

// deep first search for nodeID and its children to [][]id, id is string,
// if node.Prev have more than one, add a new line
type DeepFirstPath struct {
	res     [][]string
	next    func(int) []int
	current *orderedmap.OrderedMap // map[string]nil
}

func (d *DeepFirstPath) deepFirst(nodeID int) {
	if _, ok := d.current.Get(NodeName(nodeID)); ok {
		// log.Infof("node %d already in current skip", nodeID)
		return
	}
	d.current.Set(NodeName(nodeID), nil)
	// log.Infof("node %d add to current path: %v", nodeID, d.current.Keys())
	nextNodes := d.next(nodeID)
	nextNodes = lo.Uniq(nextNodes)
	// log.Infof("next node :%v", nextNodes)
	if len(nextNodes) == 0 {
		d.res = append(d.res, d.current.Keys())
		return
	}
	if len(nextNodes) == 1 {
		prev := nextNodes[0]
		d.deepFirst(prev)
		return
	}

	// origin
	current := d.current
	for _, next := range nextNodes {
		// new line
		d.current = current.Copy()
		d.deepFirst(next)
	}
}

func GraphPathPrev(g *Graph, nodeId int) [][]string {
	return GraphPath(nodeId, func(i int) []int {
		node := g.GetNodeByID(i)
		return node.Prevs()
	})
}

func GraphPathNext(g *Graph, nodeId int) [][]string {
	return GraphPath(nodeId, func(i int) []int {
		node := g.GetNodeByID(i)
		return node.Nexts()
	})
}

func GraphPath(nodeID int, next func(int) []int) [][]string {
	df := &DeepFirstPath{
		res:     make([][]string, 0),
		current: orderedmap.New(),
		next:    next,
	}
	df.deepFirst(nodeID)
	return df.res
}
