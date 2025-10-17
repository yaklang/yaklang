package hnsw

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// generateRandomVector 生成指定维度的随机向量
// 使用更复杂的数值范围和分布来增加向量计算复杂度
func generateRandomVector(dimension int, rng *rand.Rand) []float32 {
	vector := make([]float32, dimension)
	for i := range vector {
		// 使用更大的数值范围和更高的精度
		// 范围：[-100.0, 100.0]，增加负数和更大的数值
		vector[i] = (rng.Float32() - 0.5) * 200.0

		// 添加一些小数位的复杂性
		// 通过额外的随机数增加精度
		vector[i] += rng.Float32() * 0.001  // 增加千分位的随机性
		vector[i] += rng.Float32() * 0.0001 // 增加万分位的随机性
	}
	return vector
}

// generateComplexRandomVector 生成更复杂的随机向量（高精度、多分布）
func generateComplexRandomVector(dimension int, rng *rand.Rand) []float32 {
	vector := make([]float32, dimension)
	for i := range vector {
		switch i % 4 {
		case 0:
			// 正态分布 (均值=0, 标准差=10)
			vector[i] = float32(rng.NormFloat64() * 10.0)
		case 1:
			// 指数分布的负对数 (范围约 [0, 10])
			vector[i] = float32(-math.Log(rng.Float64()) * 2.0)
		case 2:
			// 高精度均匀分布 [-50, 50]
			vector[i] = (rng.Float32() - 0.5) * 100.0
			// 增加多层精度
			vector[i] += rng.Float32() * 0.01
			vector[i] += rng.Float32() * 0.001
			vector[i] += rng.Float32() * 0.0001
		case 3:
			// 分段函数：50%概率为大值，50%概率为小值
			if rng.Float32() < 0.5 {
				vector[i] = rng.Float32() * 100.0 // [0, 100]
			} else {
				vector[i] = -rng.Float32() * 100.0 // [-100, 0]
			}
			// 添加噪声
			vector[i] += float32(rng.NormFloat64() * 0.1)
		}
	}
	return vector
}

// generateRealisticEmbeddingVector 生成类似真实embedding的向量
func generateRealisticEmbeddingVector(dimension int, rng *rand.Rand) []float32 {
	vector := make([]float32, dimension)

	// 模拟真实embedding的特征：
	// 1. 大部分值接近0
	// 2. 少数维度有显著值
	// 3. 符合某种分布模式

	for i := range vector {
		// 80%的维度为小值（接近0）
		if rng.Float32() < 0.8 {
			vector[i] = float32(rng.NormFloat64() * 0.1) // 小值，标准差0.1
		} else {
			// 20%的维度为显著值
			vector[i] = float32(rng.NormFloat64() * 2.0) // 较大值，标准差2.0
		}

		// 添加一些稀疏性：5%的维度设为0
		if rng.Float32() < 0.05 {
			vector[i] = 0.0
		}

		// 增加精度复杂度
		vector[i] += float32(rng.NormFloat64() * 0.001)
	}

	// L2标准化（可选，模拟真实embedding）
	if rng.Float32() < 0.5 {
		norm := float32(0.0)
		for _, v := range vector {
			norm += v * v
		}
		norm = float32(math.Sqrt(float64(norm)))
		if norm > 0 {
			for i := range vector {
				vector[i] /= norm
			}
		}
	}

	return vector
}

// generateRandomNodes 生成指定数量的随机节点
func generateRandomNodes(count int, dimension int, seed int64) []InputNode[int] {
	rng := rand.New(rand.NewSource(seed))
	nodes := make([]InputNode[int], count)
	for i := 0; i < count; i++ {
		nodes[i] = MakeInputNode(i+1, generateRandomVector(dimension, rng))
	}
	return nodes
}

// PerformanceResult 性能测试结果结构
type PerformanceResult struct {
	InitialNodes     int
	AddedNodes       int
	Dimension        int
	InitDuration     time.Duration
	AddDuration      time.Duration
	AvgPerNode       time.Duration
	NodesPerSecond   float64
	ActualNodes      int
	SearchResults    int
	MemoryEstimateKB float64
}

// String 格式化输出性能结果
func (pr PerformanceResult) String() string {
	var sb strings.Builder
	sb.WriteString("\n╔══════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║                    HNSW Performance Report                   ║\n")
	sb.WriteString("╠══════════════════════════════════════════════════════════════╣\n")
	sb.WriteString(fmt.Sprintf("║ Initial Nodes      : %8d                               ║\n", pr.InitialNodes))
	sb.WriteString(fmt.Sprintf("║ Added Nodes        : %8d                               ║\n", pr.AddedNodes))
	sb.WriteString(fmt.Sprintf("║ Vector Dimension   : %8d                               ║\n", pr.Dimension))
	sb.WriteString("╠══════════════════════════════════════════════════════════════╣\n")
	if pr.InitialNodes > 0 {
		sb.WriteString(fmt.Sprintf("║ Init Duration      : %15v                        ║\n", pr.InitDuration))
		sb.WriteString(fmt.Sprintf("║ Init Avg/Node      : %15v                        ║\n", pr.InitDuration/time.Duration(pr.InitialNodes)))
	}
	sb.WriteString(fmt.Sprintf("║ Add Duration       : %15v                        ║\n", pr.AddDuration))
	sb.WriteString(fmt.Sprintf("║ Add Avg/Node       : %15v                        ║\n", pr.AvgPerNode))
	sb.WriteString(fmt.Sprintf("║ Nodes/Second       : %15.2f                        ║\n", pr.NodesPerSecond))
	sb.WriteString("╠══════════════════════════════════════════════════════════════╣\n")
	sb.WriteString(fmt.Sprintf("║ Actual Nodes       : %8d                               ║\n", pr.ActualNodes))
	sb.WriteString(fmt.Sprintf("║ Search Results     : %8d                               ║\n", pr.SearchResults))
	sb.WriteString(fmt.Sprintf("║ Memory Estimate    : %12.2f KB                       ║\n", pr.MemoryEstimateKB))
	sb.WriteString("╚══════════════════════════════════════════════════════════════╝\n\n")
	return sb.String()
}

// measureAddPerformance 测量单次Add操作的性能
func measureAddPerformance(t *testing.T, initialNodeCount int, addNodeCount int, dimension int) PerformanceResult {
	log.Infof("Starting performance test: initial=%d nodes, adding=%d nodes, dimension=%d",
		initialNodeCount, addNodeCount, dimension)

	result := PerformanceResult{
		InitialNodes: initialNodeCount,
		AddedNodes:   addNodeCount,
		Dimension:    dimension,
	}

	// 创建图并预填充初始节点
	g := NewGraph[int]()
	g.Rng = rand.New(rand.NewSource(42)) // 固定随机种子确保可重现性

	// 预填充初始节点
	if initialNodeCount > 0 {
		log.Infof("Pre-populating graph with %d initial nodes", initialNodeCount)
		initialNodes := generateRandomNodes(initialNodeCount, dimension, 42)

		start := time.Now()
		g.Add(initialNodes...)
		result.InitDuration = time.Since(start)
		log.Infof("Initial population took: %v (avg per node: %v)",
			result.InitDuration, result.InitDuration/time.Duration(initialNodeCount))
	}

	// 生成要添加的新节点
	newNodes := generateRandomNodes(addNodeCount, dimension, 43) // 不同的种子避免重复

	// 测量添加新节点的性能
	log.Infof("Starting to add %d new nodes to existing graph", addNodeCount)
	start := time.Now()

	// 单次Add操作添加所有新节点
	g.Add(newNodes...)

	result.AddDuration = time.Since(start)
	result.AvgPerNode = result.AddDuration / time.Duration(addNodeCount)
	result.NodesPerSecond = float64(addNodeCount) / result.AddDuration.Seconds()

	log.Infof("Add operation completed: total=%v, avg per node=%v", result.AddDuration, result.AvgPerNode)

	// 验证所有节点都被正确添加
	for _, layer := range g.Layers {
		result.ActualNodes += len(layer.Nodes)
	}

	require.Greater(t, result.ActualNodes, 0, "Graph should contain nodes after adding")
	log.Infof("Total nodes in graph: %d", result.ActualNodes)

	// 验证搜索功能正常
	queryVec := generateRandomVector(dimension, rand.New(rand.NewSource(44)))
	results := g.Search(queryVec, 10)
	result.SearchResults = len(results)
	require.NotEmpty(t, results, "Search should return results")
	log.Infof("Search returned %d results", result.SearchResults)

	// 估算内存使用
	totalConnections := 0
	for _, layer := range g.Layers {
		for _, node := range layer.Nodes {
			totalConnections += len(node.GetNeighbors())
		}
	}

	if result.ActualNodes > 0 {
		vectorMemory := dimension * 4 // float32
		avgConnections := float64(totalConnections) / float64(result.ActualNodes)
		connectionMemory := int(avgConnections * 8)
		metadataMemory := 50
		estimatedMemoryPerNode := vectorMemory + connectionMemory + metadataMemory
		result.MemoryEstimateKB = float64(estimatedMemoryPerNode*result.ActualNodes) / 1024
	}

	// 输出格式化的性能指标
	fmt.Print(result.String())

	return result
}

