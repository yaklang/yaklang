package dot

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/samber/lo"
)

// Graph represents a set of nodes, edges and attributes that can be
// translated to DOT language.
type Graph struct {
	idGetter  func() int
	parent    *Graph
	subGraphs []*Graph
	// global
	registeredNodes map[int]*node
	registeredEdges map[int]*Edge

	// in this graph
	nodes           map[int]*node
	edges           map[int]*Edge
	graphAttributes attributes
	nodeAttributes  attributes
	edgeAttributes  attributes
	drawMultiEdges  bool
	directed        bool
	sameRank        [][]*node
}

func (g *Graph) IsSubGraph() bool {
	if g.parent != nil {
		return true
	}
	return false
}

func (g *Graph) CreateSubGraph(label string) *Graph {
	if label == "" {
		return g
	}
	sub := &Graph{
		idGetter:        g.idGetter,
		registeredNodes: g.registeredNodes,
		registeredEdges: g.registeredEdges,
		drawMultiEdges:  g.drawMultiEdges,
		directed:        g.directed,
	}
	sub.parent = g
	sub.SetTitle(label)
	g.subGraphs = append(g.subGraphs, sub)
	return sub
}

func New() *Graph {
	counter := 0
	idGetter := func() int {
		counter++
		return counter
	}
	return &Graph{idGetter: idGetter, registeredEdges: make(map[int]*Edge), registeredNodes: make(map[int]*node)}
}

// SetTitle sets a title for the graph.
func (g *Graph) SetTitle(title string) {
	g.GraphAttribute("label", title)
}

// AddNode adds a new node with the given label and returns its id.
func (g *Graph) AddNode(label string) int {
	newId := g.idGetter()
	nod := CreateNode(newId, label)
	if g.nodes == nil {
		g.nodes = make(map[int]*node)
	}
	g.nodes[nod.id] = &nod
	g.registeredNodes[nod.id] = &nod
	return nod.id
}

func (g *Graph) SetNode(id int, label string) {
	g.nodes[id].label = label
}

func (g *Graph) GetOrCreateSubGraph(label string) *Graph {
	for _, sub := range g.subGraphs {
		if sub.GraphAttribute("label", label); sub != nil {
			return sub
		}
	}
	return g.CreateSubGraph(label)
}

func (g *Graph) GetOrCreateNodeWithSubGraph(node string, subGraph string) int {
	return g.CreateSubGraph(subGraph).GetOrCreateNode(node)
}

func (g *Graph) Root() *Graph {
	if g.parent != nil {
		return g.parent.Root()
	}
	return g
}

func (g *Graph) AddEdgeInRoot(from, to string) {
	g.Root().AddEdgeByLabel(from, to)
}

func (g *Graph) AddEdgeWithSubGraph(from, to string, subGraph string) {
	g.CreateSubGraph(subGraph).AddEdgeByLabel(from, to)
}

// GetOrCreateNode returns the id of the node with the given label if it
func (g *Graph) GetOrCreateNode(label string) int {
	id, ok := g.NodeExisted(label)
	if ok {
		return id
	}

	if g.parent != nil {
		id, ok := g.parent.NodeExisted(label)
		if ok {
			return id
		}
	}

	for _, sub := range g.subGraphs {
		id, ok := sub.NodeExisted(label)
		if ok {
			return id
		}
	}

	return g.AddNode(label)
}

