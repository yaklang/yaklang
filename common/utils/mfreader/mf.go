package mfreader

import (
	"io"
	"os"
	"sync"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type MultiFileLineReader struct {
	// 文件名数组
	files []string

	// 当前使用到第一个文件了？
	currentFileIndex int

	// 这两个函数通过 nextline 自动设置
	currentFp   *os.File
	currentLine string
	// 最后一次读取的行的指针位置
	currentPtr int64

	// 文件名->指针位置
	fpPtrTable sync.Map
	// 文件名->大小
	fSizeTable sync.Map
}

func (m *MultiFileLineReader) GetPercent() float64 {
	if m.currentFileIndex >= len(m.files) {
		return 0
	}

	var total int64
	var finishedFile int64
	for index, f := range m.files {
		stat, _ := os.Stat(f)
		if stat != nil {
			if index < m.currentFileIndex {
				finishedFile += stat.Size()
			}

			if index == m.currentFileIndex && m.currentFp != nil {
				offset, _ := m.currentFp.Seek(0, 1)
				finishedFile += offset
			}
			total += stat.Size()
		}
	}
	return float64(finishedFile) / float64(total)
}

func (m *MultiFileLineReader) GetLastRecordPtr() int64 {
	ptr, ok := m.fpPtrTable.Load(m.currentFp.Name())
	if ok {
		return ptr.(int64)
	}
	return 0
}

// SetRecoverPtr 设置扫描文件对应的指针位置
func (m *MultiFileLineReader) SetRecoverPtr(file string, ptr int64) error {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return utils.Errorf("file %s not exist", file)
	}
	m.fpPtrTable.Store(file, ptr)
	return nil
}

func (m *MultiFileLineReader) SetCurrFileIndex(index int) {
	m.currentFileIndex = index
}

func NewMultiFileLineReader(files ...string) (*MultiFileLineReader, error) {
	for _, i := range files {
		fp, err := os.Open(i)
		if err != nil {
			return nil, utils.Errorf("os.Open/Readable %v failed: %v", i, err)
		}
		fp.Close()
	}

	m := &MultiFileLineReader{files: files}
	return m, nil
}

func (m *MultiFileLineReader) Next() bool {
	line, err := m.nextLine()
	if err != nil {
		return false
	}
	m.currentLine = line
	return true
}

func (m *MultiFileLineReader) Text() string {
	return m.currentLine
}

func (m *MultiFileLineReader) nextLine() (string, error) {
NEXTFILE:
	switch true {
	case m.currentFp == nil && m.currentFileIndex == 0:
		// 第一次执行
		if len(m.files) <= 0 {
			return "", utils.Error("empty files")
		}
		fp, err := os.Open(m.files[m.currentFileIndex])
		if err != nil {
			log.Errorf("open %v failed: %v", m.files, err)
			m.currentFileIndex++
			goto NEXTFILE
		}
		m.currentFp = fp
		// 当 currentFileIndex 为 0 时 应当也可以进行恢复扫描
		m.restoreFilePointer() // 恢复文件指针位置

		goto NEXTFILE
	case m.currentFp == nil && m.currentFileIndex > 0:
		// 恢复场景
		if len(m.files) <= 0 {
			return "", utils.Errorf("empty files")
		}

		if m.currentFileIndex >= len(m.files) {
			return "", io.EOF
		}
		fp, err := os.Open(m.files[m.currentFileIndex])
		if err != nil {
			log.Errorf("open %v failed: %v", m.files, err)
			m.currentFileIndex++
			goto NEXTFILE
		}
		m.currentFp = fp

		m.restoreFilePointer() // 恢复文件指针位置

		goto NEXTFILE
	case m.currentFp != nil:
		lines, n, err := utils.ReadLineEx(m.currentFp)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				log.Infof("use next fileindex: %v", m.currentFileIndex+1)
			} else {
				log.Errorf("read file failed: %s, use next file", err)
			}
		}

		if n > 0 {
			// 保存读取位置
			m.fpPtrTable.Store(m.currentFp.Name(), m.currentPtr)
			// 先读取然后再加,在恢复场景中，应当从上一次中断的目标开始恢复
			// 比如
			//	1.1.1.1
			//	1.1.1.2
			//	1.1.1.3
			//	本次中断在 1.1.1.2 下次恢复应当还是从 1.1.1.2 开始
			m.currentPtr += n
			return string(lines), nil
		} else {
			m.currentFp.Close()
			m.currentFp = nil
			m.currentPtr = 0
			m.currentFileIndex++
			goto NEXTFILE
		}
	default:
		return "", utils.Error("BUG: unknown status")
	}
}

// 恢复文件指针位置
func (m *MultiFileLineReader) restoreFilePointer() {
	ptr, ok := m.fpPtrTable.Load(m.currentFp.Name())
	if ok {
		offset, ok := ptr.(int64)
		if ok {
			if _, err := m.currentFp.Seek(offset, 0); err != nil {
				log.Errorf("seek file failed: %v", err)
			}
			m.currentPtr = offset

		}
	}
}