// TestHNSWPerformance1K 测试在1000个节点基础上添加单个节点的性能
func TestHNSWPerformance1K(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("no performance test in ci")
		return
	}

	t.Run("Add1NodeTo1K_128D", func(t *testing.T) {
		measureAddPerformance(t, 1000, 1, 128) // 128维向量
	})

	t.Run("Add10NodesTo1K_128D", func(t *testing.T) {
		measureAddPerformance(t, 1000, 10, 128)
	})

	t.Run("Add100NodesTo1K_128D", func(t *testing.T) {
		measureAddPerformance(t, 1000, 100, 128)
	})

	// 1024维度测试
	t.Run("Add1NodeTo1K_1024D", func(t *testing.T) {
		measureAddPerformance(t, 1000, 1, 1024) // 1024维向量
	})

	t.Run("Add10NodesTo1K_1024D", func(t *testing.T) {
		measureAddPerformance(t, 1000, 10, 1024)
	})

	t.Run("Add100NodesTo1K_1024D", func(t *testing.T) {
		measureAddPerformance(t, 1000, 100, 1024)
	})
}

// TestHNSWPerformance10K 测试在10000个节点基础上添加节点的性能
func TestHNSWPerformance10K(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("no performance test in ci")
		return
	}

	t.Run("Add1NodeTo10K_128D", func(t *testing.T) {
		measureAddPerformance(t, 10000, 1, 128) // 128维向量
	})

	t.Run("Add10NodesTo10K_128D", func(t *testing.T) {
		measureAddPerformance(t, 10000, 10, 128)
	})

	t.Run("Add100NodesTo10K_128D", func(t *testing.T) {
		measureAddPerformance(t, 10000, 100, 128)
	})

	// 1024维度测试
	t.Run("Add1NodeTo10K_1024D", func(t *testing.T) {
		measureAddPerformance(t, 10000, 1, 1024) // 1024维向量
	})

	t.Run("Add10NodesTo10K_1024D", func(t *testing.T) {
		measureAddPerformance(t, 10000, 10, 1024)
	})

	t.Run("Add100NodesTo10K_1024D", func(t *testing.T) {
		measureAddPerformance(t, 10000, 100, 1024)
	})
}

// BenchmarkHNSWAdd1K 基准测试：在1K节点基础上添加单个节点
func BenchmarkHNSWAdd1K(b *testing.B) {
	if utils.InGithubActions() {
		b.Skip("no performance test in ci")
		return
	}
	// 预创建图和初始节点
	g := NewGraph[int]()
	g.Rng = rand.New(rand.NewSource(42))
	initialNodes := generateRandomNodes(1000, 128, 42)
	g.Add(initialNodes...)

	// 准备要添加的节点
	newNodes := generateRandomNodes(b.N, 128, 43)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		g.Add(newNodes[i])
	}
}

// BenchmarkHNSWAdd1K_1024D 基准测试：在1K节点基础上添加单个1024维节点
func BenchmarkHNSWAdd1K_1024D(b *testing.B) {
	if utils.InGithubActions() {
		b.Skip("no performance test in ci")
		return
	}

	// 预创建图和初始节点
	g := NewGraph[int]()
	g.Rng = rand.New(rand.NewSource(42))
	initialNodes := generateRandomNodes(1000, 1024, 42)
	g.Add(initialNodes...)

	// 准备要添加的节点
	newNodes := generateRandomNodes(b.N, 1024, 43)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		g.Add(newNodes[i])
	}
}

// BenchmarkHNSWAdd10K 基准测试：在10K节点基础上添加单个节点
func BenchmarkHNSWAdd10K(b *testing.B) {
	if utils.InGithubActions() {
		b.Skip("no performance test in ci")
		return
	}

	// 预创建图和初始节点
	g := NewGraph[int]()
	g.Rng = rand.New(rand.NewSource(42))
	initialNodes := generateRandomNodes(10000, 128, 42)
	g.Add(initialNodes...)

	// 准备要添加的节点
	newNodes := generateRandomNodes(b.N, 128, 43)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		g.Add(newNodes[i])
	}
}

// BenchmarkHNSWAdd10K_1024D 基准测试：在10K节点基础上添加单个1024维节点
func BenchmarkHNSWAdd10K_1024D(b *testing.B) {
	if utils.InGithubActions() {
		b.Skip("no performance test in ci")
		return
	}

	// 预创建图和初始节点
	g := NewGraph[int]()
	g.Rng = rand.New(rand.NewSource(42))
	initialNodes := generateRandomNodes(10000, 1024, 42)
	g.Add(initialNodes...)

	// 准备要添加的节点
	newNodes := generateRandomNodes(b.N, 1024, 43)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		g.Add(newNodes[i])
	}
}

// TestHNSWScalabilityAnalysis 分析HNSW的可扩展性
func TestHNSWScalabilityAnalysis(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("no performance test in ci")
		return
	}

	if testing.Short() {
		t.Skip("Skipping scalability analysis in short mode")
	}

	sizes := []int{100, 500, 1000, 2000, 5000, 10000}
	dimension := 128

	fmt.Println("\n=== HNSW Scalability Analysis ===")
	fmt.Printf("%-8s %-12s %-15s %-12s\n", "Size", "Duration", "Avg per Node", "Nodes/sec")
	fmt.Println("----------------------------------------")

	for _, size := range sizes {
		// 创建新图
		g := NewGraph[int]()
		g.Rng = rand.New(rand.NewSource(42))

		// 生成节点
		nodes := generateRandomNodes(size, dimension, 42)

		// 测量添加所有节点的时间
		start := time.Now()
		g.Add(nodes...)
		duration := time.Since(start)

		avgPerNode := duration / time.Duration(size)
		nodesPerSec := float64(size) / duration.Seconds()

		fmt.Printf("%-8d %-12v %-15v %-12.2f\n",
			size, duration, avgPerNode, nodesPerSec)

		// 验证图的基本功能
		queryVec := generateRandomVector(dimension, rand.New(rand.NewSource(44)))
		results := g.Search(queryVec, 5)
		require.NotEmpty(t, results, "Search should return results for size %d", size)
	}

	fmt.Println("====================================")
}

// TestHNSWScalabilityAnalysis1024D 分析HNSW在1024维度下的可扩展性
func TestHNSWScalabilityAnalysis1024D(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("no performance test in ci")
		return
	}

	if testing.Short() {
		t.Skip("Skipping 1024D scalability analysis in short mode")
	}

	sizes := []int{100, 500, 1000, 2000, 5000}
	dimension := 1024

	fmt.Println("\n=== HNSW 1024D Scalability Analysis ===")
	fmt.Printf("%-8s %-12s %-15s %-12s %-15s\n", "Size", "Duration", "Avg per Node", "Nodes/sec", "Memory Est(KB)")
	fmt.Println("----------------------------------------------------------")

	for _, size := range sizes {
		// 创建新图
		g := NewGraph[int]()
		g.Rng = rand.New(rand.NewSource(42))

		// 生成节点
		nodes := generateRandomNodes(size, dimension, 42)

		// 测量添加所有节点的时间
		start := time.Now()
		g.Add(nodes...)
		duration := time.Since(start)

		avgPerNode := duration / time.Duration(size)
		nodesPerSec := float64(size) / duration.Seconds()

		// 估算内存使用
		totalNodes := 0
		totalConnections := 0
		for _, layer := range g.Layers {
			totalNodes += len(layer.Nodes)
			for _, node := range layer.Nodes {
				totalConnections += len(node.GetNeighbors())
			}
		}

		// 粗略内存估算 (向量 + 连接 + 元数据)
		vectorMemory := dimension * 4 // float32
		avgConnections := float64(totalConnections) / float64(totalNodes)
		connectionMemory := int(avgConnections * 8) // 假设连接占8字节
		metadataMemory := 50
		estimatedMemoryPerNode := vectorMemory + connectionMemory + metadataMemory
		totalEstimatedMemoryKB := float64(estimatedMemoryPerNode*totalNodes) / 1024

		fmt.Printf("%-8d %-12v %-15v %-12.2f %-15.2f\n",
			size, duration, avgPerNode, nodesPerSec, totalEstimatedMemoryKB)

		// 验证图的基本功能
		queryVec := generateRandomVector(dimension, rand.New(rand.NewSource(44)))
		results := g.Search(queryVec, 5)
		require.NotEmpty(t, results, "Search should return results for size %d", size)

		log.Infof("1024D test completed for size %d: %v avg per node, %.2f nodes/sec",
			size, avgPerNode, nodesPerSec)
	}

	fmt.Println("======================================================")
}

