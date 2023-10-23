package binx

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"net"
	"strings"
	"time"
)

type PartDescriptor struct {
	Identifier        string
	IdentifierVerbose string

	// List / Struct
	// if SubPartLength > 0 ? use List
	// if SubPartLength <= 0 ? use struct
	SubPartLength     uint64
	SubPartDescriptor []*PartDescriptor

	_byteOrder int

	// size
	size     uint64
	sizeFrom string
	typeFlag BinaryTypeVerbose

	// for net.Conn
	timeout time.Duration
}

type PartDescriptorBuilder func(*PartDescriptor)

func NewDefaultNetworkPartDescriptor() *PartDescriptor {
	return &PartDescriptor{}
}

func (p *PartDescriptor) Config(i ...PartDescriptorBuilder) {
	for _, v := range i {
		v(p)
	}
}

func (p *PartDescriptor) Name(id string, verbose ...string) *PartDescriptor {
	p.Identifier = id
	p.IdentifierVerbose = strings.Join(verbose, " ")
	return p
}

func (p *PartDescriptor) SetIdentifier(id string, verbose ...string) *PartDescriptor {
	p.Identifier = id
	p.IdentifierVerbose = strings.Join(verbose, " ")
	return p
}

func (p *PartDescriptor) Verbose(verbose ...string) *PartDescriptor {
	p.IdentifierVerbose = strings.Join(verbose, " ")
	return p
}

func (p *PartDescriptor) IsEmpty() bool {
	if p == nil {
		return true
	}

	if p.size == 0 && p.sizeFrom == "" {
		if len(p.SubPartDescriptor) == 0 {
			return true
		}
	}
	return false
}

func (p *PartDescriptor) GetTimeoutDuration() time.Duration {
	if p == nil {
		return 5 * time.Second
	}
	if p.timeout > 0 {
		return p.timeout
	}
	return 5 * time.Second
}

func read(lastResults []ResultIf, p *PartDescriptor, reader io.Reader, startOffset int64) ([]ResultIf, int64, []ResultIf, error) {
	if p.IsEmpty() {
		return nil, startOffset, lastResults, nil
	}

	conn, ok := reader.(net.Conn)
	if ok {
		conn.SetReadDeadline(time.Now().Add(p.GetTimeoutDuration()))
		defer func() {
			conn.SetReadDeadline(time.Time{})
		}()
	}

	handleSubPartDesc := func(subs []*PartDescriptor, merged ResultCompactIf) error {
		var firstOffset int64 = -1
		var bufs = make([]ResultIf, len(p.SubPartDescriptor))
		var bufsSize int64
		for i := 0; i < int(p.SubPartLength); i++ {
			var err error
			var results []ResultIf
			results, startOffset, lastResults, err = read(lastResults, p.SubPartDescriptor[i], reader, startOffset)
			if err != nil {
				return err
			}
			for _, subResult := range results {
				if firstOffset < 0 {
					firstOffset = subResult.GetInt64Offset()
				}
				bufs[i] = subResult
				bufsSize += int64(len(bufs[i].GetBytes()))
			}
		}
		merged.SetOffset(firstOffset)
		var finalBytes = make([]byte, bufsSize)
		var offset = 0
		for _, buf := range bufs {
			copy(finalBytes[offset:], buf.GetBytes())
			offset += len(buf.GetBytes())
		}
		merged.SetBytes(finalBytes)
		merged.SetResults(bufs)
		return nil
	}

	// list
	if p.SubPartLength > 0 {
		if len(p.SubPartDescriptor) == 0 {
			return nil, startOffset, lastResults, utils.Error("SubPartLength > 0 but SubPartDescriptor is empty")
		}
		if p.SubPartLength > uint64(len(p.SubPartDescriptor)) {
			return nil, startOffset, lastResults, utils.Error("SubPartLength > len(SubPartDescriptor)")
		}

		p.SubPartDescriptor = p.SubPartDescriptor[:p.SubPartLength]
		result := NewListResult()
		result.Identifier = p.Identifier
		result.IdentifierVerbose = p.IdentifierVerbose
		err := handleSubPartDesc(p.SubPartDescriptor, result)
		if err != nil {
			return nil, startOffset, lastResults, err
		}
		return []ResultIf{
			result,
		}, startOffset, lastResults, nil
	}

	// struct
	if len(p.SubPartDescriptor) > 0 {
		result := NewStructResult()
		result.Identifier = p.Identifier
		result.IdentifierVerbose = p.IdentifierVerbose
		err := handleSubPartDesc(p.SubPartDescriptor, result)
		if err != nil {
			return nil, startOffset, lastResults, err
		}
		return []ResultIf{
			result,
		}, startOffset, lastResults, nil
	}

	// ordinary
	if p.size <= 0 && p.sizeFrom != "" {
		ret := FindResultByIdentifier(lastResults, p.sizeFrom)
		if ret == nil {
			return nil, 0, nil, utils.Errorf("sizeFrom %v not found", p.sizeFrom)
		}
		p.size = uint64(utils.InterfaceToInt(ret.Value()))
	}
	if p.size > 0 {
		var byteBuf = make([]byte, 1)
		var readBuffer = make([]byte, p.size)
		for i := uint64(0); i < p.size; i++ {
			bytes, err := io.ReadFull(reader, byteBuf)
			if bytes != 1 {
				if err == nil {
					err = io.EOF
				}
				return nil, startOffset, lastResults, err
			}
			startOffset++
			readBuffer[i] = byteBuf[0]
		}
		result := NewResult(readBuffer)
		result.SetOffset(startOffset - int64(p.size))
		result.Identifier = p.Identifier
		result.IdentifierVerbose = p.IdentifierVerbose
		if result.Identifier == "" {
			result.Identifier = fmt.Sprintf("offset_%v_%v", result.GetOffset(), startOffset)
		}
		result.Type = p.typeFlag
		lastResults = append(lastResults, result)
		return []ResultIf{result}, startOffset, lastResults, nil
	}

	return nil, startOffset, lastResults, utils.Error("unknown error, size or size from is not valid")
}

