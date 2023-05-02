package mfreader

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
)

func TestMultiFileLineReader_GetPercent(t *testing.T) {
	names := []string{}
	a, err := consts.TempFile("ab-*c.txt")
	if err != nil {
		panic(err)
	}
	for _, r := range mutate.MutateQuick(`{{net(47.52.100.1/24)}}`) {
		a.WriteString(r + "\r\n")
	}
	a.Close()
	names = append(names, a.Name())

	a, err = consts.TempFile("ab-*d.txt")
	if err != nil {
		panic(err)
	}
	for _, r := range mutate.MutateQuick(`  12  {{net(47.52.11.1/24)}}`) {
		a.WriteString(r + "\r\n")
	}
	a.Close()
	names = append(names, a.Name())

	a, err = consts.TempFile("ab-*d.txt")
	if err != nil {
		panic(err)
	}
	for _, r := range mutate.MutateQuick(`{{net(11.52.11.1/22)}}`) {
		a.WriteString(r + "\n")
	}
	a.Close()
	names = append(names, a.Name())

	mr, err := NewMultiFileLineReader(names...)
	mr.currentFileIndex = 1
	if err != nil {
		panic(err)
	}
	show := func() {
		fmt.Printf("percent: %.2f line: %#v\n", mr.GetPercent(), mr.Text())
	}
	go func() {
		for {
			show()
			time.Sleep(time.Millisecond * 100)
		}
	}()
	for mr.Next() {
		show()
		time.Sleep(time.Millisecond * 10)
	}
}

func TestMultiFileLineReader_Recover(t *testing.T) {
	// 创建临时文件
	tmpFile1, err := ioutil.TempFile("", "targets1.txt")
	if err != nil {
		log.Errorf("创建临时文件失败: %v", err)
	}
	tmpFile2, err := ioutil.TempFile("", "targets2.txt")
	if err != nil {
		log.Errorf("创建临时文件失败: %v", err)
	}
	tmpFile3, err := ioutil.TempFile("", "targets3.txt")
	if err != nil {
		log.Errorf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile1.Name())
	defer os.Remove(tmpFile2.Name())
	defer os.Remove(tmpFile3.Name())

	// 向临时文件写入数据
	for i, r := range mutate.MutateQuick(`{{net(1.1.1.1/24)}}`) {
		if i >= 10 {
			break
		}
		tmpFile1.WriteString(r + "\r\n")
	}
	for i, r := range mutate.MutateQuick(`{{net(2.2.2.2/24)}}`) {
		if i >= 10 {
			break
		}
		tmpFile2.WriteString(r + "\r\n")
	}
	for i, r := range mutate.MutateQuick(`{{net(3.3.3.3/24)}}`) {
		if i >= 10 {
			break
		}
		tmpFile3.WriteString(r + "\r\n")
	}
	tmpFile1.Close()
	tmpFile2.Close()
	tmpFile3.Close()

	files := []string{tmpFile1.Name(), tmpFile2.Name(), tmpFile3.Name()}
	reader, err := NewMultiFileLineReader(files...)
	if err != nil {
		log.Errorf("创建MultiFileLineReader失败: %v", err)
	}

	// 读取一部分内容
	for i := 0; i < 5; i++ {
		if reader.Next() {
			line := reader.Text()
			t.Log(line)
		} else {
			break
		}
	}

	// 模拟中断，保存文件指针位置和currentFileIndex
	reader.fpPtrTable.Range(func(key, value interface{}) bool {
		t.Logf("保存文件 %s 的指针位置: %d\n", key, value)
		return true
	})
	t.Logf("保存currentFileIndex: %d\n", reader.currentFileIndex)

	// 模拟恢复读取，从保存的文件指针位置和currentFileIndex继续读取
	t.Log("恢复读取:")
	reader2, err := NewMultiFileLineReader(files...)
	if err != nil {
		log.Errorf("创建MultiFileLineReader失败: %v", err)
	}
	reader2.fpPtrTable = reader.fpPtrTable             // 从已保存的指针位置恢复读取
	reader2.currentFileIndex = reader.currentFileIndex // 恢复currentFileIndex

	for reader2.Next() {
		line := reader2.Text()
		t.Log(line)
	}
}