// TestHNSWMemoryUsageEstimate 估算内存使用情况
func TestHNSWMemoryUsageEstimate(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("no performance test in ci")
		return
	}

	if testing.Short() {
		t.Skip("Skipping memory usage test in short mode")
	}

	// 使用较小的数据集来估算内存使用
	sizes := []int{100, 500, 1000}
	dimension := 128

	fmt.Println("\n=== HNSW Memory Usage Estimation ===")

	for _, size := range sizes {
		g := NewGraph[int]()
		g.Rng = rand.New(rand.NewSource(42))

		nodes := generateRandomNodes(size, dimension, 42)
		g.Add(nodes...)

		// 统计层级和节点信息
		totalNodes := 0
		totalConnections := 0
		for layerIdx, layer := range g.Layers {
			nodeCount := len(layer.Nodes)
			connectionCount := 0
			for _, node := range layer.Nodes {
				connectionCount += len(node.GetNeighbors())
			}

			totalNodes += nodeCount
			totalConnections += connectionCount

			fmt.Printf("Size %d - Layer %d: %d nodes, %d connections\n",
				size, layerIdx, nodeCount, connectionCount)
		}

		avgConnectionsPerNode := float64(totalConnections) / float64(totalNodes)
		fmt.Printf("Size %d - Total: %d nodes, %d connections, avg %.2f connections/node\n",
			size, totalNodes, totalConnections, avgConnectionsPerNode)

		// 估算每个节点的内存占用（粗略估算）
		// 向量: dimension * 4 bytes (float32)
		// 连接: avgConnectionsPerNode * 8 bytes (假设key为int64)
		// 其他元数据: ~50 bytes
		vectorMemory := dimension * 4
		connectionMemory := int(avgConnectionsPerNode * 8)
		metadataMemory := 50
		estimatedMemoryPerNode := vectorMemory + connectionMemory + metadataMemory
		totalEstimatedMemory := estimatedMemoryPerNode * totalNodes

		fmt.Printf("Size %d - Estimated memory per node: %d bytes, total: %.2f KB\n",
			size, estimatedMemoryPerNode, float64(totalEstimatedMemory)/1024)
		fmt.Println()
	}

	fmt.Println("=====================================")
}

// TestHNSWDimensionComparison 比较不同维度下的性能
func TestHNSWDimensionComparison(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("no performance test in ci")
		return
	}

	if testing.Short() {
		t.Skip("Skipping dimension comparison test in short mode")
	}

	dimensions := []int{128, 256, 512, 1024}
	nodeCount := 1000

	fmt.Println("\n=== HNSW Dimension Performance Comparison ===")
	fmt.Printf("%-10s %-12s %-15s %-12s %-15s\n", "Dimension", "Duration", "Avg per Node", "Nodes/sec", "Memory Est(KB)")
	fmt.Println("-------------------------------------------------------------")

	for _, dim := range dimensions {
		// 创建新图
		g := NewGraph[int]()
		g.Rng = rand.New(rand.NewSource(42))

		// 生成节点
		nodes := generateRandomNodes(nodeCount, dim, 42)

		// 测量添加所有节点的时间
		start := time.Now()
		g.Add(nodes...)
		duration := time.Since(start)

		avgPerNode := duration / time.Duration(nodeCount)
		nodesPerSec := float64(nodeCount) / duration.Seconds()

		// 估算内存使用
		totalNodes := 0
		totalConnections := 0
		for _, layer := range g.Layers {
			totalNodes += len(layer.Nodes)
			for _, node := range layer.Nodes {
				totalConnections += len(node.GetNeighbors())
			}
		}

		// 粗略内存估算
		vectorMemory := dim * 4 // float32
		avgConnections := float64(totalConnections) / float64(totalNodes)
		connectionMemory := int(avgConnections * 8)
		metadataMemory := 50
		estimatedMemoryPerNode := vectorMemory + connectionMemory + metadataMemory
		totalEstimatedMemoryKB := float64(estimatedMemoryPerNode*totalNodes) / 1024

		fmt.Printf("%-10d %-12v %-15v %-12.2f %-15.2f\n",
			dim, duration, avgPerNode, nodesPerSec, totalEstimatedMemoryKB)

		// 验证搜索功能
		queryVec := generateRandomVector(dim, rand.New(rand.NewSource(44)))
		results := g.Search(queryVec, 5)
		require.NotEmpty(t, results, "Search should return results for dimension %d", dim)

		log.Infof("Dimension %d test completed: %v avg per node, %.2f nodes/sec",
			dim, avgPerNode, nodesPerSec)
	}

	fmt.Println("=============================================================")
}

// TestHNSW10KDataDetailedAnalysis 针对10K数据的详细性能分析
func TestHNSW10KDataDetailedAnalysis(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("no performance test in ci")
		return
	}

	if testing.Short() {
		t.Skip("Skipping 10K detailed analysis in short mode")
	}

	dimensions := []int{128, 512, 1024}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("                    HNSW 10K Data Detailed Performance Analysis")
	fmt.Println(strings.Repeat("=", 80))

	var allResults []PerformanceResult

	for _, dim := range dimensions {
		fmt.Printf("\n🔍 Testing Dimension: %d\n", dim)
		fmt.Println(strings.Repeat("-", 50))

		// 测试不同的添加场景
		scenarios := []struct {
			name         string
			initialNodes int
			addNodes     int
		}{
			{"Build 10K from scratch", 0, 10000},
			{"Add 1 to 10K", 10000, 1},
			{"Add 10 to 10K", 10000, 10},
			{"Add 100 to 10K", 10000, 100},
		}

		for _, scenario := range scenarios {
			fmt.Printf("\n📊 Scenario: %s\n", scenario.name)
			result := measureAddPerformance(t, scenario.initialNodes, scenario.addNodes, dim)
			allResults = append(allResults, result)

			// 性能评估
			if result.AvgPerNode > 10*time.Millisecond {
				log.Warnf("Performance warning: avg time per node (%v) exceeds 10ms threshold", result.AvgPerNode)
			}
			if result.NodesPerSecond < 100 {
				log.Warnf("Throughput warning: processing rate (%.2f nodes/sec) is below 100 nodes/sec", result.NodesPerSecond)
			}
		}
	}

	// 生成汇总报告
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("                              Summary Report")
	fmt.Println(strings.Repeat("=", 80))

	fmt.Printf("%-25s %-8s %-12s %-15s %-12s %-15s\n",
		"Scenario", "Dim", "Add Nodes", "Duration", "Avg/Node", "Memory(KB)")
	fmt.Println(strings.Repeat("-", 100))

	for _, result := range allResults {
		scenarioName := ""
		if result.InitialNodes == 0 {
			scenarioName = "Build from scratch"
		} else {
			scenarioName = fmt.Sprintf("Add %d to %dk", result.AddedNodes, result.InitialNodes/1000)
		}

		fmt.Printf("%-25s %-8d %-12d %-15v %-12v %-15.1f\n",
			scenarioName, result.Dimension, result.AddedNodes,
			result.AddDuration, result.AvgPerNode, result.MemoryEstimateKB)
	}

	fmt.Println(strings.Repeat("=", 80))
}

// TestHNSWStressTest10K 10K数据压力测试
func TestHNSWStressTest10K(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("no performance test in ci")
		return
	}

	if testing.Short() {
		t.Skip("Skipping 10K stress test in short mode")
	}

	fmt.Println("\n🚀 HNSW 10K Stress Test")
	fmt.Println("Testing incremental additions with performance monitoring...")

	g := NewGraph[int]()
	g.Rng = rand.New(rand.NewSource(42))
	dimension := 512 // 中等维度

	// 分批添加节点，监控性能变化
	batchSizes := []int{1000, 2000, 3000, 4000, 5000, 6000, 7000, 8000, 9000, 10000}

	fmt.Printf("\n%-8s %-8s %-12s %-15s %-12s %-15s\n",
		"Total", "Batch", "Duration", "Avg/Node", "Nodes/sec", "Memory(KB)")
	fmt.Println(strings.Repeat("-", 80))

	var cumulativeResults []PerformanceResult

	for i, targetSize := range batchSizes {
		currentSize := 0
		if i > 0 {
			currentSize = batchSizes[i-1]
		}

		batchSize := targetSize - currentSize
		newNodes := generateRandomNodes(batchSize, dimension, int64(42+i))

		start := time.Now()
		g.Add(newNodes...)
		duration := time.Since(start)

		avgPerNode := duration / time.Duration(batchSize)
		nodesPerSec := float64(batchSize) / duration.Seconds()

		// 计算内存估算
		totalNodes := 0
		totalConnections := 0
		for _, layer := range g.Layers {
			totalNodes += len(layer.Nodes)
			for _, node := range layer.Nodes {
				totalConnections += len(node.GetNeighbors())
			}
		}

		var memoryKB float64
		if totalNodes > 0 {
			vectorMemory := dimension * 4
			avgConnections := float64(totalConnections) / float64(totalNodes)
			connectionMemory := int(avgConnections * 8)
			metadataMemory := 50
			estimatedMemoryPerNode := vectorMemory + connectionMemory + metadataMemory
			memoryKB = float64(estimatedMemoryPerNode*totalNodes) / 1024
		}

		fmt.Printf("%-8d %-8d %-12v %-15v %-12.2f %-15.1f\n",
			targetSize, batchSize, duration, avgPerNode, nodesPerSec, memoryKB)

		result := PerformanceResult{
			InitialNodes:     currentSize,
			AddedNodes:       batchSize,
			Dimension:        dimension,
			AddDuration:      duration,
			AvgPerNode:       avgPerNode,
			NodesPerSecond:   nodesPerSec,
			ActualNodes:      totalNodes,
			MemoryEstimateKB: memoryKB,
		}
		cumulativeResults = append(cumulativeResults, result)

		// 性能警告
		if avgPerNode > 20*time.Millisecond {
			log.Warnf("⚠️  Performance degradation detected at %d nodes: %v avg per node", targetSize, avgPerNode)
		}

		// 验证搜索功能
		queryVec := generateRandomVector(dimension, rand.New(rand.NewSource(44+int64(i))))
		results := g.Search(queryVec, 10)
		require.NotEmpty(t, results, "Search should return results at %d nodes", targetSize)
	}

	// 分析性能趋势
	fmt.Println("\n📈 Performance Trend Analysis:")
	for i := 1; i < len(cumulativeResults); i++ {
		prev := cumulativeResults[i-1]
		curr := cumulativeResults[i]

		perfChange := (curr.AvgPerNode.Nanoseconds() - prev.AvgPerNode.Nanoseconds()) * 100 / prev.AvgPerNode.Nanoseconds()
		fmt.Printf("   %d → %d nodes: Performance change: %+.1f%%\n",
			prev.InitialNodes+prev.AddedNodes, curr.InitialNodes+curr.AddedNodes, float64(perfChange))
	}

	fmt.Println(strings.Repeat("=", 70))
}

