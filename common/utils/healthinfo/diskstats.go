package healthinfo

import (
	"bytes"
	"io/ioutil"
	"strconv"
	"github.com/yaklang/yaklang/common/log"
)

// sample:
//
//	0       1 2    3       4    5        6      7        8      9         10      11 12      13
//
// "   8       1 sda1 3774108 2444 30194552 637303 11746691 283841 529382982 90114806 0 1657750 90747516"
//
//	%4d     %7d %s   %lu     %lu  %lu      %u     %lu      %lu    %lu       %u      %u %u      %u\n
//
// ^ from linux/block/genhd.c ~ line 1139
type Diskstat struct {
	Major         uint   //  0: major dev no
	Minor         uint   //  1: minor dev no
	Name          string //  2: device name
	ReadComplete  uint64 //  3: reads completed
	ReadMerged    uint64 //  4: writes merged
	ReadSectors   uint64 //  5: sectors read
	ReadMs        uint   //  6: ms spent reading
	WriteComplete uint64 //  7: reads completed
	WriteMerged   uint64 //  8: writes merged
	WriteSectors  uint64 //  9: sectors read
	WriteMs       uint   // 10: ms spent writing
	IOPending     uint   // 11: number of IOs currently in progress
	IOMs          uint   // 12: jiffies_to_msecs(part_stat_read(hd, io_ticks))
	IOQueueMs     uint   // 13: jiffies_to_msecs(part_stat_read(hd, time_in_queue))
}

func ReadDiskstats() (out []Diskstat, _ error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("read diskt stats failed; %s", err)
		}
	}()

	data, err := ioutil.ReadFile("/proc/diskstats")
	if err != nil {
		return nil, err
	}

	// slice off the last byte, a newline, to prevent a phantom row
	rows := bytes.Split(data[0:len(data)-1], []byte{byte('\n')})

	// bytes.Split doesn't handle variable whitespace between fields
	fields := make([]string, 14)
	field := make([]byte, 32)
	var f, i int
	for _, row := range rows {
		f = 0
		i = 0
		for _, b := range row[0 : len(row)-1] {
			if b != byte(' ') {
				if i >= 32 {
					continue
				}
				field[i] = b
				i++
			} else if i > 0 {

				if f >= 14 {
					continue
				}

				fields[f] = string(field[0:i])
				f++
				i = 0
			}
		}

		st := Diskstat{
			fldtouint(fields, 0),
			fldtouint(fields, 1),
			fields[2],
			fldtouint64(fields, 3),
			fldtouint64(fields, 4),
			fldtouint64(fields, 5),
			fldtouint(fields, 6),
			fldtouint64(fields, 7),
			fldtouint64(fields, 8),
			fldtouint64(fields, 9),
			fldtouint(fields, 10),
			fldtouint(fields, 11),
			fldtouint(fields, 12),
			fldtouint(fields, 13),
		}

		out = append(out, st)
	}

	return out, nil
}

func fldtouint(fields []string, idx int) uint {
	return uint(fldtouint64(fields, idx))
}

func fldtouint64(fields []string, idx int) uint64 {
	if len(fields[idx]) == 0 {
		return 0
	}

	out, err := strconv.ParseUint(fields[idx], 10, 64)
	if err != nil {
		log.Errorf("Failed to convert field %d, value '%s', device '%s' to int: %s\n",
			idx, fields[idx], fields[2], err)
		return 0
	}

	return out
}

type UsageStat struct {
	Path string `json:"path"`
	//Fstype            string  `json:"fstype"`
	Total             uint64  `json:"total"`
	Free              uint64  `json:"free"`
	Used              uint64  `json:"used"`
	UsedPercent       float64 `json:"usedPercent"`
	InodesTotal       uint64  `json:"inodesTotal"`
	InodesUsed        uint64  `json:"inodesUsed"`
	InodesFree        uint64  `json:"inodesFree"`
	InodesUsedPercent float64 `json:"inodesUsedPercent"`
}

// Unescape escaped octal chars (like space 040, ampersand 046 and backslash 134) to their real value in fstab fields issue#555
func unescapeFstab(path string) string {
	escaped, err := strconv.Unquote(`"` + path + `"`)
	if err != nil {
		return path
	}
	return escaped
}

//func getFsType(stat unix.Statfs_t) string {
//	return IntToString(stat.Fstypename[:])
//}

func IntToString(orig []int8) string {
	ret := make([]byte, len(orig))
	size := -1
	for i, o := range orig {
		if o == 0 {
			size = i
			break
		}
		ret[i] = byte(o)
	}
	if size == -1 {
		size = len(orig)
	}

	return string(ret[0:size])
}
