package randomforest

import (
	"bufio"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/rpa/character"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"math/rand"
	"os"
	"strings"
	"time"
)

func RandomNumberGenerate(start int, end int, count int) ([]int, error) {
	if end < start || (end-start) < count {
		return []int{}, utils.Errorf("start larger than end or too much count.")
	}
	nums := make([]int, 0)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := start; i < end; i++ {
		nums = append(nums, i)
	}
	all_length := len(nums)
	var cache []int
	var length, randInt int
	for {
		length = len(nums)
		if length == count {
			// fmt.Println(nums, cache)
			return nums, nil
		} else if length+count == all_length {
			// fmt.Println(nums, cache)
			return cache, nil
		}
		randInt = r.Intn(length)
		cache = append(cache, nums[randInt])
		nums = append(nums[:randInt], nums[randInt+1:]...)
	}
}

func ReadFile(path string) ([][]interface{}, []string, error) {
	fi, err := os.Open(path)
	if err != nil {
		log.Infof("open files error:%s", err)
	}
	defer fi.Close()
	reader := bufio.NewReader(fi)
	v := make([][]interface{}, 0)
	r := make([]string, 0)
	for {
		lineBytes, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return [][]interface{}{}, []string{}, utils.Errorf("read bytes error:%s", err)
		}
		if err == io.EOF {
			break
		}
		afterDeleteSpace := character.Delete_extra_space(strings.ToLower(strings.TrimSpace(string(lineBytes))))
		kv := strings.Split(afterDeleteSpace, " ")
		letters, err := character.GetOnlyLetters(kv[0])
		if err != nil {
			log.Errorf("get letters error: %s", err)
			continue
		}
		vec := character.String2Vec(letters)
		var rr string
		if len(kv) > 1 {
			rr = "1"
		} else {
			rr = "0"
		}
		v = append(v, vec)
		r = append(r, rr)
	}
	return v, r, nil
}

func SplitDatafromY(X [][]interface{}, y []string) ([][]interface{}, [][]interface{}) {
	positive := make([][]interface{}, 0)
	negative := make([][]interface{}, 0)
	for num, info := range y {
		if strings.Compare(info, "1") == 0 {
			positive = append(positive, X[num])
		} else if strings.Compare(info, "0") == 0 {
			negative = append(negative, X[num])
		} else {
			log.Errorf("error unknown compare result: %s", info)
		}
	}
	return positive, negative
}
