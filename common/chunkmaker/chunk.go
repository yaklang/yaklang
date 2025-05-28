package chunkmaker

import (
	"bytes"
	"sync"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/utils/chanx"
)

type Chunk interface {
	IsUTF8() bool
	Data() []byte
	BytesSize() int64
	RunesSize() int64
}

type BufferChunk struct {
	mu *sync.RWMutex

	isUTF8   bool
	buffer   *bytes.Buffer
	bytesize int64
	runesize int64
}

var _ Chunk = (*BufferChunk)(nil)

func NewBufferChunk(buffer []byte) *BufferChunk {
	bc := &BufferChunk{
		mu:       new(sync.RWMutex),
		isUTF8:   utf8.Valid(buffer),
		buffer:   bytes.NewBuffer(buffer),
		bytesize: int64(len(buffer)),
	}
	if bc.isUTF8 {
		bc.runesize = int64(len([]rune(string(buffer))))
	} else {
		bc.runesize = bc.bytesize
	}
	return bc
}
func (c *BufferChunk) FlushFullChunkSizeTo(dst *chanx.UnlimitedChan[Chunk], chunkSize int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isUTF8 {
		// use runes size
		runes := []rune(c.buffer.String())
		processed := 0

		// 修复：使用 <= 确保处理所有完整的chunk
		for i := 0; i+int(chunkSize) <= len(runes); i += int(chunkSize) {
			end := i + int(chunkSize)
			chunk := NewBufferChunk([]byte(string(runes[i:end])))
			dst.SafeFeed(chunk)
			processed = end
		}

		// 保留剩余数据而不是清空
		if processed < len(runes) {
			remaining := string(runes[processed:])
			c.buffer.Reset()
			c.buffer.WriteString(remaining)
			c.runesize = int64(len([]rune(remaining)))
			c.bytesize = int64(len(remaining))
		} else {
			// 只有当所有数据都被处理时才完全重置
			c.buffer.Reset()
			c.runesize = 0
			c.bytesize = 0
		}
		c.isUTF8 = true
	} else {
		// use bytes size
		bytes := c.buffer.Bytes()
		processed := 0

		// 修复：使用 <= 确保处理所有完整的chunk
		for i := 0; i+int(chunkSize) <= len(bytes); i += int(chunkSize) {
			end := i + int(chunkSize)
			chunk := NewBufferChunk(bytes[i:end])
			dst.SafeFeed(chunk)
			processed = end
		}

		// 保留剩余数据而不是清空
		if processed < len(bytes) {
			remaining := bytes[processed:]
			c.buffer.Reset()
			c.buffer.Write(remaining)
			c.bytesize = int64(len(remaining))
			c.runesize = 0 // 非UTF8数据不计算rune size
		} else {
			// 只有当所有数据都被处理时才完全重置
			c.buffer.Reset()
			c.bytesize = 0
			c.runesize = 0
		}
		c.isUTF8 = false
	}
}

func (c *BufferChunk) FlushAllChunkSizeTo(dst *chanx.UnlimitedChan[Chunk], chunkSize int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isUTF8 {
		// use runes size
		runes := []rune(c.buffer.String())

		// 处理所有数据，包括最后不完整的chunk
		for i := 0; i < len(runes); i += int(chunkSize) {
			end := i + int(chunkSize)
			if end > len(runes) {
				end = len(runes)
			}
			chunk := NewBufferChunk([]byte(string(runes[i:end])))
			dst.SafeFeed(chunk)
		}

		// 完全重置buffer
		c.buffer.Reset()
		c.isUTF8 = true
		c.bytesize = 0
		c.runesize = 0
	} else {
		// use bytes size
		bytes := c.buffer.Bytes()

		// 处理所有数据，包括最后不完整的chunk
		for i := 0; i < len(bytes); i += int(chunkSize) {
			end := i + int(chunkSize)
			if end > len(bytes) {
				end = len(bytes)
			}
			chunk := NewBufferChunk(bytes[i:end])
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