func (g *Graph) GetNodeByID(id int) *node {
	return g.registeredNodes[id]
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

func (g *Graph) GetNodesByLabel(label string) []*node {
	var ret []*node
	for _, node := range g.nodes {
		if node.label == label {
			ret = append(ret, node)
		}
	}
	return ret
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

// AddEdgeByLabel adds a new edge between the nodes with the given
func (g *Graph) AddEdgeByLabel(from, to string, label ...string) int {
	fromNode := g.GetOrCreateNode(from)
	toNode := g.GetOrCreateNode(to)
	return g.AddEdge(fromNode, toNode, strings.Join(label, ""))
}

// AddEdge adds a new edge between the given nodes with the specified
// label and returns an id for the new edge.
func (g *Graph) AddEdge(from, to int, label string) int {
	fromNode := g.registeredNodes[from]
	toNode := g.registeredNodes[to]
	id := g.idGetter()
	edg := CreateEdge(fromNode, toNode, label)
	if g.edges == nil {
		g.edges = make(map[int]*Edge)
	}
	g.edges[id] = &edg
	g.registeredEdges[id] = &edg
	return id
}

// GetEdges returns the ids of the edges between the given nodes.
func (g *Graph) GetEdges(from, to int) []int {
	var ret []int
	for id, edge := range g.edges {
		if edge.from.id == from && edge.to.id == to {
			ret = append(ret, id)
		}
	}
	return ret
}

func (g *Graph) GetEdge(id int) *Edge {
	if edge, ok := g.edges[id]; ok {
		return edge
	}
	return nil
}

// AddDashEdge adds a new edge between the given nodes with the specified
// label and returns an id for the new edge.
// style
func (g *Graph) AddDashEdge(from, to int, label string) int {
	fromNode := g.registeredNodes[from]
	toNode := g.registeredNodes[to]
	id := g.idGetter()
	edg := CreateEdge(fromNode, toNode, label)
	edg.attributes.set("style", "dashed")
	if g.edges == nil {
		g.edges = make(map[int]*Edge)
	}
	g.edges[id] = &edg
	g.registeredEdges[id] = &edg
	return id
}

// AddDashEdge adds a new edge between the given nodes with the specified
// label and returns an id for the new edge.
// style
func (g *Graph) AddDashEdgeWithoutArrowHead(from, to int, label string) int {
	fromNode := g.registeredNodes[from]
	toNode := g.registeredNodes[to]
	id := g.idGetter()
	edg := CreateEdge(fromNode, toNode, label)
	edg.attributes.set("style", "dashed")
	edg.attributes.set("dir", "none")
	if g.edges == nil {
		g.edges = make(map[int]*Edge)
	}
	g.edges[id] = &edg
	g.registeredEdges[id] = &edg
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
	if name == "" {
		name = "key"
	}
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

func (g *Graph) FindNode(name string) *node {
	for _, node := range g.nodes {
		if node.label == name {
			return node
		}
	}
	return nil
}

func (g *Graph) HasEdge(n1, n2 *node) bool {
	for _, edge := range g.edges {
		if edge.from == n1 && edge.to == n2 {
			return true
		}
	}
	return false
}

func (g *Graph) generateDot(indent int, w io.Writer) int {
	if g.IsSubGraph() {
		fmt.Fprintf(g.drawIndent(w, indent), "subgraph cluster_%v ", g.idGetter())
	} else {
		if !g.drawMultiEdges {
			fmt.Fprint(w, "strict ")
		}
		if g.directed {
			fmt.Fprint(w, "digraph ")
		} else {
			fmt.Fprint(w, "graph ")
		}
	}

	fmt.Fprintln(w, "{")
	indent++

	for _, sub := range g.subGraphs {
		indent = sub.generateDot(indent, w)
	}

	for graphAttribs := g.graphAttributes.iterate(); graphAttribs.hasMore(); {
		name, value := graphAttribs.next()
		fmt.Fprintf(g.drawIndent(w, indent), "%v = %#v;\n", name, value)
	}
	for nodeAttribs := g.nodeAttributes.iterate(); nodeAttribs.hasMore(); {
		name, value := nodeAttribs.next()
		fmt.Fprintf(g.drawIndent(w, indent), "node [ %v = %#v ]\n", name, value)
	}
	for edgeAttribs := g.edgeAttributes.iterate(); edgeAttribs.hasMore(); {
		name, value := edgeAttribs.next()
		fmt.Fprintf(g.drawIndent(w, indent), "edge [ %v = %#v ]\n", name, value)
	}
	for _, node := range g.nodes {
		g.drawIndent(w, indent)
		node.generateDOT(w)
		fmt.Fprintln(w)
	}
	for _, r := range g.sameRank {
		fmt.Fprint(g.drawIndent(w, indent), "  {rank=same; ")
		for _, x := range r {
			fmt.Fprintf(w, "%v; ", x.name())
		}
		fmt.Fprintln(w, "}")
	}
	for _, edge := range g.edges {
		g.drawIndent(w, indent)
		edge.generateDOT(w, g.directed)
		fmt.Fprintln(w)
	}
	fmt.Fprintln(g.drawIndent(w, indent-1), "}")
	indent--
	return indent
}

func (g *Graph) drawIndent(w io.Writer, indent int) io.Writer {
	fmt.Fprint(w, strings.Repeat(" ", indent*4))
	return w
}

func (g *Graph) GenerateDOT(w io.Writer) {
	g.generateDot(0, w)
}

func (g *Graph) GenerateDOTString() string {
	var buf bytes.Buffer
	g.GenerateDOT(&buf)
	return buf.String()
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

func (a *attributes) get(name string) string {
	if a.attributeMap == nil {
		return ""
	}
	if value, ok := a.attributeMap[name]; ok {
		return value
	}
	return ""
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
	nexts      []int
	prevs      []int
	attributes attributes
}

func CreateNode(id int, label string) node {
	return node{id: id, label: label}
}

func (n node) ID() int {
	return n.id
}

func (n node) Prevs() []int {
	return n.prevs
}

func (n node) Nexts() []int {
	return n.nexts
}

func NodeName(id int) string {
	return fmt.Sprintf("n%v", id)
}

func (n node) name() string {
	return NodeName(n.id)
}

func (n node) generateDOT(w io.Writer) {
	fmt.Fprintf(w, "%v [label=%#v", n.name(), n.label)
	for attribs := n.attributes.iterate(); attribs.hasMore(); {
		name, value := attribs.next()
		fmt.Fprintf(w, ", %v=%#v", name, value)
	}
	fmt.Fprint(w, "]")
}

type Edge struct {
	from       *node
	to         *node
	Label      string
	attributes attributes
}

func CreateEdge(from, to *node, label string) Edge {
	from.nexts = append(from.nexts, to.id)
	to.prevs = append(to.prevs, from.id)
	return Edge{from: from, to: to, Label: label}
}

func (e Edge) generateDOT(w io.Writer, directed bool) {
	edgeOp := "--"
	if directed {
		edgeOp = "->"
	}
	fmt.Fprintf(w, "%v %v %v [label=%#v", e.from.name(), edgeOp, e.to.name(), e.Label)
	for attribs := e.attributes.iterate(); attribs.hasMore(); {
		name, value := attribs.next()
		fmt.Fprintf(w, ", %v=%#v", name, value)
	}
	fmt.Fprint(w, "]")
}

func (e Edge) Attribute(name string) string {
	return e.attributes.get(name)
}

func (e Edge) From() *node {
	return e.from
}

func (e Edge) To() *node {
	return e.to
}

// IsConnected checks if there is a path from node 'from' to node 'to'
// Uses depth-first search to find connectivity
func (g *Graph) IsConnected(fromLabel, toLabel string) bool {
	fromNodes := g.GetNodesByLabel(fromLabel)
	toNodes := g.GetNodesByLabel(toLabel)

	if len(fromNodes) == 0 || len(toNodes) == 0 {
		return false
	}

	visited := make(map[int]bool)

	// Check if any fromNode can reach any toNode
	for _, fromNode := range fromNodes {
		for _, toNode := range toNodes {
			if g.dfs(fromNode.id, toNode.id, visited) {
				return true
			}
		}
	}

	return false
}

// dfs performs depth-first search to find path from start to target
func (g *Graph) dfs(start, target int, visited map[int]bool) bool {
	if start == target {
		return true
	}

	visited[start] = true
	startNode := g.registeredNodes[start]

	// Check direct connections
	for _, nextID := range startNode.nexts {
		if !visited[nextID] {
			if g.dfs(nextID, target, visited) {
				return true
			}
		}
	}

	// For undirected graphs, also check previous connections
	if !g.directed {
		for _, prevID := range startNode.prevs {
			if !visited[prevID] {
				if g.dfs(prevID, target, visited) {
					return true
				}
			}
		}
	}

	return false
}

// GetEdgeLabel returns the label of the edge between two nodes
// If multiple edges exist, returns the first one found
func (g *Graph) GetEdgeLabel(fromLabel, toLabel string) string {
	fromNodes := g.GetNodesByLabel(fromLabel)
	toNodes := g.GetNodesByLabel(toLabel)

	if len(fromNodes) == 0 || len(toNodes) == 0 {
		return ""
	}

	for _, fromNode := range fromNodes {
		for _, toNode := range toNodes {
			for _, edge := range g.edges {
				if edge.from.id == fromNode.id && edge.to.id == toNode.id {
					return edge.Label
				}
			}
		}
	}

	return ""
}

// GetAllEdgeLabels returns all labels of edges between two nodes
func (g *Graph) GetAllEdgeLabels(fromLabel, toLabel string) []string {
	fromNodes := g.GetNodesByLabel(fromLabel)
	toNodes := g.GetNodesByLabel(toLabel)

	if len(fromNodes) == 0 || len(toNodes) == 0 {
		return nil
	}

	var labels []string
	for _, fromNode := range fromNodes {
		for _, toNode := range toNodes {
			for _, edge := range g.edges {
				if edge.from.id == fromNode.id && edge.to.id == toNode.id {
					labels = append(labels, edge.Label)
				}
			}
		}
	}

	return labels
}

// HasEdgeWithLabel checks if there is an edge between two nodes with the specified label
func (g *Graph) HasEdgeWithLabel(fromLabel, toLabel, expectedLabel string) bool {
	fromNodes := g.GetNodesByLabel(fromLabel)
	toNodes := g.GetNodesByLabel(toLabel)

	if len(fromNodes) == 0 || len(toNodes) == 0 {
		return false
	}

	for _, fromNode := range fromNodes {
		for _, toNode := range toNodes {
			for _, edge := range g.edges {
				if edge.from.id == fromNode.id && edge.to.id == toNode.id && edge.Label == expectedLabel {
					return true
				}
			}
		}
	}

	return false
}

func (g *Graph) HasEdgeContainLabel(fromLabel, toLabel, expectedLabel string) bool {
	fromNodes := g.GetNodesByLabel(fromLabel)

	if len(fromNodes) == 0 {
		return false
	}
	for _, fromNode := range fromNodes {
		for _, next := range fromNode.nexts {
			toNode := g.GetNodeByID(next)
			if toNode.label == toLabel {
				return true
			}
		}
	}
	return false
}

// GetNodeLabel returns the label of a node by its label name
func (g *Graph) GetNodeLabel(nodeLabel string) string {
	id, exists := g.NodeExisted(nodeLabel)
	if !exists {
		return ""
	}

	node := g.registeredNodes[id]
	return node.label
}

// GetAllNodes returns all node labels in the graph
func (g *Graph) GetAllNodes() []string {
	var labels []string
	for _, node := range g.nodes {
		labels = append(labels, node.label)
	}
	return labels
}

// GetAllEdges returns all edges in the graph as a slice of edge info
func (g *Graph) GetAllEdges() []*Edge {
	return lo.MapToSlice(g.edges, func(key int, value *Edge) *Edge {
		return value
	})
}

// IsNeighbor checks if two nodes are directly connected (neighbors)
// Returns true if there is a direct edge between the two nodes, false otherwise
func (g *Graph) IsNeighbor(node1, node2 string) bool {
	fromNodes := g.GetNodesByLabel(node1)
	toNodes := g.GetNodesByLabel(node2)

	if len(fromNodes) == 0 || len(toNodes) == 0 {
		return false
	}

	// Check for direct edge between any fromNode and any toNode
	for _, fromNode := range fromNodes {
		for _, toNode := range toNodes {
			for _, edge := range g.edges {
				if (edge.from.id == fromNode.id && edge.to.id == toNode.id) ||
					(edge.from.id == toNode.id && edge.to.id == fromNode.id) {
					return true
				}
			}
		}
	}

	return false
}
