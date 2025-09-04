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

// WithRandomSeed 设置随机种子
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

	// 设置随机种子
	rand.Seed(options.RandomSeed)

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
			subVectors[i] = make([]float64, subVectorDim)
			copy(subVectors[i], vec[start:end])
		}

		// 运行K-Means算法
		var centroids [][]float64
		var err error
		if options.ParallelDistanceCalc {
			centroids, err = kMeansParallel(subVectors, options.K, options.MaxIters, options.Tolerance, options.MaxWorkers)
		} else {
			centroids, err = kMeans(subVectors, options.K, options.MaxIters, options.Tolerance)
		}
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
		Start          int
		End            int
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
				var centroids [][]float64
				var err error
				if options.ParallelDistanceCalc {
					centroids, err = kMeansParallel(task.SubVectors, options.K, options.MaxIters, options.Tolerance, runtime.NumCPU())
				} else {
					centroids, err = kMeans(task.SubVectors, options.K, options.MaxIters, options.Tolerance)
				}

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
				subVectors[i] = make([]float64, subVectorDim)
				copy(subVectors[i], vec[start:end])
			}

			taskChan <- TrainingTask{
				SubVectorIndex: m,
				SubVectors:     subVectors,
				Start:          start,
				End:            end,
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
	for result := range resultChan {
		if result.Error != nil {
			return fmt.Errorf("K-Means failed for sub-vector %d: %v", result.SubVectorIndex, result.Error)
		}

		codebook.Centroids[result.SubVectorIndex] = result.Centroids
		completedCount++

		if completedCount%4 == 0 || completedCount == options.M {
			log.Infof("Completed %d/%d sub-vectors", completedCount, options.M)
		}
	}

	return nil
}

// kMeans K-Means聚类算法实现
func kMeans(data [][]float64, k int, maxIters int, tolerance float64) ([][]float64, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	if k <= 0 {
		return nil, fmt.Errorf("k must be positive")
	}
	if len(data) < k {
		return nil, fmt.Errorf("data size %d is less than k %d", len(data), k)
	}

	dim := len(data[0])

	// 初始化聚类中心：从数据中随机选择k个点
	centroids := make([][]float64, k)
	perm := rand.Perm(len(data))
	for i := 0; i < k; i++ {
		centroids[i] = make([]float64, dim)
		copy(centroids[i], data[perm[i]])
	}

	assignments := make([]int, len(data))

	for iter := 0; iter < maxIters; iter++ {
		// 分配步骤：将每个数据点分配到最近的聚类中心
		changed := false
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

		// 如果没有分配改变，算法收敛
		if !changed {
			log.Infof("K-Means converged after %d iterations", iter+1)
			break
		}

		// 更新步骤：重新计算聚类中心
		newCentroids := make([][]float64, k)
		counts := make([]int, k)

		for i := range newCentroids {
			newCentroids[i] = make([]float64, dim)
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
				randomIdx := rand.Intn(len(data))
				copy(newCentroids[i], data[randomIdx])
			}

			// 计算聚类中心的变化
			change := euclideanDistanceSquared(centroids[i], newCentroids[i])
			if change > maxChange {
				maxChange = change
			}
		}

		centroids = newCentroids

		// 检查收敛性
		if maxChange < tolerance {
			log.Infof("K-Means converged after %d iterations (tolerance reached)", iter+1)
			break
		}
	}

	return centroids, nil
}

// kMeansParallel 并行K-Means聚类算法实现
func kMeansParallel(data [][]float64, k int, maxIters int, tolerance float64, numWorkers int) ([][]float64, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	if k <= 0 {
		return nil, fmt.Errorf("k must be positive")
	}
	if len(data) < k {
		return nil, fmt.Errorf("data size %d is less than k %d", len(data), k)
	}

	dim := len(data[0])

	// 初始化聚类中心：从数据中随机选择k个点
	centroids := make([][]float64, k)
	perm := rand.Perm(len(data))
	for i := 0; i < k; i++ {
		centroids[i] = make([]float64, dim)
		copy(centroids[i], data[perm[i]])
	}

	assignments := make([]int, len(data))

	// 限制worker数量
	if numWorkers > len(data) {
		numWorkers = len(data)
	}
	if numWorkers <= 0 {
		numWorkers = 1
	}

	for iter := 0; iter < maxIters; iter++ {
		// 并行分配步骤：将每个数据点分配到最近的聚类中心
		changed := parallelAssignment(data, centroids, assignments, numWorkers)

		// 如果没有分配改变，算法收敛
		if !changed {
			log.Infof("Parallel K-Means converged after %d iterations", iter+1)
			break
		}

		// 更新步骤：重新计算聚类中心
		newCentroids := make([][]float64, k)
		counts := make([]int, k)

		for i := range newCentroids {
			newCentroids[i] = make([]float64, dim)
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
				randomIdx := rand.Intn(len(data))
				copy(newCentroids[i], data[randomIdx])
			}

			// 计算聚类中心的变化
			change := euclideanDistanceSquared(centroids[i], newCentroids[i])
			if change > maxChange {
				maxChange = change
			}
		}

		centroids = newCentroids

		// 检查收敛性
		if maxChange < tolerance {
			log.Infof("Parallel K-Means converged after %d iterations (tolerance reached)", iter+1)
			break
		}
	}

	return centroids, nil
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
