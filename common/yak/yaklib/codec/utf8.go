package codec

import (
	"io"
	"os"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/log"
)

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

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		log.Errorf("failed to get file stat for %s: %v", filename, err)
		return false, err
	}

	fileSize := stat.Size()
	log.Debugf("checking UTF-8 for file %s, size: %d bytes", filename, fileSize)

	const halfK = 512
	const oneK = 1024

	if fileSize < halfK {
		// Small file: check entire content
		content, err := io.ReadAll(file)
		if err != nil {
			log.Errorf("failed to read file %s: %v", filename, err)
			return false, err
		}
		return isValidUTF8(content), nil
	} else if fileSize < oneK {
		// Medium file: check one 0.5K sample
		sample := make([]byte, halfK)
		n, err := file.Read(sample)
		if err != nil && err != io.EOF {
			log.Errorf("failed to read sample from file %s: %v", filename, err)
			return false, err
		}
		sample = sample[:n]

		// Fix UTF-8 boundaries
		sample = fixUTF8Boundaries(sample)
		return isValidUTF8(sample), nil
	} else {
		// Large file: sample strategy
		return checkLargeFileUTF8(file, fileSize)
	}
}

// checkLargeFileUTF8 handles sampling for large files (>1K)
func checkLargeFileUTF8(file *os.File, fileSize int64) (bool, error) {
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

		sample, err := readSampleAtPosition(file, pos, sampleSize*4) // Read more bytes to account for multi-byte UTF-8
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

// readSampleAtPosition reads a sample from the specified position
func readSampleAtPosition(file *os.File, pos int64, size int) ([]byte, error) {
	_, err := file.Seek(pos, io.SeekStart)
	if err != nil {
		return nil, err
	}

	sample := make([]byte, size)
	n, err := file.Read(sample)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return sample[:n], nil
}

// fixUTF8Boundaries fixes UTF-8 character boundaries by trimming incomplete characters
// at the start and end of the sample, while preserving invalid UTF-8 content for detection
func fixUTF8Boundaries(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	// Find the start position - skip incomplete UTF-8 sequences at the beginning
	start := 0
	for start < len(data) && !utf8.RuneStart(data[start]) {
		start++
	}

	// Find the end position - work backwards to find the last valid UTF-8 boundary
	end := len(data)
	for end > start {
		// Try validating from start to this end position
		if utf8.Valid(data[start:end]) {
			break
		}
		// Move back one byte and try again
		end--
		// Make sure we don't end in the middle of a UTF-8 character
		for end > start && !utf8.RuneStart(data[end]) {
			end--
		}
	}

	// If we couldn't find a valid range, return original data to preserve invalid content
	if end <= start {
		log.Debugf("could not establish valid UTF-8 boundaries, preserving original for detection")
		return data
	}

	result := data[start:end]
	if start > 0 || end < len(data) {
		log.Debugf("fixed UTF-8 boundaries: %d bytes -> %d bytes (trimmed %d from start, %d from end)",
			len(data), len(result), start, len(data)-end)
	}
	return result
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
