package dot

import (
	"fmt"
	"io"
)

// Graph represents a set of nodes, edges and attributes that can be
// translated to DOT language.
type Graph struct {
	nodes           map[int]*node
	edges           map[int]*edge
	n, e            int
	graphAttributes attributes
	nodeAttributes  attributes
	edgeAttributes  attributes
	drawMultiEdges  bool
	directed        bool
	sameRank        [][]*node
}

func New() *Graph {
	return &Graph{}
}

// SetTitle sets a title for the graph.
func (g *Graph) SetTitle(title string) {
	g.GraphAttribute("label", title)
}

// AddNode adds a new node with the given label and returns its id.
func (g *Graph) AddNode(label string) int {
	nod := node{id: g.n, label: label}
	g.n++
	if g.nodes == nil {
		g.nodes = make(map[int]*node)
	}
	g.nodes[nod.id] = &nod
	return nod.id
}

// GetOrCreateNode returns the id of the node with the given label if it
func (g *Graph) GetOrCreateNode(label string) int {
	id, ok := g.NodeExisted(label)
	if ok {
		return id
	}
	return g.AddNode(label)
}

// NodeExisted returns the id of the node with the given label if it
func (g *Graph) NodeExisted(label string) (int, bool) {
	for _, node := range g.nodes {
		if node.label == label {
			return node.id, true
		}
	}
	return -1, false
}

// MakeSameRank causes the specified nodes to be drawn on the same rank.
// Only effective when using the dot tool.
func (g *Graph) MakeSameRank(node1, node2 int, others ...int) {
	r := make([]*node, 2+len(others))
	r[0] = g.nodes[node1]
	r[1] = g.nodes[node2]
	for i := range others {
		r[2+i] = g.nodes[others[i]]
	}
	g.sameRank = append(g.sameRank, r)
}

// AddEdge adds a new edge between the given nodes with the specified
// label and returns an id for the new edge.
func (g *Graph) AddEdge(from, to int, label string) int {
	fromNode := g.nodes[from]
	toNode := g.nodes[to]
	// TODO: Check for errors (non-existing nodes)
	edg := edge{from: fromNode, to: toNode, label: label}
	id := g.e
	g.e++
	if g.edges == nil {
		g.edges = make(map[int]*edge)
	}
	g.edges[id] = &edg
	return id
}

// MakeDirected makes the graph a directed graph. By default, a new
// graph is undirected
func (g *Graph) MakeDirected() {
	g.directed = true
}

// DrawMultipleEdges causes multiple edges between same pair of nodes
// to be drawn separately. By default, for a given pair of nodes, only
// the edge that was last added to the graph is drawn.
func (g *Graph) DrawMultipleEdges() {
	g.drawMultiEdges = true
}

// NodeAttribute sets an attribute for the specified node.
func (g *Graph) NodeAttribute(id int, name, value string) {
	// TODO: check for errors (id out of range)
	g.nodes[id].attributes.set(name, value)
}

// EdgeAttribute sets an attribute for the specified edge.
func (g *Graph) EdgeAttribute(id int, name, value string) {
	// TODO: check for errors (id out of range)
	g.edges[id].attributes.set(name, value)
}

// DefaultNodeAttribute sets an attribute for all nodes by default.
func (g *Graph) DefaultNodeAttribute(name, value string) {
	g.nodeAttributes.set(name, value)
}

// DefaultEdgeAttribute sets an attribute for all edges by default.
func (g *Graph) DefaultEdgeAttribute(name, value string) {
	g.edgeAttributes.set(name, value)
}

// GraphAttribute sets an attribute for the graph
func (g *Graph) GraphAttribute(name, value string) {
	g.graphAttributes.set(name, value)
}

// GenerateDOT generates the graph description in DOT language
func (g Graph) GenerateDOT(w io.Writer) {
	if !g.drawMultiEdges {
		fmt.Fprint(w, "strict ")
	}
	if g.directed {
		fmt.Fprint(w, "digraph ")
	} else {
		fmt.Fprint(w, "graph ")
	}
	fmt.Fprintln(w, "{")
	for graphAttribs := g.graphAttributes.iterate(); graphAttribs.hasMore(); {
		name, value := graphAttribs.next()
		fmt.Fprintf(w, "  %v = %#v;\n", name, value)
	}
	for nodeAttribs := g.nodeAttributes.iterate(); nodeAttribs.hasMore(); {
		name, value := nodeAttribs.next()
		fmt.Fprintf(w, "  node [ %v = %#v ]\n", name, value)
	}
	for edgeAttribs := g.edgeAttributes.iterate(); edgeAttribs.hasMore(); {
		name, value := edgeAttribs.next()
		fmt.Fprintf(w, "  edge [ %v = %#v ]\n", name, value)
	}
	for i := 0; i < g.n; i++ {
		fmt.Fprint(w, "  ")
		g.nodes[i].generateDOT(w)
		fmt.Fprintln(w)
	}
	for _, r := range g.sameRank {
		fmt.Fprint(w, "  {rank=same; ")
		for _, x := range r {
			fmt.Fprintf(w, "%v; ", x.name())
		}
		fmt.Fprintln(w, "}")
	}
	for i := 0; i < g.e; i++ {
		fmt.Fprint(w, "  ")
		g.edges[i].generateDOT(w, g.directed)
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "}")
}

type attributes struct {
	attributeMap map[string]string
	namesOrdered []string
}

func (a *attributes) set(name, value string) {
	if a.attributeMap == nil {
		a.attributeMap = make(map[string]string)
	}
	if _, exists := a.attributeMap[name]; !exists {
		a.namesOrdered = append(a.namesOrdered, name)
	}
	a.attributeMap[name] = value
}

func (a *attributes) iterate() attributeIterator {
	return attributeIterator{a, 0}
}

type attributeIterator struct {
	attributes *attributes
	index      int
}

func (ai *attributeIterator) hasMore() bool {
	return ai.index < len(ai.attributes.namesOrdered)
}

func (ai *attributeIterator) next() (name, value string) {
	name = ai.attributes.namesOrdered[ai.index]
	value = ai.attributes.attributeMap[name]
	ai.index++
	return name, value
}

type node struct {
	id         int
	label      string
	attributes attributes
}

func (n node) name() string {
	return fmt.Sprintf("n%v", n.id)
}

func (n node) generateDOT(w io.Writer) {
	fmt.Fprintf(w, "%v [label=%#v", n.name(), n.label)
	for attribs := n.attributes.iterate(); attribs.hasMore(); {
		name, value := attribs.next()
		fmt.Fprintf(w, ", %v=%#v", name, value)
	}
	fmt.Fprint(w, "]")
}

type edge struct {
	from       *node
	to         *node
	label      string
	attributes attributes
}

func (e edge) generateDOT(w io.Writer, directed bool) {
	edgeOp := "--"
	if directed {
		edgeOp = "->"
	}
	fmt.Fprintf(w, "%v %v %v [label=%#v", e.from.name(), edgeOp, e.to.name(), e.label)
	for attribs := e.attributes.iterate(); attribs.hasMore(); {
		name, value := attribs.next()
		fmt.Fprintf(w, ", %v=%#v", name, value)
	}
	fmt.Fprint(w, "]")
}
