package utils

import (
	"fmt"
	"github.com/glaslos/ssdeep"
	"github.com/mfonda/simhash"
	"gopkg.in/fatih/set.v0"
	"sort"
	"strconv"
	"yaklang.io/yaklang/common/go-funk"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils/mixer"
)

func SimHash(raw []byte) uint64 {
	return simhash.Simhash(simhash.NewWordFeatureSet(raw))
}

func SSDeepHash(raw []byte) string {
	hash, err := ssdeep.FuzzyBytes(raw)
	if err != nil {
		log.Warn(err.Error())
		return ""
	}

	return hash
}

func CalcSimilarity(raw ...[]byte) float64 {
	var (
		err     error
		percent float64
	)

	lens := funk.Map(raw, func(i []byte) int {
		return len(i)
	}).([]int)
	maxLength := funk.MaxInt(lens)
	minLength := funk.MinInt(lens)

	if maxLength <= 0 {
		return 0
	}

	if maxLength <= 2000 {
		percent, err = CalcTextSubStringStability(raw...)
		if err != nil {
			log.Errorf("calc text substr similarity/stability failed: %s", err)
			return 0
		}
		return percent
	}

	if minLength >= 30000 {
		percent, err = CalcSSDeepStability(raw...)
		if err == nil {
			return percent
		}
	}

	percent, err = CalcSimHashStability(raw...)
	if err == nil {
		return percent
	}
	return 0
}

func CalcTextSubStringStability(raw ...[]byte) (float64, error) {
	var samples []string
	for _, i := range raw {
		samples = append(samples, string(i))
	}

	if len(samples) <= 0 {
		return 1, Errorf("no enough samples")
	}

	m, err := mixer.NewMixer(samples, samples)
	if err != nil {
		return 1, Errorf("create mixer failed: %s", err)
	}

	var max float64 = 0
	var min float64 = 1
	for m.Next() == nil {
		results := m.Value()
		hash1, hash2 := results[0], results[1]
		score := similarText(hash1, hash2)

		if score <= min {
			min = score
		}
		if score >= max {
			max = score
		}
	}
	return min, nil
}

// 稳定性定义为最远距离 / 最低分数
func CalcSSDeepStability(req ...[]byte) (float64, error) {
	var hash []string
	for _, r := range req {
		h := SSDeepHash(r)
		if h == "" {
			continue
		}
		hash = append(hash, h)
	}

	if len(hash) <= 0 {
		return 1, Errorf("no enough hash")
	}

	m, err := mixer.NewMixer(hash, hash)
	if err != nil {
		return 1, Errorf("create mixer failed: %s", err)
	}

	var max = 0
	var min = 100
	for m.Next() == nil {
		results := m.Value()
		hash1, hash2 := results[0], results[1]
		score, err := ssdeep.Distance(hash1, hash2)
		if err != nil {
			continue
		}

		if score <= min {
			min = score
		}
		if score >= max {
			max = score
		}
	}
	return float64(min) / float64(100), nil
}

// 计算 simhash 稳定性
func CalcSimHashStability(req ...[]byte) (float64, error) {
	var hash []string
	for _, r := range req {
		h := SimHash(r)
		hash = append(hash, fmt.Sprint(h))
	}

	m, err := mixer.NewMixer(hash, hash)
	if err != nil {
		return 0, err
	}
	var max uint8 = 0
	var min uint8 = 255
	for m.Next() == nil {
		results := m.Value()
		hash1, _ := strconv.ParseUint(results[0], 10, 64)
		hash2, _ := strconv.ParseUint(results[1], 10, 64)
		res := simhash.Compare(hash1, hash2)
		if res <= min {
			min = res
		}
		if res >= max {
			max = res
		}
	}
	return (256 - float64(max)) / float64(256), nil
}

// return the len of longest string both in str1 and str2 and the positions in str1 and str2
func SimilarStr(str1 []rune, str2 []rune) (int, int, int) {
	var sameLen, tmp, pos1, pos2 = 0, 0, 0, 0
	len1, len2 := len(str1), len(str2)
	for p := 0; p < len1; p++ {
		for q := 0; q < len2; q++ {
			tmp = 0
			for p+tmp < len1 && q+tmp < len2 && str1[p+tmp] == str2[q+tmp] {
				tmp++
			}
			if tmp > sameLen {
				sameLen, pos1, pos2 = tmp, p, q
			}
		}
	}
	return sameLen, pos1, pos2
}

// return the total length of longest string both in str1 and str2
func similarChar(str1 []rune, str2 []rune) int {
	maxLen, pos1, pos2 := SimilarStr(str1, str2)
	total := maxLen
	if maxLen != 0 {
		if pos1 > 0 && pos2 > 0 {
			total += similarChar(str1[:pos1], str2[:pos2])
		}
		if pos1+maxLen < len(str1) && pos2+maxLen < len(str2) {
			total += similarChar(str1[pos1+maxLen:], str2[pos2+maxLen:])
		}
	}
	return total
}

// return a int value in [0, 1], which stands for match level
func similarText(str1 string, str2 string) float64 {
	txt1, txt2 := []rune(str1), []rune(str2)
	if len(txt1) == 0 || len(txt2) == 0 {
		return 0
	}
	totalLength := float64(similarChar(txt1, txt2))
	return totalLength * 2 / float64(len(txt1)+len(txt2))
}

func checkIsSamePage(BaseBody []byte, currentBody []byte, boundary float64) bool {
	currentHTML := string(currentBody)
	baseHTML := string(BaseBody)
	isSamePage := false
	// 先计算PageRatio
	ratio := similarText(currentHTML, baseHTML)
	if ratio > boundary {
		isSamePage = true
	}
	return isSamePage
}

func GetSameSubStringsRunes(text1, text2 []rune) [][]rune {
	var ln, pos1, pos2 = 0, 0, 0
	var results [][]rune

	for {
		ln, pos1, pos2 = SimilarStr(text1, text2)
		if ln > 0 {
			result := text1[pos1 : pos1+ln]
			results = append(results, result)
		} else {
			return results
		}
		text1 = text1[pos1+ln:]
		text2 = text2[pos2+ln:]
	}
}

func GetSameSubStrings(raw ...string) []string {
	if len(raw) < 2 {
		return nil
	}

	m, err := mixer.NewMixer(raw, raw)
	if err != nil {
		return nil
	}

	var results []set.Interface

	var visited []string
	for {
		res := m.Value()
		sort.Strings(res)
		hash := CalcSha1(res[0], res[1])
		if res[0] != res[1] && !StringSliceContain(visited, hash) {
			visited = append(visited, hash)

			subStrs := GetSameSubStringsRunes([]rune(res[0]), []rune(res[1]))
			var tmp = set.New(set.ThreadSafe)
			for _, r := range subStrs {
				tmp.Add(string(r))
			}
			results = append(results, tmp)
		}

		err := m.Next()
		if err != nil {
			break
		}
	}
	if len(results) > 2 {
		return set.StringSlice(set.Intersection(results[0], results[1], results[2:]...))
	}

	if len(results) == 2 {
		return set.StringSlice(set.Intersection(results[0], results[1]))
	}

	if len(results) == 1 {
		return set.StringSlice(results[0])
	}

	return nil
}