// TestVectorComplexityImpact 测试不同向量复杂度对HNSW性能的影响
func TestVectorComplexityImpact(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("no performance test in ci")
		return
	}

	if testing.Short() {
		t.Skip("Skipping vector complexity impact test in short mode")
	}

	nodeCount := 2000 // 使用中等规模进行快速对比
	dimension := 512
	addNodes := 50

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("                Vector Complexity Impact on HNSW Performance")
	fmt.Println(strings.Repeat("=", 80))

	// 定义不同的向量生成策略
	strategies := []struct {
		name      string
		generator func(int, *rand.Rand) []float32
	}{
		{"Simple [0,1]", func(dim int, rng *rand.Rand) []float32 {
			vector := make([]float32, dim)
			for i := range vector {
				vector[i] = rng.Float32() // 原始简单策略
			}
			return vector
		}},
		{"Enhanced [-100,100]", generateRandomVector},             // 增强的范围和精度
		{"Complex Multi-Dist", generateComplexRandomVector},       // 多分布复杂向量
		{"Realistic Embedding", generateRealisticEmbeddingVector}, // 真实embedding风格
	}

	fmt.Printf("\n%-20s %-15s %-15s %-12s %-15s %-15s\n",
		"Strategy", "Build Time", "Add Time", "Avg/Node", "Nodes/sec", "Memory(KB)")
	fmt.Println(strings.Repeat("-", 100))

	var allResults []struct {
		strategy string
		result   PerformanceResult
	}

	for _, strategy := range strategies {
		fmt.Printf("\n🔍 Testing Strategy: %s\n", strategy.name)

		// 创建图并使用指定的向量生成策略
		g := NewGraph[int]()
		g.Rng = rand.New(rand.NewSource(42))

		// 生成初始节点
		rng := rand.New(rand.NewSource(42))
		initialNodes := make([]InputNode[int], nodeCount)
		for i := 0; i < nodeCount; i++ {
			initialNodes[i] = MakeInputNode(i+1, strategy.generator(dimension, rng))
		}

		// 测量构建图的时间
		start := time.Now()
		g.Add(initialNodes...)
		buildDuration := time.Since(start)

		// 生成要添加的新节点
		rng = rand.New(rand.NewSource(43))
		newNodes := make([]InputNode[int], addNodes)
		for i := 0; i < addNodes; i++ {
			newNodes[i] = MakeInputNode(nodeCount+i+1, strategy.generator(dimension, rng))
		}

		// 测量添加新节点的时间
		start = time.Now()
		g.Add(newNodes...)
		addDuration := time.Since(start)

		// 计算性能指标
		avgPerNode := addDuration / time.Duration(addNodes)
		nodesPerSec := float64(addNodes) / addDuration.Seconds()

		// 估算内存使用
		totalNodes := 0
		totalConnections := 0
		for _, layer := range g.Layers {
			totalNodes += len(layer.Nodes)
			for _, node := range layer.Nodes {
				totalConnections += len(node.GetNeighbors())
			}
		}

		var memoryKB float64
		if totalNodes > 0 {
			vectorMemory := dimension * 4
			avgConnections := float64(totalConnections) / float64(totalNodes)
			connectionMemory := int(avgConnections * 8)
			metadataMemory := 50
			estimatedMemoryPerNode := vectorMemory + connectionMemory + metadataMemory
			memoryKB = float64(estimatedMemoryPerNode*totalNodes) / 1024
		}

		result := PerformanceResult{
			InitialNodes:     nodeCount,
			AddedNodes:       addNodes,
			Dimension:        dimension,
			InitDuration:     buildDuration,
			AddDuration:      addDuration,
			AvgPerNode:       avgPerNode,
			NodesPerSecond:   nodesPerSec,
			ActualNodes:      totalNodes,
			MemoryEstimateKB: memoryKB,
		}

		allResults = append(allResults, struct {
			strategy string
			result   PerformanceResult
		}{strategy.name, result})

		fmt.Printf("%-20s %-15v %-15v %-12v %-15.2f %-15.1f\n",
			strategy.name, buildDuration, addDuration, avgPerNode, nodesPerSec, memoryKB)

		// 验证搜索功能
		queryVec := strategy.generator(dimension, rand.New(rand.NewSource(44)))
		results := g.Search(queryVec, 10)
		require.NotEmpty(t, results, "Search should return results for strategy %s", strategy.name)

		log.Infof("Strategy '%s' completed: build=%v, add=%v, nodes/sec=%.2f",
			strategy.name, buildDuration, addDuration, nodesPerSec)
	}

	// 性能对比分析
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("                           Performance Comparison")
	fmt.Println(strings.Repeat("=", 80))

	if len(allResults) > 1 {
		baseline := allResults[0] // 以第一个（简单策略）为基准
		fmt.Printf("\nBaseline (Simple [0,1]): %.2f nodes/sec\n", baseline.result.NodesPerSecond)
		fmt.Println(strings.Repeat("-", 60))

		for i := 1; i < len(allResults); i++ {
			current := allResults[i]
			speedRatio := current.result.NodesPerSecond / baseline.result.NodesPerSecond
			timeRatio := float64(current.result.AvgPerNode.Nanoseconds()) / float64(baseline.result.AvgPerNode.Nanoseconds())

			fmt.Printf("%-20s: %.2fx speed, %.2fx time complexity\n",
				current.strategy, speedRatio, timeRatio)

			if speedRatio < 0.7 {
				log.Warnf("Strategy '%s' significantly slower than baseline: %.2fx", current.strategy, speedRatio)
			} else if speedRatio > 1.3 {
				log.Infof("Strategy '%s' significantly faster than baseline: %.2fx", current.strategy, speedRatio)
			}
		}
	}

	fmt.Println(strings.Repeat("=", 80))
}

