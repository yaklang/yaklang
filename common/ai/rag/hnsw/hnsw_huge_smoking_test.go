package hnsw

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

	fmt.Printf("\n%-8s %-12s %-15s %-12s %-15s\n",
		"Nodes", "Duration", "Avg/Node", "Nodes/sec", "Memory(KB)")
	fmt.Println(strings.Repeat("-", 70))

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

		fmt.Printf("%-8d %-12v %-15v %-12.2f %-15.1f\n",
			targetSize, duration, avgPerNode, nodesPerSec, memoryKB)

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
