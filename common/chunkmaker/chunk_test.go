package chunkmaker

import (
	"bytes"
	"testing"
)

func TestBufferChunk_Linking(t *testing.T) {
	c1Data := []byte("chunk1")
	c2Data := []byte("chunk2")
	c3Data := []byte("chunk3")

	bc1 := NewBufferChunk(c1Data)
	bc2 := NewBufferChunk(c2Data)
	bc3 := NewBufferChunk(c3Data)

	// Manually link for testing direct prev assignment
	bc2.prev = bc1 // bc2 -> bc1
	bc3.prev = bc2 // bc3 -> bc2 -> bc1

	t.Run("HaveLastChunk_DirectLink", func(t *testing.T) {
		if !bc3.HaveLastChunk() {
			t.Errorf("bc3.HaveLastChunk() = false, want true")
		}
		if !bc2.HaveLastChunk() {
			t.Errorf("bc2.HaveLastChunk() = false, want true")
		}
		if bc1.HaveLastChunk() {
			t.Errorf("bc1.HaveLastChunk() = true, want false")
		}
	})

	t.Run("LastChunk_DirectLink", func(t *testing.T) {
		if got := bc3.LastChunk(); got != bc2 {
			t.Errorf("bc3.LastChunk() = %p, want %p", got, bc2)
		}
		if got := bc2.LastChunk(); got != bc1 {
			t.Errorf("bc2.LastChunk() = %p, want %p", got, bc1)
		}
		if got := bc1.LastChunk(); got != nil {
			t.Errorf("bc1.LastChunk() = %p, want nil", got)
		}
	})

	t.Run("DataIntegrity_AfterDirectLink", func(t *testing.T) {
		if !bytes.Equal(bc3.Data(), c3Data) {
			t.Errorf("bc3.Data() = %s, want %s", string(bc3.Data()), string(c3Data))
		}
		if !bytes.Equal(bc2.Data(), c2Data) {
			t.Errorf("bc2.Data() = %s, want %s", string(bc2.Data()), string(c2Data))
		}
		if !bytes.Equal(bc1.Data(), c1Data) {
			t.Errorf("bc1.Data() = %s, want %s", string(bc1.Data()), string(c1Data))
		}
	})
}

func TestNewBufferChunk_InitialState(t *testing.T) {
	data := []byte("initial")
	bc := NewBufferChunk(data)

	if bc.HaveLastChunk() {
		t.Errorf("NewBufferChunk should not have a last chunk initially, HaveLastChunk() got true")
	}
	if bc.LastChunk() != nil {
		t.Errorf("NewBufferChunk should return nil for LastChunk() initially, got %v", bc.LastChunk())
	}
	if !bytes.Equal(bc.Data(), data) {
		t.Errorf("NewBufferChunk data = %s, want %s", string(bc.Data()), string(data))
	}
}

// TestBufferChunk_HaveLastChunk and TestBufferChunk_LastChunk (original tests)
// can be kept if they test unlinked chunks or specific states of single chunks.
// For this iteration, focusing on linked behavior.

