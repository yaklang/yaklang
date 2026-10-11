package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Owned grammar control: fixed GPA/GSF 22a9aa1 BinaryImageFrames uses a
// 65535-byte first frame, strips later 16-byte headers and validates one CRC
// over the entire original image (including first CONT_IDX=1).
func c37ContinuationControl(channels int) ([][]byte, []byte) {
	body := []byte{0, 1, 0, 15, 66, 64, 0, 1, 3, 'P', 'M', 'U', 0, 7}
	body = append(body, make([]byte, 16)...)
	body = append(body, 0, 0, byte(channels>>8), byte(channels), 0, 0, 0, 0)
	for i := 0; i < channels; i++ {
		body = append(body, 255)
		body = append(body, bytes.Repeat([]byte{'A'}, 255)...)
	}
	for i := 0; i < channels; i++ {
		body = append(body, 0, 0, 0, 0, 0x3f, 0x80, 0, 0, 0, 0, 0, 0)
	}
	body = append(body, make([]byte, 12)...)
	body = append(body, 'M', 0, 0, 0, 10, 0, 0, 0, 0, 0, 0, 0, 1, 0, 50)
	image := append([]byte{170, 0x52, 255, 255, 0, 7, 0, 0, 0, 1, 0, 0, 0, 0}, body...)
	crc := c37118CRC(image)
	image = append(image, byte(crc>>8), byte(crc))
	frames := [][]byte{bytes.Clone(image[:65535])}
	for at, index := 65535, uint16(2); at < len(image); index++ {
		end := min(at+65519, len(image))
		w := bytes.Clone(image[:16])
		binary.BigEndian.PutUint16(w[2:], uint16(16+end-at))
		idx := index
		if end == len(image) {
			idx = 65535
		}
		binary.BigEndian.PutUint16(w[14:], idx)
		w = append(w, image[at:end]...)
		frames = append(frames, w)
		at = end
	}
	data := make([]byte, 2+channels*4+4)
	binary.BigEndian.PutUint16(data[len(data)-4:], 5)
	binary.BigEndian.PutUint16(data[len(data)-2:], 0xfffe)
	return frames, c37frame(0, 2, 7, data)
}

func TestC37118ContinuationExistingAPI(t *testing.T) {
	frames, data := c37ContinuationControl(256)
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(39300, 28400))
	require.NoError(t, err)
	defer s.Close("test")
	for i, wire := range frames {
		owned := bytes.Clone(wire)
		r := s.Feed(0, time.Unix(int64(i+1), 0), owned)
		clear(owned)
		require.Len(t, r.Events, 1)
		if i+1 < len(frames) {
			require.NotNil(t, r.Err)
			require.Equal(t, ErrContextRequired, r.Err.Kind, "physical fragment is pending cumulative CRC, not a malformed standalone frame")
			require.NotContains(t, r.Events[0].Session, "Configuration")
		} else {
			require.Nil(t, r.Err)
			require.Contains(t, r.Events[0].Session, "Configuration")
		}
	}
	r := s.Feed(0, time.Unix(3, 0), data)
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Contains(t, r.Events[0].Session, "Data")
	s.Close("test")
	s.Close("again")
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestC37118ContinuationOwnershipAndClose(t *testing.T) {
	frames, data := c37ContinuationControl(256)
	for _, chunk := range []int{1, 14, 16, 1200, 65535} {
		t.Run(fmt.Sprint(chunk), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
			require.NoError(t, err)
			for i, w := range frames {
				var events []*ProtocolEvent
				for at := 0; at < len(w); at += chunk {
					b := bytes.Clone(w[at:min(at+chunk, len(w))])
					r := s.Feed(0, time.Unix(int64(i+1), 0), b)
					clear(b)
					events = append(events, r.Events...)
					if at+chunk < len(w) {
						require.Empty(t, r.Events)
						require.Equal(t, ErrNeedMore, r.Err.Kind)
					}
				}
				require.Len(t, events, 1)
				e := events[0]
				if i+1 < len(frames) {
					require.Equal(t, ErrContextRequired, e.sessionError.Kind)
					e.Session["Configuration Image Bytes"] = -1
					clear(e.Raw)
				} else {
					f, err := e.GetFields()
					require.NoError(t, err)
					cfg := f["Configuration"].(map[string]any)
					p := cfg["PMUs"].([]map[string]any)[0]
					names := p["Phasor Names"].([]string)
					require.Len(t, names, 256)
					for _, n := range names {
						require.Equal(t, string(bytes.Repeat([]byte{'A'}, 255)), n)
					}
					p["Phasor Names"] = nil
					e.Session = nil
					_, err = e.Decode()
					require.NoError(t, err)
				}
			}
			r := s.Feed(0, time.Unix(3, 0), data)
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			ps := r.Events[0].Session["Data"].([]map[string]any)
			require.Len(t, ps[0]["Phasors"].([]map[string]any), 256)
			require.Empty(t, s.Close("completed"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	for _, dir := range []int{0, 1} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
		require.NoError(t, err)
		r := s.Feed(dir, time.Unix(1, 0), frames[0])
		require.Equal(t, ErrContextRequired, r.Err.Kind)
		events := s.Close("partial")
		require.Len(t, events, 1)
		e := events[0]
		require.Equal(t, "incomplete", e.Status)
		require.Equal(t, dir, e.Direction)
		require.Equal(t, map[string]any{"Configuration Fragments": 1, "Configuration Image Bytes": 65535, "Cumulative CRC Validated": false}, e.Session)
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
		require.EqualValues(t, 1, s.Stats().Incomplete)
		require.Equal(t, ErrFatalSessionError, s.Feed(dir, time.Unix(2, 0), frames[1]).Err.Kind)
	}
}

func TestC37118ContinuationLogicalCodecBoundaries(t *testing.T) {
	frames, _ := c37ContinuationControl(256)
	image := bytes.Clone(frames[0])
	for _, w := range frames[1:] {
		image = append(image, w[16:]...)
	}
	_, err := stream_parser.DecodeC37118Message(image, nil, 4096)
	require.Error(t, err, "logical image must never be admitted as a physical frame")
	_, err = stream_parser.DecodeC37118ConfigurationImage(image, 4096, len(image)-1)
	require.ErrorIs(t, err, stream_parser.ErrC37118CollectionLimit)
	_, err = stream_parser.DecodeC37118ConfigurationImage(image, 256, len(image))
	require.ErrorIs(t, err, stream_parser.ErrC37118CollectionLimit)
	m, err := stream_parser.DecodeC37118ConfigurationImage(image, 4096, len(image))
	require.NoError(t, err)
	require.True(t, m.BodyDecoded)
	clear(image)
	require.Equal(t, string(bytes.Repeat([]byte{'A'}, 255)), m.Configuration.PMUs[0].PhasorNames[255])
	bad := bytes.Clone(frames[0])
	bad = append(bad, frames[1][16:]...)
	binary.BigEndian.PutUint16(bad[14:], 0)
	_, err = stream_parser.DecodeC37118ConfigurationImage(bad, 4096, len(bad))
	require.Error(t, err, "normalizing the first header before CRC is forbidden")
}
