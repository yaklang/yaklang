// Package tools
// @Author bcy2007  2023/7/14 14:54
package tools

import (
	"io"
	"math/rand"
	"os"
	"strconv"
	"time"
)

// 读取文件到二进制
func ReadFile(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	return content, err
}

// 写入二进制到文件
func WriteFile(fileName string, strTest []byte) error {
	var f *os.File
	var err error
	if CheckFileExist(fileName) { //文件存在
		f, err = os.OpenFile(fileName, os.O_APPEND, 0666) //打开文件
		if err != nil {
			return err
		}
	} else { //文件不存在
		f, err = os.Create(fileName) //创建文件
		if err != nil {
			return err
		}
	}
	defer f.Close()
	//将文件写进去
	_, err1 := io.WriteString(f, string(strTest))
	if err1 != nil {
		return err1
	}
	return nil
}

// 验证文件（目录）是否存在
func CheckFileExist(fileName string) bool {
	_, err := os.Stat(fileName)
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

// 判断所给路径是否为文件夹
func IsDir(path string) bool {
	s, err := os.Stat(path)
	if err != nil {
		return false
	}
	return s.IsDir()
}

// 判断所给路径是否为文件
func IsFile(path string) bool {
	return !IsDir(path)
}

// 调用os.MkdirAll递归创建文件夹
func CreateDir(path string) error {
	if !CheckFileExist(path) {
		err := os.MkdirAll(path, os.ModePerm)
		return err
	}
	return nil
}

// 删除文件
func RemoveFile(path string) error {
	return os.Remove(path)
}

// 获取一个随机的临时文件名
func GetFileTmpName(preString string, rand int) string {
	timeUnixNano := time.Now().UnixNano()
	timeString := strconv.FormatInt(timeUnixNano, 10)

	return preString + "_" + timeString + "_" + GetRandomString(rand)
}

func GetRandomString(n int) string {
	rand.Seed(time.Now().UnixNano())
	str := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := []byte(str)
	var result []byte
	for i := 0; i < n; i++ {
		result = append(result, bytes[rand.Intn(len(bytes))])
	}
	return string(result)
}
