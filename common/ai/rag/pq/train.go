package pq

import (
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

/*
TDD:

result = make(chan []float64)

codes = pq.Train(result, pq.WithM(12), pq.WithK(256))
// codes 是 码表

qt := pq.NewQuantizer(codes)
result, err = qt.Encode(...)
if err != nil {}

*/

// TrainOptions PQ训练的配置选项
type TrainOptions struct {
	M                    int     // 子向量数量
	K                    int     // 每个子空间的聚类中心数量
	MaxIters             int     // K-Means最大迭代次数
	Tolerance            float64 // 收敛阈值
	RandomSeed           int64   // 随机种子
	ConcurrentKMeans     bool    // 是否启用并行K-Means训练
	MaxWorkers           int     // 最大并发worker数量
	ParallelDistanceCalc bool    // 是否启用K-Means内部距离计算并行化
}

// TrainOption 配置函数类型
type TrainOption func(*TrainOptions)

// WithM 设置子向量数量
func WithM(m int) TrainOption {
	return func(opts *TrainOptions) {
		opts.M = m
	}
}

// WithK 设置每个子空间的聚类中心数量
func WithK(k int) TrainOption {
	return func(opts *TrainOptions) {
		opts.K = k
	}
}

// WithMaxIters 设置K-Means最大迭代次数
func WithMaxIters(maxIters int) TrainOption {
	return func(opts *TrainOptions) {
		opts.MaxIters = maxIters
	}
}

// WithTolerance 设置收敛阈值
func WithTolerance(tolerance float64) TrainOption {
	return func(opts *TrainOptions) {
		opts.Tolerance = tolerance
	}
}

// WithRandomSeed 设置随机种子；相同的输入和种子在不同并发配置下产生相同码本。
func WithRandomSeed(seed int64) TrainOption {
	return func(opts *TrainOptions) {
		opts.RandomSeed = seed
	}
}

// WithConcurrentKMeans 启用并行K-Means训练
func WithConcurrentKMeans(enabled bool) TrainOption {
	return func(opts *TrainOptions) {
		opts.ConcurrentKMeans = enabled
	}
}

// WithMaxWorkers 设置最大并发worker数量
func WithMaxWorkers(maxWorkers int) TrainOption {
	return func(opts *TrainOptions) {
		opts.MaxWorkers = maxWorkers
	}
}

// WithParallelDistanceCalc 启用K-Means内部距离计算并行化
func WithParallelDistanceCalc(enabled bool) TrainOption {
	return func(opts *TrainOptions) {
		opts.ParallelDistanceCalc = enabled
	}
}

// Codebook 码本结构，存储所有的聚类中心
type Codebook struct {
	M            int           // 子向量数量
	K            int           // 每个子空间的聚类中心数量
	SubVectorDim int           // 每个子向量的维度
	Centroids    [][][]float64 // 码本，维度: [M][K][SubVectorDim]
}

const (
	DefaultM = 16
	DefaultK = 256
)

// Train 训练PQ模型，从输入通道读取向量数据并生成码本
func Train(input <-chan []float64, opts ...TrainOption) (*Codebook, error) {
	// 设置默认选项
	options := &TrainOptions{
		M:                    16,   // 默认16个子向量
		K:                    256,  // 默认256个聚类中心
		MaxIters:             100,  // 默认最大100次迭代
		Tolerance:            1e-6, // 默认收敛阈值
		RandomSeed:           time.Now().UnixNano(),
		ConcurrentKMeans:     true,             // 默认启用并行训练
		MaxWorkers:           runtime.NumCPU(), // 默认使用所有CPU核心
		ParallelDistanceCalc: true,             // 默认启用并行距离计算
	}

	// 应用用户提供的选项
	for _, opt := range opts {
		opt(options)
	}

	// 参数验证
	if options.M <= 0 {
		return nil, fmt.Errorf("M (number of sub-vectors) must be positive, got %d", options.M)
	}
	if options.K <= 0 || options.K > 256 {
		return nil, fmt.Errorf("K (number of centroids) must be between 1 and 256, got %d", options.K)
	}
	if options.MaxIters <= 0 {
		return nil, fmt.Errorf("MaxIters must be positive, got %d", options.MaxIters)
	}
	if options.Tolerance < 0 || math.IsNaN(options.Tolerance) || math.IsInf(options.Tolerance, 0) {
		return nil, fmt.Errorf("Tolerance must be finite and non-negative, got %v", options.Tolerance)
	}
	if options.MaxWorkers <= 0 {
		options.MaxWorkers = 1
	}

	log.Infof("Starting PQ training with M=%d, K=%d, MaxIters=%d", options.M, options.K, options.MaxIters)

	// 从通道读取所有训练数据
	var vectors [][]float64
	for vec := range input {
		if len(vec) == 0 {
			continue
		}
		vectors = append(vectors, vec)
	}

	if len(vectors) == 0 {
		return nil, fmt.Errorf("no training data received")
	}
	if len(vectors) < options.K {
		return nil, fmt.Errorf("data size %d is less than k %d", len(vectors), options.K)
	}

	// 检查向量维度一致性
	dim := len(vectors[0])
	for i, vec := range vectors {
		if len(vec) != dim {
			return nil, fmt.Errorf("vector %d has dimension %d, expected %d", i, len(vec), dim)
		}
	}

	// 检查维度是否能被M整除
	if dim%options.M != 0 {
		return nil, fmt.Errorf("vector dimension %d is not divisible by M %d", dim, options.M)
	}

	subVectorDim := dim / options.M
	log.Infof("Training with %d vectors of dimension %d, sub-vector dimension: %d", len(vectors), dim, subVectorDim)

	// 初始化码本
	codebook := &Codebook{
		M:            options.M,
		K:            options.K,
		SubVectorDim: subVectorDim,
		Centroids:    make([][][]float64, options.M),
	}

	// 选择训练模式：并行或串行
	if options.ConcurrentKMeans {
		log.Infof("Starting parallel training with %d workers", options.MaxWorkers)
		err := trainParallel(vectors, codebook, subVectorDim, options)
		if err != nil {
			return nil, err
		}
	} else {
		log.Infof("Starting sequential training")
		err := trainSequential(vectors, codebook, subVectorDim, options)
		if err != nil {
			return nil, err
		}
	}

	log.Infof("PQ training completed successfully")
	return codebook, nil
}

// trainSequential 串行训练模式（原始方法）
func trainSequential(vectors [][]float64, codebook *Codebook, subVectorDim int, options *TrainOptions) error {
	for m := 0; m < options.M; m++ {
		log.Infof("Training codebook for sub-vector %d/%d", m+1, options.M)

		// 提取当前分段的所有子向量
		subVectors := make([][]float64, len(vectors))
		start := m * subVectorDim
		end := start + subVectorDim

		for i, vec := range vectors {
			// K-Means only reads its input; keep a view instead of copying each point.
			subVectors[i] = vec[start:end]
		}

		// 运行K-Means算法
		centroids, err := trainSubspace(subVectors, m, options, options.MaxWorkers)
		if err != nil {
			return fmt.Errorf("K-Means failed for sub-vector %d: %v", m, err)
		}

		codebook.Centroids[m] = centroids
		log.Infof("Completed training for sub-vector %d, generated %d centroids", m+1, len(centroids))
	}
	return nil
}

// trainParallel 并行训练模式
func trainParallel(vectors [][]float64, codebook *Codebook, subVectorDim int, options *TrainOptions) error {
	// 工作任务结构
	type TrainingTask struct {
		SubVectorIndex int
		SubVectors     [][]float64
	}

	// 结果结构
	type TrainingResult struct {
		SubVectorIndex int
		Centroids      [][]float64
		Error          error
	}

	// 创建任务队列
	taskChan := make(chan TrainingTask, options.M)
	resultChan := make(chan TrainingResult, options.M)

	// 限制并发数量
	maxWorkers := options.MaxWorkers
	if maxWorkers > options.M {
		maxWorkers = options.M // 不能超过子向量数量
	}
	// Share the worker budget across subspaces and distance calculations.
	distanceWorkers := options.MaxWorkers / maxWorkers

	log.Infof("Using %d parallel workers for %d sub-vectors", maxWorkers, options.M)

	// 启动worker协程
	var wg sync.WaitGroup
	for w := 0; w < maxWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for task := range taskChan {
				log.Infof("Worker %d: Training sub-vector %d/%d", workerID, task.SubVectorIndex+1, options.M)

				// 运行K-Means算法
				centroids, err := trainSubspace(task.SubVectors, task.SubVectorIndex, options, distanceWorkers)

				// 发送结果
				resultChan <- TrainingResult{
					SubVectorIndex: task.SubVectorIndex,
					Centroids:      centroids,
					Error:          err,
				}

				if err == nil {
					log.Infof("Worker %d: Completed sub-vector %d, generated %d centroids", workerID, task.SubVectorIndex+1, len(centroids))
				}
			}
		}(w)
	}

	// 发送训练任务
	go func() {
		defer close(taskChan)
		for m := 0; m < options.M; m++ {
			// 提取当前分段的所有子向量
			subVectors := make([][]float64, len(vectors))
			start := m * subVectorDim
			end := start + subVectorDim

			for i, vec := range vectors {
				subVectors[i] = vec[start:end]
			}

			taskChan <- TrainingTask{
				SubVectorIndex: m,
				SubVectors:     subVectors,
			}
		}
	}()

	// 等待所有worker完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果
	completedCount := 0
	var firstErr error
	for result := range resultChan {
		if result.Error != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("K-Means failed for sub-vector %d: %v", result.SubVectorIndex, result.Error)
			}
			continue
		}

		codebook.Centroids[result.SubVectorIndex] = result.Centroids
		completedCount++

		if completedCount%4 == 0 || completedCount == options.M {
			log.Infof("Completed %d/%d sub-vectors", completedCount, options.M)
		}
	}

	return firstErr
}

