package codec

import (
	"io"
	"io/fs"
	"os"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/memfile"
)

// FileReader 定义了统一的文件读取接口，os.File 和 memfile.File 都实现了这个接口
type FileReader interface {
	io.Reader
	io.Seeker
	Stat() (fs.FileInfo, error)
}

func IsUTF8(i any) (bool, error) {
	switch ret := i.(type) {
	case FileReader:
		// 直接使用 FileReader 接口
		return isUTF8FromReader(ret)
	case io.Reader:
		// 将 io.Reader 读取到内存后创建 memfile
		bytes, err := io.ReadAll(ret)
		if err != nil {
			return false, err
		}
		mf := memfile.New(bytes)
		return isUTF8FromReader(mf)
	default:
		// 其他类型转换为字节后创建 memfile
		bytes := AnyToBytes(i)
		mf := memfile.New(bytes)
		return isUTF8FromReader(mf)
	}
}

// IsUTF8File checks if a file is UTF-8 encoded using sampling strategy
// For files < 0.5K: check entire content
// For files 0.5K-1K: check one 0.5K sample
// For files > 1K: check 4+ samples (256 runes each), up to 8 samples max
// If sampling cuts into UTF-8 character, look forward/backward to find valid boundaries
func IsUTF8File(filename string) (bool, error) {
	file, err := os.Open(filename)
	if err != nil {
		log.Errorf("failed to open file %s: %v", filename, err)
		return false, err
	}
	defer file.Close()

	return isUTF8FromReader(file)
}

// isUTF8FromReader 使用统一的 FileReader 接口检查 UTF-8 编码
func isUTF8FromReader(reader FileReader) (bool, error) {
	// Get file size
	stat, err := reader.Stat()
	if err != nil {
		log.Errorf("failed to get file stat: %v", err)
		return false, err
	}

	fileSize := stat.Size()
	log.Debugf("checking UTF-8 for file, size: %d bytes", fileSize)

	const halfK = 512
	const oneK = 1024

	if fileSize < halfK {
		// Small file: check entire content
		content, err := io.ReadAll(reader)
		if err != nil {
			log.Errorf("failed to read file: %v", err)
			return false, err
		}
		return isValidUTF8(content), nil
	} else if fileSize < oneK {
		// Medium file: check one 0.5K sample
		sample := make([]byte, halfK)
		n, err := reader.Read(sample)
		if err != nil && err != io.EOF {
			log.Errorf("failed to read sample from file: %v", err)
			return false, err
		}
		sample = sample[:n]

		// Fix UTF-8 boundaries
		sample = fixUTF8Boundaries(sample)
		return isValidUTF8(sample), nil
	} else {
		// Large file: sample strategy
		return checkLargeFileUTF8FromReader(reader, fileSize)
	}
}

// checkLargeFileUTF8FromReader handles sampling for large files (>1K) using FileReader interface
func checkLargeFileUTF8FromReader(reader FileReader, fileSize int64) (bool, error) {
	const oneK = 1024
	const sampleSize = 256 // 256 runes per sample
	const maxSamples = 8

	// Calculate number of samples: 4 base + 1 per additional 1K, max 8
	numSamples := 4 + int(fileSize/oneK) - 1
	if numSamples > maxSamples {
		numSamples = maxSamples
	}

	log.Debugf("using %d samples for file size %d", numSamples, fileSize)

	// Calculate sample positions evenly distributed across file
	samplePositions := make([]int64, numSamples)
	if numSamples == 1 {
		samplePositions[0] = 0
	} else {
		for i := 0; i < numSamples; i++ {
			samplePositions[i] = int64(i) * (fileSize - int64(sampleSize*4)) / int64(numSamples-1)
		}
	}

	// Check each sample
	for i, pos := range samplePositions {
		log.Debugf("checking sample %d at position %d", i+1, pos)

		sample, err := readSampleAtPositionFromReader(reader, pos, sampleSize*4) // Read more bytes to account for multi-byte UTF-8
		if err != nil {
			log.Errorf("failed to read sample at position %d: %v", pos, err)
			return false, err
		}

		// Fix UTF-8 boundaries
		sample = fixUTF8Boundaries(sample)

		// Get approximately sampleSize runes
		sample = limitToRunes(sample, sampleSize)

		if !isValidUTF8(sample) {
			log.Debugf("sample %d failed UTF-8 validation", i+1)
			return false, nil
		}
	}

	log.Debugf("all samples passed UTF-8 validation")
	return true, nil
}