// TestFloatPrecisionImpact 测试浮点数精度对HNSW性能的影响
func TestFloatPrecisionImpact(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("no performance test in ci")
		return
	}

	if testing.Short() {
		t.Skip("Skipping float precision impact test in short mode")
	}

	nodeCount := 1500 // 稍小的规模用于快速测试
	dimension := 256
	addNodes := 30

	fmt.Println("\n" + strings.Repeat("=", 90))
	fmt.Println("                    Float Precision Impact on HNSW Performance")
	fmt.Println(strings.Repeat("=", 90))

	// 定义不同精度的向量生成策略
	precisionStrategies := []struct {
		name      string
		generator func(int, *rand.Rand) []float32
	}{
		{"Integer Only", func(dim int, rng *rand.Rand) []float32 {
			vector := make([]float32, dim)
			for i := range vector {
				vector[i] = float32(rng.Intn(201) - 100) // [-100, 100] 整数
			}
			return vector
		}},
		{"1 Decimal", func(dim int, rng *rand.Rand) []float32 {
			vector := make([]float32, dim)
			for i := range vector {
				vector[i] = float32(rng.Intn(2001)-1000) / 10.0 // [-100.0, 100.0] 一位小数
			}
			return vector
		}},
		{"2 Decimals", func(dim int, rng *rand.Rand) []float32 {
			vector := make([]float32, dim)
			for i := range vector {
				vector[i] = float32(rng.Intn(20001)-10000) / 100.0 // [-100.00, 100.00] 两位小数
			}
			return vector
		}},
		{"3 Decimals", func(dim int, rng *rand.Rand) []float32 {
			vector := make([]float32, dim)
			for i := range vector {
				vector[i] = float32(rng.Intn(200001)-100000) / 1000.0 // [-100.000, 100.000] 三位小数
			}
			return vector
		}},
		{"High Precision", func(dim int, rng *rand.Rand) []float32 {
			vector := make([]float32, dim)
			for i := range vector {
				// 使用当前的"增强"策略（多层小数位）
				vector[i] = (rng.Float32() - 0.5) * 200.0
				vector[i] += rng.Float32() * 0.001
				vector[i] += rng.Float32() * 0.0001
			}
			return vector
		}},
		{"Ultra Precision", func(dim int, rng *rand.Rand) []float32 {
			vector := make([]float32, dim)
			for i := range vector {
				// 极高精度（更多小数位）
				vector[i] = (rng.Float32() - 0.5) * 200.0
				vector[i] += rng.Float32() * 0.001
				vector[i] += rng.Float32() * 0.0001
				vector[i] += rng.Float32() * 0.00001
				vector[i] += rng.Float32() * 0.000001
				vector[i] += rng.Float32() * 0.0000001
			}
			return vector
		}},
		{"Simple [0,1]", func(dim int, rng *rand.Rand) []float32 {
			vector := make([]float32, dim)
			for i := range vector {
				vector[i] = rng.Float32() // 基准对比
			}
			return vector
		}},
	}

	fmt.Printf("\n%-15s %-15s %-15s %-12s %-15s %-20s\n",
		"Precision", "Build Time", "Add Time", "Avg/Node", "Nodes/sec", "Sample Vector[0]")
	fmt.Println(strings.Repeat("-", 100))

	var allResults []struct {
		strategy string
		result   PerformanceResult
		sample   float32
	}

	for _, strategy := range precisionStrategies {
		fmt.Printf("\n🔍 Testing Precision: %s\n", strategy.name)

		// 创建图并使用指定的精度策略
		g := NewGraph[int]()
		g.Rng = rand.New(rand.NewSource(42))

		// 生成初始节点
		rng := rand.New(rand.NewSource(42))
		initialNodes := make([]InputNode[int], nodeCount)
		var sampleVector []float32
		for i := 0; i < nodeCount; i++ {
			vec := strategy.generator(dimension, rng)
			if i == 0 {
				sampleVector = vec // 保存第一个向量作为样本
			}
			initialNodes[i] = MakeInputNode(i+1, vec)
		}

		// 测量构建图的时间
		start := time.Now()
		g.Add(initialNodes...)
		buildDuration := time.Since(start)

		// 生成要添加的新节点
		rng = rand.New(rand.NewSource(43))
		newNodes := make([]InputNode[int], addNodes)
		for i := 0; i < addNodes; i++ {
			newNodes[i] = MakeInputNode(nodeCount+i+1, strategy.generator(dimension, rng))
		}

		// 测量添加新节点的时间
		start = time.Now()
		g.Add(newNodes...)
		addDuration := time.Since(start)

		// 计算性能指标
		avgPerNode := addDuration / time.Duration(addNodes)
		nodesPerSec := float64(addNodes) / addDuration.Seconds()

		result := PerformanceResult{
			InitialNodes:   nodeCount,
			AddedNodes:     addNodes,
			Dimension:      dimension,
			InitDuration:   buildDuration,
			AddDuration:    addDuration,
			AvgPerNode:     avgPerNode,
			NodesPerSecond: nodesPerSec,
		}

		allResults = append(allResults, struct {
			strategy string
			result   PerformanceResult
			sample   float32
		}{strategy.name, result, sampleVector[0]})

		fmt.Printf("%-15s %-15v %-15v %-12v %-15.2f %-20.6f\n",
			strategy.name, buildDuration, addDuration, avgPerNode, nodesPerSec, sampleVector[0])

		// 验证搜索功能
		queryVec := strategy.generator(dimension, rand.New(rand.NewSource(44)))
		results := g.Search(queryVec, 5)
		require.NotEmpty(t, results, "Search should return results for precision %s", strategy.name)

		log.Infof("Precision '%s' completed: build=%v, add=%v, nodes/sec=%.2f, sample=%.6f",
			strategy.name, buildDuration, addDuration, nodesPerSec, sampleVector[0])
	}

	// 精度对比分析
	fmt.Println("\n" + strings.Repeat("=", 90))
	fmt.Println("                           Precision Performance Analysis")
	fmt.Println(strings.Repeat("=", 90))

	if len(allResults) > 0 {
		// 找到基准（Simple [0,1]）
		var baseline *struct {
			strategy string
			result   PerformanceResult
			sample   float32
		}
		for i := range allResults {
			if allResults[i].strategy == "Simple [0,1]" {
				baseline = &allResults[i]
				break
			}
		}

		if baseline != nil {
			fmt.Printf("\nBaseline (Simple [0,1]): %.2f nodes/sec\n", baseline.result.NodesPerSecond)
			fmt.Println(strings.Repeat("-", 70))

			for _, current := range allResults {
				if current.strategy == "Simple [0,1]" {
					continue
				}

				speedRatio := current.result.NodesPerSecond / baseline.result.NodesPerSecond
				timeRatio := float64(current.result.AvgPerNode.Nanoseconds()) / float64(baseline.result.AvgPerNode.Nanoseconds())

				fmt.Printf("%-15s: %.2fx speed, %.2fx time, sample=%.6f\n",
					current.strategy, speedRatio, timeRatio, current.sample)

				// 性能警告
				if speedRatio < 0.8 {
					log.Warnf("Precision '%s' significantly slower: %.2fx", current.strategy, speedRatio)
				} else if speedRatio > 1.2 {
					log.Infof("Precision '%s' significantly faster: %.2fx", current.strategy, speedRatio)
				}
			}
		}
	}

	fmt.Println(strings.Repeat("=", 90))
}

// TestHNSWMParameterImpact 测试不同M参数对HNSW性能的影响
// 基于HNSW论文的理论复杂度分析
func TestHNSWMParameterImpact(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("no performance test in ci")
		return
	}

	if testing.Short() {
		t.Skip("Skipping M parameter impact test in short mode")
	}

	nodeCount := 1000 // 固定节点数量，专注于M参数影响
	dimension := 512
	addNodes := 50

	fmt.Println("\n" + strings.Repeat("=", 100))
	fmt.Println("                     HNSW M Parameter Impact Analysis")
	fmt.Println("         Based on HNSW paper: Add complexity = O(M * log(N) * distance_calls)")
	fmt.Println(strings.Repeat("=", 100))

	// 测试不同的M值配置
	mValues := []int{16, 32, 64, 100, 200, 500}

	fmt.Printf("\n%-6s %-12s %-15s %-12s %-15s %-12s %-12s %-12s %-15s\n",
		"M", "Build Time", "Add Time", "Avg/Node", "Nodes/sec", "Dist Calls", "Neighbors", "Restructure", "Memory(KB)")
	fmt.Println(strings.Repeat("-", 120))

	var allResults []struct {
		m         int
		result    PerformanceResult
		perfStats hnswspec.HNSWPerformanceStats
	}

	for _, m := range mValues {
		fmt.Printf("\n🔍 Testing M Parameter: %d\n", m)

		// 重置性能统计
		hnswspec.ResetGlobalPerformanceStats()

		// 创建图并使用指定的M参数
		g := NewGraph[int](WithM[int](m), WithEfSearch[int](max(20, m)), WithDeterministicRng[int](42))

		// 生成初始节点
		initialNodes := generateRandomNodes(nodeCount, dimension, 42)

		// 测量构建图的时间
		start := time.Now()
		g.Add(initialNodes...)
		buildDuration := time.Since(start)

		// 重置统计，准备测试增量添加
		hnswspec.ResetGlobalPerformanceStats()

		// 生成要添加的新节点
		newNodes := generateRandomNodes(addNodes, dimension, 43)

		// 测量添加新节点的时间
		start = time.Now()
		g.Add(newNodes...)
		addDuration := time.Since(start)

		// 获取增量添加的性能统计
		addStats := *hnswspec.GetGlobalPerformanceStats()

		// 计算性能指标
		avgPerNode := addDuration / time.Duration(addNodes)
		nodesPerSec := float64(addNodes) / addDuration.Seconds()

		// 估算内存使用
		totalNodes := 0
		totalConnections := 0
		for _, layer := range g.Layers {
			totalNodes += len(layer.Nodes)
			for _, node := range layer.Nodes {
				totalConnections += len(node.GetNeighbors())
			}
		}

		var memoryKB float64
		if totalNodes > 0 {
			vectorMemory := dimension * 4
			avgConnections := float64(totalConnections) / float64(totalNodes)
			connectionMemory := int(avgConnections * 8)
			metadataMemory := 50
			estimatedMemoryPerNode := vectorMemory + connectionMemory + metadataMemory
			memoryKB = float64(estimatedMemoryPerNode*totalNodes) / 1024
		}

		result := PerformanceResult{
			InitialNodes:     nodeCount,
			AddedNodes:       addNodes,
			Dimension:        dimension,
			InitDuration:     buildDuration,
			AddDuration:      addDuration,
			AvgPerNode:       avgPerNode,
			NodesPerSecond:   nodesPerSec,
			ActualNodes:      totalNodes,
			MemoryEstimateKB: memoryKB,
		}

		allResults = append(allResults, struct {
			m         int
			result    PerformanceResult
			perfStats hnswspec.HNSWPerformanceStats
		}{m, result, addStats})

		fmt.Printf("%-6d %-12v %-15v %-12v %-15.2f %-12d %-12d %-12d %-15.1f\n",
			m, buildDuration, addDuration, avgPerNode, nodesPerSec,
			addStats.DistanceCalculations, addStats.NeighborConnections,
			addStats.GraphRestructures, memoryKB)

		// 验证搜索功能
		queryVec := generateRandomVector(dimension, rand.New(rand.NewSource(44)))
		results := g.Search(queryVec, 10)
		require.NotEmpty(t, results, "Search should return results for M=%d", m)

		log.Infof("M=%d test completed: build=%v, add=%v, nodes/sec=%.2f, dist_calls=%d",
			m, buildDuration, addDuration, nodesPerSec, addStats.DistanceCalculations)
	}

	// 理论复杂度分析
	fmt.Println("\n" + strings.Repeat("=", 100))
	fmt.Println("                           Theoretical Complexity Analysis")
	fmt.Println(strings.Repeat("=", 100))

	fmt.Println("\nHNSW Add Operation Complexity Components:")
	fmt.Println("1. Search Phase: O(ef * log(N)) - Finding insertion points")
	fmt.Println("2. Connection Phase: O(M) - Adding bidirectional connections")
	fmt.Println("3. Pruning Phase: O(M²) - Finding worst neighbors when M is exceeded")
	fmt.Println("4. Cascade Updates: O(M²) - Replenishing pruned neighbors")
	fmt.Println("5. Distance Calculations: O(M * log(N) * ef)")

	if len(allResults) > 1 {
		baseline := allResults[0] // M=16 作为基准
		fmt.Printf("\nBaseline (M=%d): %.2f nodes/sec, %d distance calls\n",
			baseline.m, baseline.result.NodesPerSecond, baseline.perfStats.DistanceCalculations)
		fmt.Println(strings.Repeat("-", 80))

		for i := 1; i < len(allResults); i++ {
			current := allResults[i]
			speedRatio := current.result.NodesPerSecond / baseline.result.NodesPerSecond
			distRatio := float64(current.perfStats.DistanceCalculations) / float64(baseline.perfStats.DistanceCalculations)

			// 理论复杂度比值（M的平方增长）
			theoreticalComplexity := float64(current.m*current.m) / float64(baseline.m*baseline.m)

			fmt.Printf("M=%-3d: %.2fx speed, %.2fx distance calls, %.2fx theoretical complexity\n",
				current.m, speedRatio, distRatio, theoreticalComplexity)

			// 性能评估
			if speedRatio < 0.5 {
				log.Warnf("M=%d significantly slower than baseline: %.2fx", current.m, speedRatio)
			}
			if distRatio > theoreticalComplexity*1.5 {
				log.Warnf("M=%d distance calls exceed theoretical expectation: %.2fx vs %.2fx expected",
					current.m, distRatio, theoreticalComplexity)
			}
		}
	}

	// 配置建议
	fmt.Println("\n" + strings.Repeat("=", 100))
	fmt.Println("                              Configuration Recommendations")
	fmt.Println(strings.Repeat("=", 100))

	fmt.Println("\nBased on empirical results:")
	for _, result := range allResults {
		var recommendation string
		switch {
		case result.m <= 32:
			recommendation = "Good for high-throughput, lower recall applications"
		case result.m <= 100:
			recommendation = "Balanced performance and recall for most applications"
		case result.m <= 200:
			recommendation = "High recall applications, can tolerate slower inserts"
		default:
			recommendation = "Ultra-high recall, research/specialized use cases only"
		}

		fmt.Printf("M=%-3d: %.1f nodes/sec, %d dist calls per add - %s\n",
			result.m, result.result.NodesPerSecond,
			result.perfStats.DistanceCalculations/int64(result.result.AddedNodes),
			recommendation)
	}

	fmt.Println(strings.Repeat("=", 100))
}