func trainSubspace(data [][]float64, index int, options *TrainOptions, workers int) ([][]float64, error) {
	if !options.ParallelDistanceCalc {
		workers = 1
	}
	// Each subspace owns its random stream, so scheduling and other Train calls
	// cannot change initialization or empty-cluster recovery.
	rng := rand.New(rand.NewSource(options.RandomSeed + int64(index)))
	return kMeansWithRand(data, options.K, options.MaxIters, options.Tolerance, workers, rng)
}

// kMeans K-Means聚类算法实现
func kMeans(data [][]float64, k int, maxIters int, tolerance float64) ([][]float64, error) {
	return kMeansWithRand(data, k, maxIters, tolerance, 1, rand.New(rand.NewSource(rand.Int63())))
}

// kMeansParallel 并行K-Means聚类算法实现
func kMeansParallel(data [][]float64, k int, maxIters int, tolerance float64, numWorkers int) ([][]float64, error) {
	return kMeansWithRand(data, k, maxIters, tolerance, numWorkers, rand.New(rand.NewSource(rand.Int63())))
}

func kMeansWithRand(data [][]float64, k int, maxIters int, tolerance float64, numWorkers int, rng *rand.Rand) ([][]float64, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	if k <= 0 {
		return nil, fmt.Errorf("k must be positive")
	}
	if len(data) < k {
		return nil, fmt.Errorf("data size %d is less than k %d", len(data), k)
	}
	if maxIters <= 0 {
		return nil, fmt.Errorf("maxIters must be positive")
	}
	if tolerance < 0 || math.IsNaN(tolerance) || math.IsInf(tolerance, 0) {
		return nil, fmt.Errorf("tolerance must be finite and non-negative")
	}

	dim := len(data[0])
	if dim == 0 {
		return nil, fmt.Errorf("empty vector")
	}
	for i, point := range data {
		if len(point) != dim {
			return nil, fmt.Errorf("vector %d has dimension %d, expected %d", i, len(point), dim)
		}
		for _, value := range point {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("vector %d contains a non-finite value", i)
			}
		}
	}

	centroids := initializeKMeansPlusPlus(data, k, rng)
	assignments := make([]int, len(data))
	for i := range assignments {
		// The first assignment must trigger a centroid update, including k=1.
		assignments[i] = -1
	}
	if numWorkers > len(data) {
		numWorkers = len(data)
	}
	if numWorkers <= 0 {
		numWorkers = 1
	}
	newCentroids := make([][]float64, k)
	counts := make([]int, k)
	for i := range newCentroids {
		newCentroids[i] = make([]float64, dim)
	}

	for iter := 0; iter < maxIters; iter++ {
		// 分配步骤：将每个数据点分配到最近的聚类中心
		changed := false
		if numWorkers > 1 {
			changed = parallelAssignment(data, centroids, assignments, numWorkers)
		} else {
			for i, point := range data {
				minDist := math.MaxFloat64
				bestCluster := 0

				for j, centroid := range centroids {
					dist := euclideanDistanceSquared(point, centroid)
					if dist < minDist {
						minDist = dist
						bestCluster = j
					}
				}

				if assignments[i] != bestCluster {
					assignments[i] = bestCluster
					changed = true
				}
			}
		}

		// 如果没有分配改变，算法收敛
		if !changed {
			log.Infof("K-Means converged after %d iterations", iter+1)
			break
		}

		// 更新步骤：重新计算聚类中心
		for i := range newCentroids {
			clear(newCentroids[i])
			counts[i] = 0
		}

		// 累加每个簇中的所有点
		for i, point := range data {
			cluster := assignments[i]
			for j, val := range point {
				newCentroids[cluster][j] += val
			}
			counts[cluster]++
		}

		// 计算平均值作为新的聚类中心
		maxChange := 0.0
		for i := range newCentroids {
			if counts[i] > 0 {
				for j := range newCentroids[i] {
					newCentroids[i][j] /= float64(counts[i])
				}
			} else {
				// 如果某个簇为空，重新随机选择一个点
				randomIdx := rng.Intn(len(data))
				copy(newCentroids[i], data[randomIdx])
			}

			// 计算聚类中心的变化
			change := euclideanDistanceSquared(centroids[i], newCentroids[i])
			if change > maxChange {
				maxChange = change
			}
		}

		centroids, newCentroids = newCentroids, centroids

		// 检查收敛性
		if maxChange < tolerance {
			log.Infof("K-Means converged after %d iterations (tolerance reached)", iter+1)
			break
		}
	}

	return centroids, nil
}

