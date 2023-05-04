package yserx

type JavaBlockData struct {
	Type        byte   `json:"type"`
	TypeVerbose string `json:"type_verbose"`
	IsLong      bool   `json:"is_long"`
	Size        uint64 `json:"size"`
	Contents    []byte `json:"contents"`
}

func (j *JavaBlockData) Marshal() []byte {
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