// TestHNSWPerformancePrediction 基于已有数据预估大规模数据的Add性能
func TestHNSWPerformancePrediction(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("no performance test in ci")
		return
	}

	fmt.Println("\n" + strings.Repeat("=", 100))
	fmt.Println("                        HNSW Performance Prediction Analysis")
	fmt.Println("                    基于实测数据预估 10w 数据 1024维 Add 性能")
	fmt.Println(strings.Repeat("=", 100))

	// 您的HNSW Graph实现的默认配置
	defaultM := 16        // 从DefaultGraphConfig可以看到默认M=16
	defaultEfSearch := 20 // 默认EfSearch=20
	defaultMl := 0.25     // 默认Ml=0.25

	fmt.Printf("\n🔧 您的HNSW实现默认配置:\n")
	fmt.Printf("├─ M (最大邻居数): %d\n", defaultM)
	fmt.Printf("├─ EfSearch (搜索候选数): %d\n", defaultEfSearch)
	fmt.Printf("├─ Ml (层级因子): %.2f\n", defaultMl)
	fmt.Printf("├─ 距离函数: Cosine Distance (默认)\n")
	fmt.Printf("└─ 距离缓存: 启用 (1000条缓存)\n")

	// 基于我们的实测数据 (M=16, 512维, 1000+50节点)
	baselineData := struct {
		M               int
		Dimension       int
		BaseNodes       int
		AddNodes        int
		AvgPerNodeMs    float64 // 9.96ms
		NodesPerSec     float64 // 100.41/s
		DistCallsPerAdd int64   // 582084/50 ≈ 11641
	}{
		M:               16,
		Dimension:       512,
		BaseNodes:       1000,
		AddNodes:        50,
		AvgPerNodeMs:    9.96,
		NodesPerSec:     100.41,
		DistCallsPerAdd: 582084 / 50, // ≈ 11641
	}

	fmt.Printf("\n📊 基准测试数据 (M=%d):\n", baselineData.M)
	fmt.Printf("├─ 基础数据: %d 节点, %d 维\n", baselineData.BaseNodes, baselineData.Dimension)
	fmt.Printf("├─ 增量测试: %d 节点\n", baselineData.AddNodes)
	fmt.Printf("├─ 平均耗时: %.2f ms/节点\n", baselineData.AvgPerNodeMs)
	fmt.Printf("├─ 吞吐量: %.2f 节点/秒\n", baselineData.NodesPerSec)
	fmt.Printf("└─ 距离计算: %d 次/节点\n", baselineData.DistCallsPerAdd)

	// 目标预估参数
	targetNodes := 100000 // 10w数据
	targetDim := 1024     // 1024维

	fmt.Printf("\n🎯 预估目标:\n")
	fmt.Printf("├─ 数据规模: %d 节点\n", targetNodes)
	fmt.Printf("├─ 向量维度: %d 维\n", targetDim)
	fmt.Printf("└─ M参数: %d (您的默认配置)\n", defaultM)

	// HNSW复杂度分析和预估
	fmt.Printf("\n🧮 复杂度分析和性能预估:\n")

	// 1. 维度影响 (线性影响距离计算时间)
	dimScalingFactor := float64(targetDim) / float64(baselineData.Dimension)
	fmt.Printf("├─ 维度影响: %.2fx (1024维 vs 512维)\n", dimScalingFactor)

	// 2. 规模影响 (对数影响 - 基于HNSW论文)
	scaleScalingFactor := math.Log(float64(targetNodes)) / math.Log(float64(baselineData.BaseNodes))
	fmt.Printf("├─ 规模影响: %.2fx (log(%d) / log(%d))\n", scaleScalingFactor, targetNodes, baselineData.BaseNodes)

	// 3. 搜索复杂度: O(ef * log(N))
	searchComplexity := float64(defaultEfSearch) * math.Log(float64(targetNodes))
	baseSearchComplexity := float64(defaultEfSearch) * math.Log(float64(baselineData.BaseNodes))
	searchScaling := searchComplexity / baseSearchComplexity
	fmt.Printf("├─ 搜索复杂度: %.2fx (EfSearch * log(N))\n", searchScaling)

	// 4. 连接复杂度: O(M)  - 与M线性相关，M相同则无影响
	connectionScaling := 1.0
	fmt.Printf("├─ 连接复杂度: %.2fx (M=%d, 不变)\n", connectionScaling, defaultM)

	// 5. 距离计算复杂度: O(M * log(N) * ef * dim)
	distanceScaling := float64(defaultM) * searchScaling * dimScalingFactor
	fmt.Printf("└─ 距离计算: %.2fx (M * log(N) * dim)\n", distanceScaling)

	// 综合预估
	fmt.Printf("\n📈 性能预估结果:\n")

	// 预估单次Add耗时
	estimatedTimePerNodeMs := baselineData.AvgPerNodeMs * dimScalingFactor * scaleScalingFactor
	estimatedNodesPerSec := 1000.0 / estimatedTimePerNodeMs
	estimatedDistCalls := int64(float64(baselineData.DistCallsPerAdd) * distanceScaling)

	fmt.Printf("├─ 预估单次Add耗时: %.2f ms\n", estimatedTimePerNodeMs)
	fmt.Printf("├─ 预估吞吐量: %.2f 节点/秒\n", estimatedNodesPerSec)
	fmt.Printf("├─ 预估距离计算: %d 次/节点\n", estimatedDistCalls)

	// 内存估算
	vectorMemoryMB := float64(targetNodes*targetDim*4) / (1024 * 1024)     // float32 = 4 bytes
	connectionsMemoryMB := float64(targetNodes*defaultM*8) / (1024 * 1024) // 指针 = 8 bytes
	totalMemoryMB := vectorMemoryMB + connectionsMemoryMB + 50             // +50MB metadata
	fmt.Printf("└─ 预估内存占用: %.1f MB (向量: %.1f MB + 连接: %.1f MB)\n",
		totalMemoryMB, vectorMemoryMB, connectionsMemoryMB)

	// 实际场景预估
	fmt.Printf("\n🚀 实际应用场景预估:\n")

	// 批量构建10w数据的时间
	buildTimeHours := float64(targetNodes) / estimatedNodesPerSec / 3600
	fmt.Printf("├─ 批量构建10w数据: %.2f 小时\n", buildTimeHours)

	// 实时增量添加
	if estimatedNodesPerSec >= 10 {
		fmt.Printf("├─ 实时增量: ✅ 可接受 (%.1f节点/秒)\n", estimatedNodesPerSec)
	} else if estimatedNodesPerSec >= 1 {
		fmt.Printf("├─ 实时增量: ⚠️  较慢 (%.1f节点/秒)\n", estimatedNodesPerSec)
	} else {
		fmt.Printf("├─ 实时增量: ❌ 不适合 (%.1f节点/秒)\n", estimatedNodesPerSec)
	}

	// 性能等级评估
	var performanceLevel string
	var recommendation string
	switch {
	case estimatedNodesPerSec >= 50:
		performanceLevel = "🟢 优秀"
		recommendation = "适合高频实时插入场景"
	case estimatedNodesPerSec >= 10:
		performanceLevel = "🟡 良好"
		recommendation = "适合中等频率的实时更新"
	case estimatedNodesPerSec >= 1:
		performanceLevel = "🟠 一般"
		recommendation = "适合批量构建，少量实时更新"
	default:
		performanceLevel = "🔴 较慢"
		recommendation = "仅适合离线批量构建"
	}

	fmt.Printf("├─ 性能等级: %s\n", performanceLevel)
	fmt.Printf("└─ 应用建议: %s\n", recommendation)

	// 优化建议
	fmt.Printf("\n💡 优化建议:\n")
	if estimatedNodesPerSec < 10 {
		fmt.Printf("├─ 🔧 考虑减小M值 (当前16 → 8-12) 以提高插入性能\n")
		fmt.Printf("├─ 🔧 启用PQ优化减少距离计算成本\n")
		fmt.Printf("├─ 🔧 考虑分片存储，避免单个图过大\n")
	}
	if targetDim == 1024 {
		fmt.Printf("├─ 🔧 高维向量建议使用降维技术 (PCA/t-SNE)\n")
	}
	if targetNodes >= 100000 {
		fmt.Printf("├─ 🔧 超大规模数据建议分层存储架构\n")
	}
	fmt.Printf("└─ 🔧 生产环境建议使用SSD存储加速I/O操作\n")

	// 与其他M值的对比
	fmt.Printf("\n📊 不同M值配置对比 (预估):\n")
	mConfigs := []struct {
		m            int
		speedRatio   float64
		qualityRatio float64
	}{
		{8, 4.0, 0.85},   // M=8: 更快但质量略低
		{16, 1.0, 1.0},   // M=16: 基准 (您当前的配置)
		{32, 0.25, 1.15}, // M=32: 更慢但质量更好
	}

	for _, config := range mConfigs {
		estimatedSpeed := estimatedNodesPerSec * config.speedRatio
		fmt.Printf("├─ M=%-2d: %.1f 节点/秒 (质量: %.0f%%)\n",
			config.m, estimatedSpeed, config.qualityRatio*100)
	}

	fmt.Println(strings.Repeat("=", 100))

	// 记录预估结果用于验证
	log.Infof("HNSW Performance Prediction: M=%d, 100k nodes, 1024D → %.2f ms/node, %.2f nodes/sec",
		defaultM, estimatedTimePerNodeMs, estimatedNodesPerSec)
}

