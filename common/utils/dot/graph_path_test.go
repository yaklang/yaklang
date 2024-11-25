package dot_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/dot"
)

func Test_DotGraph(t *testing.T) {
	t.Run("test simple path ", func(t *testing.T) {
		/*
			strict digraph {
			rankdir = "BT";
				n1[]
				n2[]
				n3[]
				n1 -> n2
				n2 -> n3
			}
		*/
		graph := dot.New()
		graph.MakeDirected()
		graph.GraphAttribute("rankdir", "BT")
		n1 := graph.AddNode("n1")
		n2 := graph.AddNode("n2")
		n3 := graph.AddNode("n3")
		graph.AddEdge(n1, n2, "")
		graph.AddEdge(n2, n3, "")
		graph.GenerateDOT(os.Stdout)

		path := dot.GraphPathPrev(graph, n3)
		require.Equal(t, len(path), 1)
		require.Equal(t, path[0], []string{"n3", "n2", "n1"})
	})

	t.Run("test multiple path", func(t *testing.T) {
		/*
			strict digraph {
				rankdir = "BT";
				n1[]
				n2[]
				n3[]
				n4[]
				n1 -> n3
				n2 -> n3
				n3 -> n4
			}
		*/
		graph := dot.New()
		graph.MakeDirected()
		graph.GraphAttribute("rankdir", "BT")
		n1 := graph.AddNode("n1")
		n2 := graph.AddNode("n2")
		n3 := graph.AddNode("n3")
		n4 := graph.AddNode("n4")
		graph.AddEdge(n1, n3, "")
		graph.AddEdge(n2, n3, "")
		graph.AddEdge(n3, n4, "")
		graph.GenerateDOT(os.Stdout)

		path := dot.GraphPathPrev(graph, n4)
		log.Infof("path: %v", path)
		require.Equal(t, len(path), 2)
		require.Equal(t, []string{"n4", "n3", "n1"}, path[0])
		require.Equal(t, []string{"n4", "n3", "n2"}, path[1])
	})

	t.Run("test same node in diff path", func(t *testing.T) {
		/*
			strict digraph {
				rankdir = "BT";
				n1[]
				n2[]
				n3[]
				n4[]
				n5[]
				n1 -> n2
				n2 -> n3
				n3 -> n5
				n2 -> n4
				n4 -> n5
			}
		*/
		graph := dot.New()
		graph.MakeDirected()
		graph.GraphAttribute("rankdir", "BT")
		n1 := graph.AddNode("n1")
		n2 := graph.AddNode("n2")
		n3 := graph.AddNode("n3")
		n4 := graph.AddNode("n4")
		n5 := graph.AddNode("n5")
		graph.AddEdge(n1, n2, "")
		graph.AddEdge(n2, n3, "")
		graph.AddEdge(n3, n5, "")
		graph.AddEdge(n2, n4, "")
		graph.AddEdge(n4, n5, "")
		graph.GenerateDOT(os.Stdout)

		path := dot.GraphPathPrev(graph, n5)
		log.Infof("path: %v", path)
		require.Equal(t, len(path), 2)
		require.Equal(t, []string{"n5", "n3", "n2", "n1"}, path[0])
		require.Equal(t, []string{"n5", "n4", "n2", "n1"}, path[1])
	})
}

func Test_DotGraph_Negative(t *testing.T) {
	t.Run("simple with duplicate prev node", func(t *testing.T) {
		/*
			strict digraph {
			rankdir = "BT";
				n1[]
				n2[]
				n3[]
				n1 -> n2
				n1 -> n2
				n2 -> n3
			}
		*/
		graph := dot.New()
		graph.MakeDirected()
		graph.GraphAttribute("rankdir", "BT")
		n1 := graph.AddNode("n1")
		n2 := graph.AddNode("n2")
		n3 := graph.AddNode("n3")
		graph.AddEdge(n1, n2, "")
		graph.AddEdge(n1, n2, "") // duplicate edge
		graph.AddEdge(n2, n3, "")
		graph.GenerateDOT(os.Stdout)

		path := dot.GraphPathPrev(graph, n3)
		log.Infof("path: %v", path)
		require.Equal(t, len(path), 1)
		require.Equal(t, path[0], []string{"n3", "n2", "n1"})
	})

}