// readSampleAtPositionFromReader reads a sample from the specified position using FileReader interface
func readSampleAtPositionFromReader(reader FileReader, pos int64, size int) ([]byte, error) {
	_, err := reader.Seek(pos, io.SeekStart)
	if err != nil {
		return nil, err
	}

	sample := make([]byte, size)
	n, err := reader.Read(sample)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return sample[:n], nil
}

// fixUTF8Boundaries fixes UTF-8 character boundaries by trimming incomplete characters
// at the start and end of the sample with enhanced error tolerance.
// It provides better safety when cutting into UTF-8 sequences.
func fixUTF8Boundaries(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	// 容错：如果数据太小，直接返回
	if len(data) < 4 {
		if utf8.Valid(data) {
			return data
		}
		// 尝试找到有效的UTF-8序列
		for i := len(data) - 1; i >= 0; i-- {
			if utf8.Valid(data[:i]) {
				return data[:i]
			}
		}
		return []byte{} // 如果没有有效序列，返回空
	}

	start := findSafeStartPosition(data)
	end := findSafeEndPosition(data, start)

	result := data[start:end]
	if start > 0 || end < len(data) {
		log.Debugf("fixed UTF-8 boundaries: %d bytes -> %d bytes (trimmed %d from start, %d from end)",
			len(data), len(result), start, len(data)-end)
	}
	return result
}

// findSafeStartPosition 找到安全的开始位置，跳过不完整的UTF-8序列
func findSafeStartPosition(data []byte) int {
	maxScan := 4 // UTF-8字符最多4字节
	if maxScan > len(data) {
		maxScan = len(data)
	}

	// 从开头扫描，找到第一个有效的rune起始位置
	for i := 0; i < maxScan; i++ {
		if utf8.RuneStart(data[i]) {
			// 验证从这个位置开始是否能解码出有效的rune
			if _, size := utf8.DecodeRune(data[i:]); size > 0 {
				return i
			}
		}
	}

	// 如果前面都找不到，继续向后找
	for i := maxScan; i < len(data); i++ {
		if utf8.RuneStart(data[i]) {
			return i
		}
	}

	return len(data) // 如果找不到任何有效起始位置，返回数据长度
}

// findSafeEndPosition 找到安全的结束位置，避免截断UTF-8字符
func findSafeEndPosition(data []byte, start int) int {
	if start >= len(data) {
		return start
	}

	// 从末尾向前扫描最多4个字节，寻找安全的截断点
	maxScan := 4
	end := len(data)

	for i := 1; i <= maxScan && end-i >= start; i++ {
		pos := end - i
		if utf8.RuneStart(data[pos]) {
			// 检查从这个位置到末尾是否是一个完整的UTF-8字符
			remainingBytes := end - pos
			r, size := utf8.DecodeRune(data[pos:end])

			if r != utf8.RuneError && size == remainingBytes {
				// 这是一个完整的字符，可以保留
				return end
			} else {
				// 这是一个不完整的字符，应该截断
				end = pos
			}
			break
		}
	}

	// 验证结果的有效性
	if start < end && !utf8.Valid(data[start:end]) {
		// 如果结果仍然无效，尝试进一步向前截断
		for end > start {
			end--
			if utf8.Valid(data[start:end]) {
				break
			}
		}
	}

	return end
}

// limitToRunes limits the byte slice to approximately the specified number of runes
func limitToRunes(data []byte, maxRunes int) []byte {
	if len(data) == 0 {
		return data
	}

	runeCount := 0
	pos := 0

	for pos < len(data) && runeCount < maxRunes {
		_, size := utf8.DecodeRune(data[pos:])
		if size == 0 {
			break
		}
		pos += size
		runeCount++
	}

	return data[:pos]
}

// isValidUTF8 checks if the byte slice is valid UTF-8
func isValidUTF8(data []byte) bool {
	if len(data) == 0 {
		return true // Empty data is considered valid UTF-8
	}

	valid := utf8.Valid(data)
	log.Debugf("UTF-8 validation result: %v for %d bytes", valid, len(data))
	return valid
}