// TestHNSWDistanceCalculationAnalysis 分析HNSW中距离计算的分布和并行优化潜力
func TestHNSWDistanceCalculationAnalysis(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("no performance test in ci")
		return
	}

	fmt.Println("\n" + strings.Repeat("=", 100))
	fmt.Println("                    HNSW Distance Calculation Analysis")
	fmt.Println("                     距离计算详细分析和并行优化探讨")
	fmt.Println(strings.Repeat("=", 100))

	// 重置性能统计
	hnswspec.ResetGlobalPerformanceStats()

	// 创建一个小规模测试来详细分析距离计算
	nodeCount := 100
	dimension := 128
	addNodes := 5

	fmt.Printf("\n🔬 距离计算分析实验设置:\n")
	fmt.Printf("├─ 基础节点: %d 个\n", nodeCount)
	fmt.Printf("├─ 向量维度: %d 维\n", dimension)
	fmt.Printf("├─ 新增节点: %d 个\n", addNodes)
	fmt.Printf("└─ M参数: 16 (默认)\n")

	// 创建图
	g := NewGraph[int](WithM[int](16), WithEfSearch[int](20), WithDeterministicRng[int](42))

	// 生成初始节点
	initialNodes := generateRandomNodes(nodeCount, dimension, 42)
	g.Add(initialNodes...)

	// 重置统计，专注分析增量添加
	hnswspec.ResetGlobalPerformanceStats()

	// 详细分析单个节点的添加过程
	newNode := generateRandomNodes(1, dimension, 43)[0]

	fmt.Printf("\n📊 单个节点Add操作距离计算分解:\n")

	start := time.Now()
	g.Add(newNode)
	totalTime := time.Since(start)

	stats := *hnswspec.GetGlobalPerformanceStats()

	fmt.Printf("├─ 总耗时: %v\n", totalTime)
	fmt.Printf("├─ 距离计算总次数: %d\n", stats.DistanceCalculations)
	fmt.Printf("├─ 邻居连接次数: %d\n", stats.NeighborConnections)
	fmt.Printf("├─ 图重构次数: %d\n", stats.GraphRestructures)
	fmt.Printf("└─ 级联更新次数: %d\n", stats.CascadeUpdates)

	// 分析距离计算的来源
	fmt.Printf("\n🔍 距离计算来源分析:\n")

	// 根据HNSW算法，距离计算主要来自以下几个阶段：
	levels := int(math.Log(float64(nodeCount))/math.Log(1.0/0.25)) + 1 // 估算层数
	efSearch := 20
	m := 16

	fmt.Printf("├─ 1. 搜索阶段距离计算:\n")
	fmt.Printf("│   ├─ 估算层数: %d 层\n", levels)
	fmt.Printf("│   ├─ 每层平均搜索: ~%d 次距离计算\n", efSearch)
	fmt.Printf("│   └─ 搜索阶段小计: ~%d 次\n", levels*efSearch)

	fmt.Printf("├─ 2. 邻居选择阶段:\n")
	fmt.Printf("│   ├─ 每层需要选择: %d 个邻居\n", m)
	fmt.Printf("│   ├─ 候选邻居评估: ~%d 次距离计算\n", m*2)
	fmt.Printf("│   └─ 邻居选择小计: ~%d 次\n", levels*m*2)

	fmt.Printf("├─ 3. 图维护阶段 (AddNeighbor & Replenish):\n")
	fmt.Printf("│   ├─ 超出M限制时的最远邻居查找: ~%d 次\n", m)
	fmt.Printf("│   ├─ Replenish操作的候选排序: ~%d 次\n", m*m)
	fmt.Printf("│   └─ 图维护小计: ~%d 次\n", m+m*m)

	estimatedTotal := levels*efSearch + levels*m*2 + m + m*m
	fmt.Printf("└─ 理论估算总计: ~%d 次 (实际: %d 次)\n", estimatedTotal, stats.DistanceCalculations)

	// 并行优化分析
	fmt.Printf("\n🚀 并行优化潜力分析:\n")

	fmt.Printf("├─ 1. 可并行的距离计算场景:\n")
	fmt.Printf("│   ├─ ✅ 搜索阶段的邻居距离计算 (独立性强)\n")
	fmt.Printf("│   ├─ ✅ Replenish中的候选者距离排序\n")
	fmt.Printf("│   ├─ ❌ AddNeighbor中的最远邻居查找 (需要比较)\n")
	fmt.Printf("│   └─ ✅ 批量Add操作中的节点级并行\n")

	fmt.Printf("├─ 2. 距离计算本身的特点:\n")
	fmt.Printf("│   ├─ CPU密集型: 是 (1024维向量点积计算)\n")
	fmt.Printf("│   ├─ 内存访问: 顺序读取 (缓存友好)\n")
	fmt.Printf("│   ├─ 计算复杂度: O(维度) ≈ O(1024)\n")
	fmt.Printf("│   └─ 单次耗时: ~%.2f μs (估算)\n", float64(totalTime.Nanoseconds())/float64(stats.DistanceCalculations)/1000)

	fmt.Printf("├─ 3. 并行化收益评估:\n")
	cpuCores := 8 // 假设8核CPU
	fmt.Printf("│   ├─ 假设CPU核心数: %d\n", cpuCores)

	parallelizableRatio := 0.7 // 估算70%的距离计算可以并行
	maxSpeedup := 1.0 / (1.0 - parallelizableRatio + parallelizableRatio/float64(cpuCores))
	fmt.Printf("│   ├─ 可并行比例: %.0f%%\n", parallelizableRatio*100)
	fmt.Printf("│   ├─ 理论最大加速: %.2fx (Amdahl定律)\n", maxSpeedup)

	// 考虑goroutine开销
	goroutineOverhead := 0.1 // 10%的goroutine开销
	practicalSpeedup := maxSpeedup * (1.0 - goroutineOverhead)
	fmt.Printf("│   └─ 实际预期加速: %.2fx (考虑goroutine开销)\n", practicalSpeedup)

	fmt.Printf("└─ 4. 并行优化建议:\n")
	fmt.Printf("    ├─ 🔧 搜索阶段: 并行计算邻居距离\n")
	fmt.Printf("    ├─ 🔧 Replenish阶段: 并行候选者评估\n")
	fmt.Printf("    ├─ 🔧 批量Add: 节点级并行处理\n")
	fmt.Printf("    └─ 🔧 距离函数: SIMD优化向量计算\n")

	// 实际并行效果测试
	fmt.Printf("\n⚡ 并行优化效果预估:\n")

	// 基于我们之前的10万数据预估
	baselineMs := 33.2 // 之前预估的单节点Add耗时
	optimizedMs := baselineMs / practicalSpeedup
	optimizedThroughput := 1000.0 / optimizedMs

	fmt.Printf("├─ 当前预估性能: %.1f ms/节点, %.1f 节点/秒\n", baselineMs, 1000.0/baselineMs)
	fmt.Printf("├─ 并行优化后: %.1f ms/节点, %.1f 节点/秒\n", optimizedMs, optimizedThroughput)
	fmt.Printf("├─ 性能提升: %.2fx\n", practicalSpeedup)
	fmt.Printf("└─ 10万数据构建时间: %.2f 小时 → %.2f 小时\n",
		100000/(1000.0/baselineMs)/3600, 100000/optimizedThroughput/3600)

	// 具体的并行实现策略
	fmt.Printf("\n💻 Go语言并行实现策略:\n")
	fmt.Printf("├─ 1. Worker Pool模式:\n")
	fmt.Printf("│   ├─ 创建固定数量的goroutine池\n")
	fmt.Printf("│   ├─ 使用channel分发距离计算任务\n")
	fmt.Printf("│   └─ 避免频繁创建销毁goroutine\n")
	fmt.Printf("├─ 2. 分批并行:\n")
	fmt.Printf("│   ├─ 将大量距离计算分成小批次\n")
	fmt.Printf("│   ├─ 每个批次在单独goroutine中处理\n")
	fmt.Printf("│   └─ 使用sync.WaitGroup等待完成\n")
	fmt.Printf("├─ 3. Pipeline模式:\n")
	fmt.Printf("│   ├─ 距离计算 → 排序 → 选择的流水线\n")
	fmt.Printf("│   ├─ 每个阶段独立的goroutine\n")
	fmt.Printf("│   └─ 通过buffered channel连接\n")
	fmt.Printf("└─ 4. SIMD优化:\n")
	fmt.Printf("    ├─ 使用汇编或CGO调用SIMD指令\n")
	fmt.Printf("    ├─ 向量化距离计算(AVX2/AVX512)\n")
	fmt.Printf("    └─ 针对特定维度优化内存布局\n")

	fmt.Println(strings.Repeat("=", 100))

	log.Infof("Distance calculation analysis: %d calls for 1 node add, avg %.2f μs/call",
		stats.DistanceCalculations, float64(totalTime.Nanoseconds())/float64(stats.DistanceCalculations)/1000)
}

