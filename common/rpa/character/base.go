package character

import (
	"regexp"
	"strings"
	"yaklang/common/utils"
)

func String2Vec(str string) []interface{} {
	var vec = make([]int, 26)
	var inf = make([]interface{}, 0)
	lowerStr := strings.ToLower(str)
	strBytes := []byte(lowerStr)
	for _, byt := range strBytes {
		if byt >= 97+26 || byt < 97 {
			// log.Infof("error character: %s from %s", byt, str)
			continue
		}
		vec[byt-97]++
	}
	for _, num := range vec {
		// strNum := strconv.Itoa(num)
		floatNum := float64(num)
		inf = append(inf, floatNum)
	}
	return inf
}

func Delete_extra_space(s string) string {
	//删除字符串中的多余空格，有多个空格时，仅保留一个空格
	s1 := strings.Replace(s, "	", " ", -1)       //替换tab为空格
	regstr := "\\s{2,}"                          //两个及两个以上空格的正则表达式
	reg, _ := regexp.Compile(regstr)             //编译正则表达式
	s2 := make([]byte, len(s1))                  //定义字符数组切片
	copy(s2, s1)                                 //将字符串复制到切片
	spc_index := reg.FindStringIndex(string(s2)) //在字符串中搜索
	for len(spc_index) > 0 {                     //找到适配项
		s2 = append(s2[:spc_index[0]+1], s2[spc_index[1]:]...) //删除多余空格
		spc_index = reg.FindStringIndex(string(s2))            //继续在字符串中搜索
	}
	return string(s2)
}

func GetOnlyLetters(s string) (string, error) {
	comStr := "[^a-zA-Z]+"
	reg, err := regexp.Compile(comStr)
	if err != nil {
		return "", utils.Errorf("reg exp compile %s error:%s", comStr, err)
	}
	result := reg.ReplaceAllString(s, "")
	return result, nil
}