// initializeKMeansPlusPlus favors points far from the centers already selected.
// Scaling weights by their maximum prevents their sum from overflowing.
func initializeKMeansPlusPlus(data [][]float64, k int, rng *rand.Rand) [][]float64 {
	centroids := make([][]float64, k)
	nearest := make([]float64, len(data))
	for i := range nearest {
		nearest[i] = math.MaxFloat64
	}
	selected := rng.Intn(len(data))
	for c := range centroids {
		centroids[c] = append([]float64(nil), data[selected]...)
		if c == k-1 {
			break
		}
		maxDistance := 0.0
		for i, point := range data {
			distance := euclideanDistanceSquared(point, centroids[c])
			if distance < nearest[i] {
				nearest[i] = distance
			}
			if nearest[i] > maxDistance {
				maxDistance = nearest[i]
			}
		}
		if maxDistance == 0 {
			// All distinct points are represented; duplicate centers are unavoidable.
			selected = rng.Intn(len(data))
			continue
		}
		total := 0.0
		for _, distance := range nearest {
			total += distance / maxDistance
		}
		threshold := rng.Float64() * total
		for i, distance := range nearest {
			if distance == 0 {
				continue
			}
			// Retain the last positive-weight point as a rounding fallback.
			selected = i
			threshold -= distance / maxDistance
			if threshold < 0 {
				break
			}
		}
	}
	return centroids
}