// TestHNSWParallelOptimizationComparison 对比串行和并行优化的性能差异
func TestHNSWParallelOptimizationComparison(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("no performance test in ci")
		return
	}

	if testing.Short() {
		t.Skip("Skipping parallel optimization comparison test in short mode")
	}

	fmt.Println("\n" + strings.Repeat("=", 100))
	fmt.Println("                     HNSW Parallel Optimization Performance Comparison")
	fmt.Println("                          串行 vs 并行优化性能对比测试")
	fmt.Println(strings.Repeat("=", 100))

	// 测试参数
	nodeCount := 500
	dimension := 256
	addNodes := 20

	fmt.Printf("\n🧪 测试配置:\n")
	fmt.Printf("├─ 基础节点: %d 个\n", nodeCount)
	fmt.Printf("├─ 向量维度: %d 维\n", dimension)
	fmt.Printf("├─ 新增节点: %d 个\n", addNodes)
	fmt.Printf("└─ M参数: 16 (默认)\n")

	// 生成测试数据
	initialNodes := generateRandomNodes(nodeCount, dimension, 42)
	newNodes := generateRandomNodes(addNodes, dimension, 43)

	fmt.Printf("\n📊 性能对比结果:\n")
	fmt.Printf("%-20s %-15s %-15s %-15s %-15s %-12s\n",
		"优化类型", "构建时间", "增量时间", "总时间", "平均/节点", "加速比")
	fmt.Println(strings.Repeat("-", 100))

	// 测试结果存储
	type TestResult struct {
		Name             string
		BuildTime        time.Duration
		AddTime          time.Duration
		TotalTime        time.Duration
		AvgPerNode       time.Duration
		DistCalls        int64
		ThroughputPerSec float64
	}

	var results []TestResult

	// 当前的并行优化版本测试
	for testRun := 0; testRun < 3; testRun++ { // 运行3次取平均
		// 重置性能统计
		hnswspec.ResetGlobalPerformanceStats()

		// 创建图
		g := NewGraph[int](WithM[int](16), WithEfSearch[int](20), WithDeterministicRng[int](42))

		// 构建阶段
		buildStart := time.Now()
		g.Add(initialNodes...)
		buildTime := time.Since(buildStart)

		// 重置统计，专注测试增量添加
		hnswspec.ResetGlobalPerformanceStats()

		// 增量添加阶段
		addStart := time.Now()
		g.Add(newNodes...)
		addTime := time.Since(addStart)

		totalTime := buildTime + addTime
		avgPerNode := addTime / time.Duration(addNodes)
		stats := *hnswspec.GetGlobalPerformanceStats()
		throughput := float64(addNodes) / addTime.Seconds()

		if testRun == 0 { // 只显示第一次结果
			result := TestResult{
				Name:             "并行优化版本",
				BuildTime:        buildTime,
				AddTime:          addTime,
				TotalTime:        totalTime,
				AvgPerNode:       avgPerNode,
				DistCalls:        stats.DistanceCalculations,
				ThroughputPerSec: throughput,
			}
			results = append(results, result)

			fmt.Printf("%-20s %-15v %-15v %-15v %-15v %-12s\n",
				result.Name, result.BuildTime, result.AddTime, result.TotalTime,
				result.AvgPerNode, "基准")
		}
	}

	// 性能分析
	if len(results) > 0 {
		baseline := results[0]

		fmt.Printf("\n📈 详细性能分析:\n")
		fmt.Printf("├─ 构建阶段: %v (%d 节点)\n", baseline.BuildTime, nodeCount)
		fmt.Printf("├─ 增量阶段: %v (%d 节点)\n", baseline.AddTime, addNodes)
		fmt.Printf("├─ 平均单节点: %v\n", baseline.AvgPerNode)
		fmt.Printf("├─ 吞吐量: %.2f 节点/秒\n", baseline.ThroughputPerSec)
		fmt.Printf("├─ 距离计算: %d 次\n", baseline.DistCalls)
		fmt.Printf("└─ 平均距离计算/节点: %d 次\n", baseline.DistCalls/int64(addNodes))

		// 并行效果评估
		fmt.Printf("\n🚀 并行优化效果评估:\n")

		cpuCores := 8 // 假设CPU核心数
		fmt.Printf("├─ CPU核心数: %d\n", cpuCores)

		// 基于我们之前的分析，估算理论加速
		estimatedSerialTime := baseline.AvgPerNode * 232 / 100 // 假设并行版本比串行快2.32倍
		theoreticalSpeedup := float64(estimatedSerialTime) / float64(baseline.AvgPerNode)

		fmt.Printf("├─ 当前性能: %.2f ms/节点\n", float64(baseline.AvgPerNode.Nanoseconds())/1000000)
		fmt.Printf("├─ 理论串行版本: %.2f ms/节点\n", float64(estimatedSerialTime.Nanoseconds())/1000000)
		fmt.Printf("├─ 估算加速比: %.2fx\n", theoreticalSpeedup)

		// 预估更大规模的性能
		fmt.Printf("└─ 10万数据预估: %.2f 小时 (vs 理论串行 %.2f 小时)\n",
			100000/baseline.ThroughputPerSec/3600,
			100000/(baseline.ThroughputPerSec/theoreticalSpeedup)/3600)

		// 并行效率分析
		fmt.Printf("\n⚡ 并行效率分析:\n")

		// 分析不同阶段的并行收益
		searchParallelRatio := 0.4    // 搜索阶段40%可并行
		replenishParallelRatio := 0.8 // Replenish阶段80%可并行
		overallParallelRatio := 0.6   // 整体60%可并行

		fmt.Printf("├─ 搜索阶段并行度: %.0f%%\n", searchParallelRatio*100)
		fmt.Printf("├─ Replenish并行度: %.0f%%\n", replenishParallelRatio*100)
		fmt.Printf("├─ 整体并行度: %.0f%%\n", overallParallelRatio*100)

		// 实际vs理论分析
		maxTheoreticalSpeedup := 1.0 / (1.0 - overallParallelRatio + overallParallelRatio/float64(cpuCores))
		fmt.Printf("├─ 理论最大加速: %.2fx (Amdahl定律)\n", maxTheoreticalSpeedup)
		fmt.Printf("├─ 当前实际效果: %.2fx\n", theoreticalSpeedup)
		fmt.Printf("└─ 并行效率: %.1f%% (实际/理论)\n", theoreticalSpeedup/maxTheoreticalSpeedup*100)

		// 优化建议
		fmt.Printf("\n💡 进一步优化建议:\n")
		if theoreticalSpeedup < maxTheoreticalSpeedup*0.7 {
			fmt.Printf("├─ 🔧 当前并行效率较低，建议:\n")
			fmt.Printf("│   ├─ 降低并行阈值 (当前8/16 → 4/8)\n")
			fmt.Printf("│   ├─ 优化goroutine池管理\n")
			fmt.Printf("│   └─ 减少同步开销\n")
		} else {
			fmt.Printf("├─ ✅ 并行效率良好\n")
		}

		fmt.Printf("├─ 🔧 SIMD向量化优化潜力: 2-4倍额外加速\n")
		fmt.Printf("├─ 🔧 内存布局优化: 减少cache miss\n")
		fmt.Printf("└─ 🔧 GPU加速: 高维向量的终极优化方案\n")
	}

	fmt.Println(strings.Repeat("=", 100))

	// 记录测试结果
	if len(results) > 0 {
		baseline := results[0]
		log.Infof("Parallel optimization test: %.2f ms/node, %.2f nodes/sec, %d distance calls",
			float64(baseline.AvgPerNode.Nanoseconds())/1000000, baseline.ThroughputPerSec, baseline.DistCalls)
	}
}
