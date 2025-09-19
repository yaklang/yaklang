package tools

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"strings"
)

// GetImgSimilarity 计算两个图片的相似度
// 参数可以是文件路径或base64编码的图片数据
// 返回0.0-1.0之间的相似度值，1.0表示完全相同
func GetImgSimilarity(imgA, imgB string) float64 {
	// 解析图片
	imageA, err := parseImage(imgA)
	if err != nil {
		return 0.0
	}

	imageB, err := parseImage(imgB)
	if err != nil {
		return 0.0
	}

	// 如果图片完全相同，直接返回1.0
	if imagesEqual(imageA, imageB) {
		return 1.0
	}

	// 计算直方图相似度 (权重: 0.4)
	histogramSimilarity := calculateHistogramSimilarity(imageA, imageB)

	// 计算结构相似度 (权重: 0.4)
	structuralSimilarity := calculateStructuralSimilarity(imageA, imageB)

	// 计算特征相似度 (权重: 0.2)
	featureSimilarity := calculateImageFeatureSimilarity(imageA, imageB)

	fmt.Println(histogramSimilarity, structuralSimilarity, featureSimilarity)

	// 加权平均
	totalSimilarity := histogramSimilarity*0.4 + structuralSimilarity*0.4 + featureSimilarity*0.2

	return totalSimilarity
}

// parseImage 解析图片，支持文件路径和base64编码
func parseImage(imgSource string) (image.Image, error) {
	var img image.Image
	var err error

	// 检查是否为base64编码
	if strings.HasPrefix(imgSource, "data:image") || strings.HasPrefix(imgSource, "iVBOR") {
		// 处理base64编码的图片
		img, err = parseBase64Image(imgSource)
	} else {
		// 处理文件路径
		img, err = parseFileImage(imgSource)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse image: %v", err)
	}

	return img, nil
}

// parseBase64Image 解析base64编码的图片
func parseBase64Image(base64Data string) (image.Image, error) {
	// 移除data:image/xxx;base64,前缀
	if strings.Contains(base64Data, ";base64,") {
		parts := strings.Split(base64Data, ";base64,")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid base64 format")
		}
		base64Data = parts[1]
	}

	// 解码base64
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %v", err)
	}

	// 解析图片
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}

	return img, nil
}

// parseFileImage 解析文件图片
func parseFileImage(filePath string) (image.Image, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}

	return img, nil
}

// imagesEqual 检查两个图片是否完全相同
func imagesEqual(imgA, imgB image.Image) bool {
	boundsA := imgA.Bounds()
	boundsB := imgB.Bounds()

	if !boundsA.Eq(boundsB) {
		return false
	}

	for y := boundsA.Min.Y; y < boundsA.Max.Y; y++ {
		for x := boundsA.Min.X; x < boundsA.Max.X; x++ {
			if imgA.At(x, y) != imgB.At(x, y) {
				return false
			}
		}
	}

	return true
}

// calculateHistogramSimilarity 计算直方图相似度
func calculateHistogramSimilarity(imgA, imgB image.Image) float64 {
	// 计算RGB直方图
	histA := calculateRGBHistogram(imgA)
	histB := calculateRGBHistogram(imgB)

	// 使用余弦相似度计算直方图相似度
	return imageCosineSimilarity(histA, histB)
}

// calculateRGBHistogram 计算RGB直方图
func calculateRGBHistogram(img image.Image) []float64 {
	bounds := img.Bounds()
	histogram := make([]float64, 768) // 256 * 3 (R, G, B)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// 将16位值转换为8位
			r8 := r >> 8
			g8 := g >> 8
			b8 := b >> 8

			histogram[r8]++
			histogram[g8+256]++
			histogram[b8+512]++
		}
	}

	// 归一化
	totalPixels := float64(bounds.Dx() * bounds.Dy())
	for i := range histogram {
		histogram[i] /= totalPixels
	}

	return histogram
}

// calculateStructuralSimilarity 计算结构相似度
func calculateStructuralSimilarity(imgA, imgB image.Image) float64 {
	// 将图片转换为灰度图
	grayA := convertToGrayscale(imgA)
	grayB := convertToGrayscale(imgB)

	// 计算结构相似性指数 (SSIM)
	return calculateSSIM(grayA, grayB)
}

// convertToGrayscale 将图片转换为灰度图
func convertToGrayscale(img image.Image) *image.Gray {
	bounds := img.Bounds()
	gray := image.NewGray(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray.Set(x, y, img.At(x, y))
		}
	}

	return gray
}

