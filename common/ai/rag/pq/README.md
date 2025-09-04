# Product Quantization (PQ) 算法实现

本模块实现了用于向量压缩的Product Quantization (PQ) 算法，可以将高维向量（如`[]float64`）压缩到极小的字节表示形式，同时保持高效的相似性搜索能力。

## 功能特性

- **高效压缩**: 将1024维的float64向量压缩到仅16字节，实现512倍的压缩比
- **快速训练**: 基于K-Means聚类的训练算法，自动学习最优的码本
- **精确搜索**: 支持非对称距离计算，可与HNSW等索引结构完美结合
- **灵活配置**: 支持自定义子向量数量(M)、聚类中心数量(K)等参数
- **批量操作**: 支持批量编码和解码操作

## 核心概念

### Product Quantization原理
PQ算法将高维向量分解为多个较小的子向量，然后为每个子空间学习一组代表向量（质心），通过组合这些质心来近似原始向量。

### 关键参数
- **M**: 子向量数量，向量被分解为M个子向量
- **K**: 每个子空间的聚类中心数量（通常为256，可用1个字节表示）
- **SubVectorDim**: 每个子向量的维度，等于`原始维度/M`

## 使用方法

### 基本使用流程

```go
package main

import (
    "github.com/yaklang/yaklang/common/ai/pq"
    "github.com/yaklang/yaklang/common/log"
)

func main() {
    // 1. 准备训练数据
    trainingData := make(chan []float64, 10000)
    go func() {
        defer close(trainingData)
        for i := 0; i < 10000; i++ {
            vector := generateVector(1024) // 生成1024维向量
            trainingData <- vector
        }
    }()

    // 2. 训练PQ模型
    codebook, err := pq.Train(trainingData, 
        pq.WithM(16),           // 16个子向量
        pq.WithK(256),          // 每个子空间256个聚类中心
        pq.WithMaxIters(50),    // 最大迭代次数
        pq.WithTolerance(1e-6), // 收敛阈值
    )
    if err != nil {
        log.Errorf("Training failed: %v", err)
        return
    }

    // 3. 创建量化器
    quantizer := pq.NewQuantizer(codebook)

    // 4. 编码向量
    vector := generateVector(1024)
    codes, err := quantizer.Encode(vector)
    if err != nil {
        log.Errorf("Encoding failed: %v", err)
        return
    }
    log.Infof("Vector compressed from %d bytes to %d bytes", 
        len(vector)*8, len(codes))

    // 5. 解码向量
    decodedVector, err := quantizer.Decode(codes)
    if err != nil {
        log.Errorf("Decoding failed: %v", err)
        return
    }

    // 6. 计算非对称距离（用于搜索）
    queryVector := generateVector(1024)
    distance, err := quantizer.AsymmetricDistance(queryVector, codes)
    if err != nil {
        log.Errorf("Distance calculation failed: %v", err)
        return
    }
    log.Infof("Asymmetric distance: %.6f", distance)
}
```

### 高级功能

#### 批量编码
```go
vectors := [][]float64{...} // 多个向量
allCodes, err := quantizer.BatchEncode(vectors)
```

#### 距离表优化
```go
// 预计算距离表以加速批量距离计算
queryVector := []float64{...}
distanceTable, err := quantizer.ComputeDistanceTable(queryVector)

// 使用距离表快速计算距离
distance, err := quantizer.AsymmetricDistanceWithTable(codes, distanceTable)
```

#### 性能分析
```go
info := quantizer.GetCodebookInfo()
compressionRatio := quantizer.GetCompressionRatio()
quantizationError, err := quantizer.EstimateQuantizationError(vector)
```

## 性能指标

基于1024维向量的测试结果：

- **压缩比**: 512:1 (8KB → 16字节)
- **内存减少**: 99.8%
- **编码时间**: ~190微秒/向量
- **解码时间**: ~850纳秒/向量
- **训练时间**: ~50秒（10,000个训练向量）
- **量化误差**: ~17.4（欧氏距离）

## 配置建议

### 维度选择
- 向量维度必须能被M整除
- 常见配置：1024维 → M=16 (64维子向量)

### 参数优化
- **M值**: 通常选择8、16或32，较大的M值提供更好的压缩但可能增加计算开销
- **K值**: 建议使用256以便每个码可用1个字节存储
- **训练数据**: 建议使用至少10,000个代表性向量进行训练

## 应用场景

1. **向量数据库**: 大规模向量存储和检索
2. **推荐系统**: 用户/物品嵌入向量的压缩存储
3. **图像检索**: 图像特征向量的高效索引
4. **自然语言处理**: 词向量、句向量的压缩
5. **与HNSW结合**: 构建高效的近似最近邻搜索索引

## 运行示例

```bash
cd common/ai/pq
go run cmd/pqcli.go
```

这将运行一个完整的演示，展示PQ算法的训练、编码、解码和性能测试。

## 技术细节

- 使用Go标准库实现，无外部依赖
- K-Means聚类算法支持自动收敛检测
- 内存效率优化，支持大规模数据处理
- 完整的错误处理和参数验证
- 使用yaklang项目的log包进行调试输出

## 注意事项

- 训练质量直接影响压缩效果和搜索精度
- 训练数据应该与实际使用的数据分布一致
- PQ算法是有损压缩，存在量化误差
- 适合与其他索引结构（如HNSW）结合使用以实现高效搜索
