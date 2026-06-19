package yserx

type JavaBlockData struct {
	Type        byte   `json:"type"`
	TypeVerbose string `json:"type_verbose"`
	IsLong      bool   `json:"is_long"`
	Size        uint64 `json:"size"`
	Contents    []byte `json:"contents"`
}

func (j *JavaBlockData) Marshal(cfg *MarshalContext) []byte {
	var header []byte
	if !j.IsLong {
		header = append(header, TC_BLOCKDATA)
		header = append(header, IntToByte(int(j.Size))...)
	} else {
		header = append(header, TC_BLOCKDATALONG)
		header = append(header, Uint64To4Bytes(j.Size)...)
	}
	header = append(header, j.Contents...)
	return header
}

// NewJavaBlockDataBytes 创建一个 Java 序列化的块数据对象(TC_BLOCKDATA)，承载原始字节
// 在 yak 中通过 java.NewJavaBlockDataBytes 调用，常用于 writeObject 自定义数据
// 参数:
//   - raw: 块数据的原始字节内容
//
// 返回值:
//   - Java 块数据序列化对象
//
// Example:
// ```
// // 该示例为示意性用法：构造原始块数据
// block = java.NewJavaBlockDataBytes([]byte("data"))
// println(block.TypeVerbose)
// ```
func NewJavaBlockDataBytes(raw []byte) *JavaBlockData {
	if len(raw) <= 0xff {
		d := &JavaBlockData{
			TypeVerbose: tcToVerbose(TC_BLOCKDATA),
			Type:        TC_BLOCKDATA,
			Size:        uint64(len(raw)),
			Contents:    raw,
		}
		initTCType(d)
		return d
	} else {
		d := &JavaBlockData{
			Type:        TC_BLOCKDATALONG,
			TypeVerbose: tcToVerbose(TC_BLOCKDATALONG),
			IsLong:      true,
			Size:        uint64(len(raw)),
			Contents:    raw,
		}
		initTCType(d)
		return d
	}
}