// resizeGrayImage 调整灰度图片尺寸
func resizeGrayImage(img *image.Gray, targetWidth, targetHeight int) *image.Gray {
	bounds := img.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	// 创建目标图片
	resized := image.NewGray(image.Rect(0, 0, targetWidth, targetHeight))

	// 计算缩放比例
	xRatio := float64(srcWidth) / float64(targetWidth)
	yRatio := float64(srcHeight) / float64(targetHeight)

	// 使用最近邻插值进行缩放
	for y := 0; y < targetHeight; y++ {
		for x := 0; x < targetWidth; x++ {
			// 计算源图片中的对应位置
			srcX := int(float64(x) * xRatio)
			srcY := int(float64(y) * yRatio)

			// 确保不越界
			if srcX >= srcWidth {
				srcX = srcWidth - 1
			}
			if srcY >= srcHeight {
				srcY = srcHeight - 1
			}

			// 复制像素值
			resized.SetGray(x, y, img.GrayAt(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}

	return resized
}

// calculateSSIM 计算结构相似性指数
func calculateSSIM(imgA, imgB *image.Gray) float64 {
	boundsA := imgA.Bounds()
	boundsB := imgB.Bounds()

	// 如果尺寸不同，将两个图片调整到相同尺寸
	if !boundsA.Eq(boundsB) {
		// 选择较小的尺寸作为目标尺寸，保持宽高比
		targetWidth := boundsA.Dx()
		targetHeight := boundsA.Dy()
		if boundsB.Dx() < targetWidth {
			targetWidth = boundsB.Dx()
		}
		if boundsB.Dy() < targetHeight {
			targetHeight = boundsB.Dy()
		}

		// 调整图片尺寸
		imgA = resizeGrayImage(imgA, targetWidth, targetHeight)
		imgB = resizeGrayImage(imgB, targetWidth, targetHeight)
	}

	bounds := imgA.Bounds()

	// SSIM参数
	const (
		k1 = 0.01
		k2 = 0.03
		L  = 255.0
	)

	c1 := (k1 * L) * (k1 * L)
	c2 := (k2 * L) * (k2 * L)

	var muA, muB, sigmaA, sigmaB, sigmaAB float64
	totalPixels := float64(bounds.Dx() * bounds.Dy())

	// 计算均值
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			valA := float64(imgA.GrayAt(x, y).Y)
			valB := float64(imgB.GrayAt(x, y).Y)

			muA += valA
			muB += valB
		}
	}
	muA /= totalPixels
	muB /= totalPixels

	// 计算方差和协方差
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			valA := float64(imgA.GrayAt(x, y).Y)
			valB := float64(imgB.GrayAt(x, y).Y)

			diffA := valA - muA
			diffB := valB - muB

			sigmaA += diffA * diffA
			sigmaB += diffB * diffB
			sigmaAB += diffA * diffB
		}
	}
	sigmaA /= totalPixels
	sigmaB /= totalPixels
	sigmaAB /= totalPixels

	// 计算SSIM
	numerator := (2*muA*muB + c1) * (2*sigmaAB + c2)
	denominator := (muA*muA + muB*muB + c1) * (sigmaA + sigmaB + c2)

	if denominator == 0 {
		return 1.0
	}

	return numerator / denominator
}

// calculateImageFeatureSimilarity 计算图片特征相似度
func calculateImageFeatureSimilarity(imgA, imgB image.Image) float64 {
	// 计算图片的统计特征
	featuresA := extractImageFeatures(imgA)
	featuresB := extractImageFeatures(imgB)

	// 使用余弦相似度计算特征相似度
	return imageCosineSimilarity(featuresA, featuresB)
}

// extractImageFeatures 提取图片特征
func extractImageFeatures(img image.Image) []float64 {
	bounds := img.Bounds()
	features := make([]float64, 0)

	// 图片尺寸特征
	features = append(features, float64(bounds.Dx()))
	features = append(features, float64(bounds.Dy()))
	features = append(features, float64(bounds.Dx()*bounds.Dy())) // 面积

	// 颜色统计特征
	var totalR, totalG, totalB, totalBrightness float64
	var minBrightness, maxBrightness float64
	minBrightness = math.MaxFloat64

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			r8 := float64(r >> 8)
			g8 := float64(g >> 8)
			b8 := float64(b >> 8)

			totalR += r8
			totalG += g8
			totalB += b8

			// 计算亮度
			brightness := 0.299*r8 + 0.587*g8 + 0.114*b8
			totalBrightness += brightness

			if brightness < minBrightness {
				minBrightness = brightness
			}
			if brightness > maxBrightness {
				maxBrightness = brightness
			}
		}
	}

	totalPixels := float64(bounds.Dx() * bounds.Dy())

	// 平均颜色值
	features = append(features, totalR/totalPixels)
	features = append(features, totalG/totalPixels)
	features = append(features, totalB/totalPixels)

	// 亮度特征
	features = append(features, totalBrightness/totalPixels)
	features = append(features, minBrightness)
	features = append(features, maxBrightness)
	features = append(features, maxBrightness-minBrightness) // 对比度

	return features
}

// imageCosineSimilarity 计算图片余弦相似度
func imageCosineSimilarity(vecA, vecB []float64) float64 {
	if len(vecA) != len(vecB) || len(vecA) == 0 {
		return 0.0
	}

	var dotProduct, normA, normB float64

	for i := 0; i < len(vecA); i++ {
		dotProduct += vecA[i] * vecB[i]
		normA += vecA[i] * vecA[i]
		normB += vecB[i] * vecB[i]
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