func TestBufferChunk_HaveLastChunk_Single(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		// prev is not set here as NewBufferChunk creates unlinked chunks
		want bool
	}{
		{
			name: "empty chunk, no prev",
			data: []byte{},
			want: false, // An empty chunk, if unlinked, has no prev
		},
		{
			name: "non-empty chunk, no prev",
			data: []byte("hello"),
			want: false, // A non-empty chunk, if unlinked, has no prev
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewBufferChunk(tt.data)
			if got := c.HaveLastChunk(); got != tt.want {
				t.Errorf("BufferChunk.HaveLastChunk() for single chunk = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBufferChunk_LastChunk_Single(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		// prev is not set here as NewBufferChunk creates unlinked chunks
		want Chunk // Expect nil as new chunks are unlinked
	}{
		{
			name: "empty chunk, no prev",
			data: []byte{},
			want: nil,
		},
		{
			name: "non-empty chunk, no prev",
			data: []byte("hello"),
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewBufferChunk(tt.data)
			got := c.LastChunk()
			if got != tt.want { // Direct comparison for nil
				t.Errorf("BufferChunk.LastChunk() for single chunk = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBufferChunk_PrevNBytes(t *testing.T) {
	c1Data := []byte("12345")      // len 5
	c2Data := []byte("abcdefghij") // len 10
	c3Data := []byte("XYZ")        // len 3. This is the data of the current chunk for PrevNBytes calls on bc3.

	bc1 := NewBufferChunk(c1Data)
	bc2 := NewBufferChunk(c2Data)
	bc3 := NewBufferChunk(c3Data) // bc3 is the "current" chunk for the first set of tests

	// Link them: bc3 -> bc2 -> bc1 (bc3.prev is bc2, bc2.prev is bc1)
	bc2.prev = bc1
	bc3.prev = bc2

	tests := []struct {
		name       string
		startChunk Chunk // The chunk on which PrevNBytes is called
		n          int
		want       []byte
	}{
		{
			name:       "bc3: n=0, prev is bc2",
			startChunk: bc3,
			n:          0,
			want:       []byte{},
		},
		{
			name:       "bc3: n=negative, prev is bc2",
			startChunk: bc3,
			n:          -5,
			want:       []byte{},
		},
		{
			name:       "bc3: n smaller than prev chunk (bc2) data",
			startChunk: bc3, // prev is bc2 ("abcdefghij")
			n:          4,
			want:       []byte("ghij"), // Last 4 bytes of bc2's data
		},
		{
			name:       "bc3: n equals prev chunk (bc2) data",
			startChunk: bc3, // prev is bc2
			n:          10,
			want:       c2Data, // All of bc2's data
		},
		{
			name:       "bc3: n larger than bc2, takes all bc2 and part of bc1",
			startChunk: bc3,                             // prev is bc2, bc2.prev is bc1 ("12345")
			n:          12,                              // 10 from bc2, needs 2 from bc1
			want:       append([]byte("45"), c2Data...), // "45" (from bc1) + "abcdefghij" (all bc2)
		},
		{
			name:       "bc3: n equals total size of all prev chunks (bc2+bc1)",
			startChunk: bc3, // prev chain: bc2 (10 bytes) -> bc1 (5 bytes) = 15 bytes
			n:          15,
			want:       bytes.Join([][]byte{c1Data, c2Data}, []byte{}), // bc1 data + bc2 data
		},
		{
			name:       "bc3: n larger than total size of all prev chunks",
			startChunk: bc3,
			n:          20,
			want:       bytes.Join([][]byte{c1Data, c2Data}, []byte{}), // same as above
		},
		{
			name:       "bc2: n smaller than prev chunk (bc1) data",
			startChunk: bc2, // prev is bc1 ("12345")
			n:          3,
			want:       []byte("345"), // Last 3 of bc1's data
		},
		{
			name:       "bc2: n equals prev chunk (bc1) data",
			startChunk: bc2, // prev is bc1
			n:          5,
			want:       c1Data, // All of bc1's data
		},
		{
			name:       "bc2: n larger than prev chunk (bc1) data",
			startChunk: bc2, // prev is bc1
			n:          8,
			want:       c1Data, // Still just bc1's data as it's the only prev
		},
		{
			name:       "bc1: no prev chunk",
			startChunk: bc1, // bc1.prev is nil
			n:          5,
			want:       []byte{},
		},
		{
			name:       "single unlinked chunk (newly created)",
			startChunk: NewBufferChunk([]byte("single")), // No .prev
			n:          4,
			want:       []byte{},
		},
		{
			name: "start chunk is empty, its prev has data",
			startChunk: func() Chunk {
				emptyCurrent := NewBufferChunk([]byte{})
				prevWithData := NewBufferChunk([]byte("prevData")) // len 8
				emptyCurrent.prev = prevWithData
				return emptyCurrent
			}(),
			n:    5,
			want: []byte("vData"), // Last 5 from "prevData"
		},
		{
			name: "start chunk has data, its prev is empty, but grandPrev has data",
			startChunk: func() Chunk {
				currWithData := NewBufferChunk([]byte("current")) // This chunk's data is irrelevant for PrevNBytes
				emptyPrev := NewBufferChunk([]byte{})
				grandPrevWithData := NewBufferChunk([]byte("grandData")) // len 9
				emptyPrev.prev = grandPrevWithData
				currWithData.prev = emptyPrev
				return currWithData
			}(),
			n:    6,                // Should get from grandPrevWithData
			want: []byte("ndData"), // Last 6 from "grandData"
		},
		{
			name: "prev chain with empty chunks interspersed",
			startChunk: func() Chunk {
				c := NewBufferChunk([]byte("current")) // Data of c is irrelevant
				p1 := NewBufferChunk([]byte("P1"))     // 2 bytes
				pEmpty1 := NewBufferChunk([]byte(""))  // 0 bytes
				p2 := NewBufferChunk([]byte("P2P2"))   // 4 bytes
				pEmpty2 := NewBufferChunk([]byte(""))  // 0 bytes
				p3 := NewBufferChunk([]byte("P3P3P3")) // 6 bytes

				c.prev = p1
				p1.prev = pEmpty1
				pEmpty1.prev = p2
				p2.prev = pEmpty2
				pEmpty2.prev = p3
				return c
			}(),
			n: 7, // Want: P3P3P3 (6) + P2 (first 1 of P2P2) -> No, last 1 of P2P2. want: "P2P2P3P3P3" (last 4 from P2, all P3), want "P2P2P3P3P3", then take last 7 => "2P3P3P3"
			// P1(2) <- pEmpty1(0) <- P2(4) <- pEmpty2(0) <- P3(6)
			// Total prev data: P1(2) + P2(4) + P3(6) = 12 bytes
			// Data order for PrevNBytes internal list before join (oldest first): p3_segment, p2_segment, p1_segment
			// If n=7: p3 contributes last 1 byte ("3"), p2 contributes all ("P2P2"), p1 contributes all ("P1")
			// Resulting list for join: {"3" , "P2P2", "P1"}
			want: []byte("3P2P2P1"), // Corrected: p3_tail + p2_full + p1_full
		},
		{
			name: "N exactly matches total size of specific prev chunks with empty ones",
			startChunk: func() Chunk {
				c := NewBufferChunk([]byte("current"))
				p1 := NewBufferChunk([]byte("Alpha")) // 5
				pE := NewBufferChunk([]byte(""))      // 0
				p2 := NewBufferChunk([]byte("Beta"))  // 4
				c.prev = p1
				p1.prev = pE
				pE.prev = p2 // p2 is the earliest
				return c
			}(),
			n:    9, // All of p2 (4 bytes, "Beta") + all of p1 (5 bytes, "Alpha")
			want: []byte("BetaAlpha"),
		},
		{
			name: "Prev chain contains only empty chunks",
			startChunk: func() Chunk {
				c := NewBufferChunk([]byte("current"))
				pE1 := NewBufferChunk([]byte(""))
				pE2 := NewBufferChunk([]byte(""))
				c.prev = pE1
				pE1.prev = pE2
				return c
			}(),
			n:    5,
			want: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// It's important to use tt.startChunk.PrevNBytes here
			got := tt.startChunk.PrevNBytes(tt.n)
			if !bytes.Equal(got, tt.want) {
				// For PrevNBytes, string(tt.startChunk.Data()) might be misleading if tt.startChunk has no prev
				// or if we are focused on what its prev contained.
				var prevDataStr string
				if tt.startChunk.HaveLastChunk() {
					prevDataStr = string(tt.startChunk.LastChunk().Data())
				} else {
					prevDataStr = "<nil>"
				}
				t.Errorf("Chunk (data: '%s', prev data: '%s').PrevNBytes(%d)\n got: %q (%s)\nwant: %q (%s)",
					string(tt.startChunk.Data()), prevDataStr, tt.n, got, string(got), tt.want, string(tt.want))
			}
		})
	}
}