// parallelAssignment 并行分配数据点到最近的聚类中心
func parallelAssignment(data [][]float64, centroids [][]float64, assignments []int, numWorkers int) bool {
	dataSize := len(data)
	chunkSize := (dataSize + numWorkers - 1) / numWorkers // 向上取整

	// 用于收集是否有变化的channel
	changedChan := make(chan bool, numWorkers)

	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			start := workerID * chunkSize
			end := start + chunkSize
			if end > dataSize {
				end = dataSize
			}

			localChanged := false

			for i := start; i < end; i++ {
				point := data[i]
				minDist := math.MaxFloat64
				bestCluster := 0

				for j, centroid := range centroids {
					dist := euclideanDistanceSquared(point, centroid)
					if dist < minDist {
						minDist = dist
						bestCluster = j
					}
				}

				if assignments[i] != bestCluster {
					assignments[i] = bestCluster
					localChanged = true
				}
			}

			changedChan <- localChanged
		}(w)
	}

	wg.Wait()
	close(changedChan)

	// 检查是否有任何worker报告了变化
	changed := false
	for localChanged := range changedChan {
		if localChanged {
			changed = true
		}
	}

	return changed
}

// euclideanDistanceSquared 计算两个向量之间的欧氏距离的平方
func euclideanDistanceSquared(v1, v2 []float64) float64 {
	if len(v1) != len(v2) {
		return math.MaxFloat64
	}

	sum := 0.0
	for i := range v1 {
		diff := v1[i] - v2[i]
		sum += diff * diff
	}
	return sum
}
