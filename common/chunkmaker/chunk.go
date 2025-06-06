package chunkmaker

import (
	"bytes"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/utils/chanx"
)

type Chunk interface {
	IsUTF8() bool
	Data() []byte
	BytesSize() int64
	RunesSize() int64
	HaveLastChunk() bool
	LastChunk() Chunk
	PrevNBytes(n int) []byte
}

type BufferChunk struct {
	mu *sync.RWMutex

	isUTF8   bool
	buffer   *bytes.Buffer
	bytesize int64
	runesize int64
	prev     Chunk // 指向前一个 Chunk
}

var _ Chunk = (*BufferChunk)(nil)

func NewBufferChunk(buffer []byte) *BufferChunk {
	bc := &BufferChunk{
		mu:       new(sync.RWMutex),
		isUTF8:   utf8.Valid(buffer),
		buffer:   bytes.NewBuffer(buffer),
		bytesize: int64(len(buffer)),
		prev:     nil, // 新创建的 chunk 默认没有前一个 chunk
	}
	if bc.isUTF8 {
		bc.runesize = int64(len([]rune(string(buffer))))
	} else {
		bc.runesize = bc.bytesize
	}
	return bc
}

func BytesHandler(data []byte, chunkSize int64, sep []byte, emitFunc func([]byte)) []byte {
	sepLength := len(sep)
	for len(data) > 0 {
		step := int(chunkSize)
		if sepIndex := bytes.Index(data, sep); sepIndex > 0 && int64(sepIndex+sepLength) < chunkSize {
			step = sepIndex + sepLength
		}
		if step > len(data) {
			break
		}
		if emitFunc != nil {
			emitFunc(data[:step])
		}
		data = data[step:]
	}
	return data
}

func RuneHandler(data []rune, chunkSize int64, sep []rune, emitFunc func([]rune)) []rune {
	sepLength := len(sep)
	for len(data) > 0 {
		step := int(chunkSize)
		if sepIndex := utils.RuneIndex(data, sep); sepIndex > 0 && int64(sepIndex+sepLength) < chunkSize {
			step = sepIndex + sepLength
		}
		if step > len(data) {
			break
		}
		if emitFunc != nil {
			emitFunc(data[:step])
		}
		data = data[step:]
	}
	return data
}

func (c *BufferChunk) FlushFullChunkSizeTo(dst *chanx.UnlimitedChan[Chunk], chunkSize int64, sep string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isUTF8 {
		remainingData := RuneHandler([]rune(c.buffer.String()), chunkSize, []rune(sep), func(runes []rune) {
			chunk := NewBufferChunk([]byte(string(runes)))
			dst.SafeFeed(chunk)
		})
		c.buffer.Reset()
		if len(remainingData) > 0 {
			c.buffer.WriteString(string(remainingData))
			c.runesize = int64(len(remainingData))
			c.bytesize = int64(len([]byte(string(remainingData))))
		} else {
			c.runesize = 0
			c.bytesize = 0
		}
		c.isUTF8 = true
	} else {
		remainingData := BytesHandler([]byte(c.buffer.String()), chunkSize, []byte(sep), func(dataBytes []byte) {
			chunk := NewBufferChunk(dataBytes)
			dst.SafeFeed(chunk)
		})
		c.buffer.Reset()
		if len(remainingData) > 0 {
			c.buffer.Write(remainingData)
			c.runesize = int64(len([]rune(string(remainingData))))
			c.bytesize = int64(len(remainingData))
		} else {
			c.runesize = 0
			c.bytesize = 0
		}
		c.isUTF8 = false
	}
}

func (c *BufferChunk) FlushAllChunkSizeTo(dst *chanx.UnlimitedChan[Chunk], chunkSize int64, sep string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isUTF8 {
		// use runes size
		remainingData := RuneHandler([]rune(c.buffer.String()), chunkSize, []rune(sep), func(runes []rune) {
			chunk := NewBufferChunk([]byte(string(runes)))
			dst.SafeFeed(chunk)
		})
		if len(remainingData) > 0 {
			chunk := NewBufferChunk([]byte(string(remainingData)))
			dst.SafeFeed(chunk)
		}
		// 完全重置buffer
		c.buffer.Reset()
		c.isUTF8 = true
		c.bytesize = 0
		c.runesize = 0
	} else {
		// use bytes size
		remainingData := BytesHandler(c.buffer.Bytes(), chunkSize, []byte(sep), func(dataBytes []byte) {
			chunk := NewBufferChunk(dataBytes)
			dst.SafeFeed(chunk)
		})
		if len(remainingData) > 0 {
			chunk := NewBufferChunk(remainingData)
			dst.SafeFeed(chunk)
		}

		// 完全重置buffer
		c.buffer.Reset()
		c.isUTF8 = false
		c.bytesize = 0
		c.runesize = 0
	}
}
func (c *BufferChunk) Write(i []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	utf8valid := utf8.Valid(i)
	if utf8valid {
		if c.isUTF8 {
			c.buffer.Write(i)
		} else {
			c.buffer.Write(i)
			c.isUTF8 = utf8.Valid(c.buffer.Bytes())
		}
	} else {
		if c.isUTF8 {
			c.buffer.Write(i)
			c.isUTF8 = false
		} else {
			c.buffer.Write(i)
			c.isUTF8 = utf8.Valid(c.buffer.Bytes())
		}
	}

	c.bytesize += int64(len(i))
	if c.isUTF8 {
		c.runesize += int64(len([]rune(string(i))))
	} else {
		c.runesize += int64(len([]rune(c.buffer.String())))
	}
}

func (c *BufferChunk) IsUTF8() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.isUTF8
}

func (c *BufferChunk) Data() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.buffer.Bytes()
}

func (c *BufferChunk) BytesSize() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.bytesize
}

func (c *BufferChunk) RunesSize() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.runesize
}

func (c *BufferChunk) HaveLastChunk() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.prev != nil
}

func (c *BufferChunk) LastChunk() Chunk {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.prev
}

// PrevNBytes collects N bytes by traversing the prev chain, excluding the current chunk's data.
func (c *BufferChunk) PrevNBytes(n int) []byte {
	if n <= 0 {
		return []byte{}
	}

	var result [][]byte
	var totalBytesCollected int
	// Start collecting from the previous chunk
	currentChunk := c.LastChunk() // This is c.prev

	for currentChunk != nil && totalBytesCollected < n {
		data := currentChunk.Data()
		bytesToTake := len(data)
		if totalBytesCollected+bytesToTake > n {
			bytesToTake = n - totalBytesCollected
		}

		if bytesToTake > 0 {
			// Prepend to maintain order (last bytes come from earlier chunks in the list)
			// If taking a partial chunk, take from its end.
			start := len(data) - bytesToTake
			result = append([][]byte{data[start:]}, result...) // Prepend slice of bytes
			totalBytesCollected += bytesToTake
		}

		if totalBytesCollected >= n {
			break
		}
		currentChunk = currentChunk.LastChunk() // Move to the next previous chunk
	}

	// Concatenate all collected byte slices
	finalBuffer := bytes.NewBuffer(make([]byte, 0, totalBytesCollected))
	for _, b := range result {
		finalBuffer.Write(b)
	}

	return finalBuffer.Bytes()
}