func NewListDescriptor(builder ...*PartDescriptor) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.SubPartLength = uint64(len(builder))
	descriptor.SubPartDescriptor = builder
	return descriptor
}

func NewStructDescriptor(builder ...*PartDescriptor) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.SubPartLength = 0
	descriptor.SubPartDescriptor = builder
	return descriptor
}

// builder
func NewUint8(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 1
	descriptor.typeFlag = Uint8
	return descriptor.Name(name, values...)
}

func NewByte(name string, values ...string) *PartDescriptor {
	return NewUint8(name, values...)
}

func NewUint16(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 2
	descriptor.typeFlag = Uint16
	return descriptor.Name(name, values...)
}

func NewUint32(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 4
	descriptor.typeFlag = Uint32
	return descriptor.Name(name, values...)
}

func NewUint64(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 8
	descriptor.typeFlag = Uint64
	return descriptor.Name(name, values...)
}

func NewInt8(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 1
	descriptor.typeFlag = Int8
	return descriptor.Name(name, values...)
}

func NewInt16(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 2
	descriptor.typeFlag = Int16
	return descriptor.Name(name, values...)
}

func NewInt32(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 4
	descriptor.typeFlag = Int32
	return descriptor.Name(name, values...)
}

func NewInt64(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 8
	descriptor.typeFlag = Int64
	return descriptor.Name(name, values...)
}

func NewBytes(name string, size any) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	sizeFrom := utils.InterfaceToString(size)
	if utils.IsValidInteger(sizeFrom) {
		descriptor.size = uint64(utils.InterfaceToInt(size))
	} else {
		descriptor.sizeFrom = sizeFrom
	}
	descriptor.typeFlag = Bytes
	return descriptor.Name(name)
}

func NewBuffer(name string, size any) *PartDescriptor {
	return NewBytes(name, size)
}

func NewBool(name string, verbose ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 1
	descriptor.typeFlag = Bool
	return descriptor.Name(name, verbose...)
}
