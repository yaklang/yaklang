package cveresources

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type CPE struct {
	//cpe:2.3:o:freebsd:freebsd:2.2.5:*:*:*:*:*:*:*
	Part, Vendor, Product, Version, Edition string
}

type versionRule func(cpe CPE) (bool, float64)

// ParseToCPE 将字符串解析成CPE结构体
func ParseToCPE(cpeStr string) (*CPE, error) {
	quoted := false
	info := strings.FieldsFunc(cpeStr, func(r rune) bool {
		if r == '\\' {
			quoted = true
			return false
		}
		ret := !quoted && r == ':'
		if quoted {
			quoted = false
		}
		return ret
	})

	if info[1] == "2.3" {
		if len(info) != 13 {
			return nil, errors.New("format error: wrong CPE")
		} else {
			return &CPE{
				Part:    info[2],
				Vendor:  info[3],
				Product: info[4],
				Version: info[5],
				Edition: info[6],
			}, nil
		}
	} else if info[1] == "/a" || info[1] == "/h" || info[1] == "/o" {
		edition := "*"
		if len(info) >= 6 {
			edition = info[5]
		}
		return &CPE{
			Part:    strings.Trim(info[1], "/"),
			Vendor:  info[2],
			Product: info[3],
			Version: info[4],
			Edition: edition,
		}, nil
	} else {
		return nil, errors.New("format error: wrong CPE")
	}
}

func Set(s []string) []string {
	result := make([]string, 0, len(s))
	temp := map[string]struct{}{}
	for _, item := range s {
		if _, ok := temp[item]; !ok {
			temp[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

func (n Nodes) GetVendor() []string {
	var Vendors []string
	if len(n.Children) > 0 {
		for _, insideNode := range n.Children {
			Vendors = append(Vendors, insideNode.GetVendor()...)
		}
	} else {
		for _, match := range n.CpeMatch {
			if match.Vulnerable == true {
				cpe, err := ParseToCPE(match.Cpe23URI)
				if err != nil {
					fmt.Println(match.Cpe23URI)
				}
				if err != nil {
					log.Error(err)
					panic(err)
				}
				Vendors = append(Vendors, cpe.Vendor)
			}
		}
	}
	return Set(Vendors)
}

func (n Nodes) GetProduct() []string {
	var Products []string
	if len(n.Children) > 0 {
		for _, insideNode := range n.Children {
			Products = append(Products, insideNode.GetProduct()...)
		}
	} else {
		for _, match := range n.CpeMatch {
			if match.Vulnerable == true {
				cpe, err := ParseToCPE(match.Cpe23URI)
				if err != nil {
					log.Error(err)
					panic(err)
				}
				Products = append(Products, cpe.Product)
			}
		}
	}
	return Set(Products)
}

func (n Nodes) GetProductVersion(name string) []map[string]string {
	var version []map[string]string

	if len(n.Children) > 0 {
		for _, insideNode := range n.Children {
			version = append(version, insideNode.GetProductVersion(name)...)
		}
	} else {
		for _, match := range n.CpeMatch {
			cpe, err := ParseToCPE(match.Cpe23URI)
			if err != nil {
				log.Error(err)
				panic(err)
			}
			if cpe.Product != name {
				continue
			}
			if match.VersionEndExcluding != "" || match.VersionStartExcluding != "" || match.VersionEndIncluding != "" || match.VersionStartIncluding != "" {
				version = append(version, match.getVersionMatchRule())
			} else {
				currentVersion := make(map[string]string)
				if cpe.Edition != "-" && cpe.Edition != "*" {
					currentVersion["current"] = cpe.Version + cpe.Edition
				} else {
					currentVersion["current"] = cpe.Version
				}
				version = append(version, currentVersion)
			}
		}
	}
	return version
}

func (n Nodes) Result(CheckCpe []CPE) float64 {
	if len(n.Children) > 0 {
		var insideLevel []float64
		for _, insideNode := range n.Children {
			insideLevel = append(insideLevel, insideNode.Result(CheckCpe))
		}
		sort.Float64s(insideLevel)
		return insideLevel[len(insideLevel)-1]
	} else {
		var Level float64 = 0
		switch n.Operator {
		case "OR":
			for _, match := range n.CpeMatch {
				for _, cpe := range CheckCpe {
					if match.Calculate(cpe) > Level {
						Level = match.Calculate(cpe)
					}
				}
			}
		// AND 模式下计算置信度
		case "AND":
			for _, match := range n.CpeMatch {
				var insideLevel float64 = 0
				for _, cpe := range CheckCpe {
					if match.Calculate(cpe) > insideLevel {
						insideLevel = match.Calculate(cpe)
					}
				}
				Level += insideLevel
			}
		default:
			panic("Operator err")

		}
		return Level
	}
}

func (m CpeMatch) Calculate(cpe CPE) float64 {
	MatchCPE, err := ParseToCPE(m.Cpe23URI)
	if err != nil {
		log.Error(err)
		panic(err)
	}
	var baseScore float64
	if m.Vulnerable {
		baseScore = 8
	} else if MatchCPE.Part == "a" {
		baseScore = 6
	} else if MatchCPE.Part == "o" {
		baseScore = 4
	} else if MatchCPE.Part == "h" {
		baseScore = 2
	}

	//Vendor Check
	if MatchCPE.Vendor != cpe.Vendor && MatchCPE.Vendor != "*" && cpe.Vendor != "*" {
		return 0
	}
	//Product Check
	if MatchCPE.Product != cpe.Product && MatchCPE.Product != "*" && cpe.Product != "*" {
		return 0
	}
	if cpe.Version == "*" {
		return baseScore
	}
	//Version Check
	if MatchCPE.Version == "*" {
		if m.VersionEndExcluding == "" && m.VersionEndIncluding == "" && m.VersionStartExcluding == "" && m.VersionStartIncluding == "" {
			return baseScore
		} else {
			rules := m.getVersionRule()
			var coe = 1.0
			for _, rule := range rules {
				flag, coeItem := rule(cpe)

				if flag {
					coe = coe * coeItem
				} else {
					return 0
				}
			}
			return baseScore * coe
		}
	} else if MatchCPE.Version == "-" {
		if MatchCPE.Edition == "*" {
			return baseScore
		} else {
			flag, coe := VersionCompare(MatchCPE.Edition, cpe.Edition)
			if flag == 0 {
				return baseScore * coe
			} else {
				return 0
			}
		}
	} else {
		flag, coe := VersionCompare(MatchCPE.Version, cpe.Version)
		if flag == 0 {
			return baseScore * coe
		} else {
			return 0
		}
	}
}

//! 1是传入的CPE,2是match中的cpe

func (m CpeMatch) getVersionRule() []versionRule {
	var rules []versionRule
	if m.VersionEndExcluding != "" {
		rules = append(rules, func(cpe CPE) (bool, float64) {
			res, levelCoe := VersionCompare(cpe.Version, m.VersionEndExcluding)
			if res == -1 {
				return true, levelCoe
			} else {
				return false, levelCoe
			}
		})
	}
	if m.VersionEndIncluding != "" {
		rules = append(rules, func(cpe CPE) (bool, float64) {
			res, levelCoe := VersionCompare(cpe.Version, m.VersionEndIncluding)
			if res == -1 || res == 0 {
				return true, levelCoe
			} else {
				return false, levelCoe
			}
		})
	}
	if m.VersionStartExcluding != "" {
		rules = append(rules, func(cpe CPE) (bool, float64) {
			res, levelCoe := VersionCompare(cpe.Version, m.VersionStartExcluding)
			if res == 1 {
				return true, levelCoe
			} else {
				return false, levelCoe
			}
		})
	}
	if m.VersionStartIncluding != "" {
		rules = append(rules, func(cpe CPE) (bool, float64) {
			res, levelCoe := VersionCompare(cpe.Version, m.VersionStartIncluding)
			if res == 1 || res == 0 {
				return true, levelCoe
			} else {
				return false, levelCoe
			}
		})
	}

	return rules
}

func (m CpeMatch) getVersionMatchRule() map[string]string {
	var ruleMap = make(map[string]string)
	if m.VersionEndExcluding != "" {
		ruleMap["versionEndExcluding"] = m.VersionEndExcluding
	}

	if m.VersionEndIncluding != "" {
		ruleMap["versionEndIncluding"] = m.VersionEndIncluding
	}

	if m.VersionStartIncluding != "" {
		ruleMap["versionStartIncluding"] = m.VersionStartIncluding
	}

	if m.VersionStartExcluding != "" {
		ruleMap["versionStartExcluding"] = m.VersionStartExcluding
	}

	return ruleMap
}

func (m CpeMatch) getRegVersion() []string {
	var res []string
	var start, end string
	if m.VersionStartIncluding != "" {
		start = m.VersionStartIncluding
	} else if m.VersionStartExcluding != "" {
		start = m.VersionStartExcluding
	} else {
		start = "-"
	}

	if m.VersionEndIncluding != "" {
		end = m.VersionEndIncluding
	} else if m.VersionEndExcluding != "" {
		end = m.VersionEndExcluding
	} else {
		end = "-"
	}

	if start == "-" {
		endParts := strings.Split(end, ".")
		for i := len(endParts) - 1; i >= 0; i-- {
			var versionPart, numVersion []string
			if i != 0 {
				numVersion = endParts[0:i:i]
			}

			if i == len(endParts)-1 {
				versionPart = ToReg("0", endParts[i], true, true)
			} else {
				versionPart = ToReg("0", endParts[i], true, false)
			}
			for _, part := range versionPart {
				item := append(numVersion, part)
				for j := 0; j < len(endParts)-1-i; j++ {
					item = append(item, "[1-9]?[0-9]")
				}
				res = append(res, strings.Join(item, "."))
			}
		}
	} else if end == "-" {
		startParts := strings.Split(start, ".")
		for i := len(startParts) - 1; i >= 0; i-- {
			var versionPart, numVersion []string
			if i != 0 {
				numVersion = startParts[0:i:i]
			}

			if i == len(startParts)-1 {
				versionPart = ToReg(startParts[i], "99", true, true)
			} else {
				versionPart = ToReg(startParts[i], "99", false, true)
			}
			for _, part := range versionPart {
				item := append(numVersion, part)
				for j := 0; j < len(startParts)-1-i; j++ {
					item = append(item, "[1-9]?[0-9]")
				}
				res = append(res, strings.Join(item, "."))
			}
		}
	} else {
		startParts := strings.Split(start, ".")
		endParts := strings.Split(end, ".")

		if len(startParts) < len(endParts) {
			for i := 0; i < len(endParts)-len(startParts); i++ {
				startParts = append(startParts, "0")
			}
		} else if len(startParts) > len(endParts) {
			for i := 0; i < len(startParts)-len(endParts); i++ {
				startParts = append(endParts, "0")
			}
		}

		flag := true
		for i := 0; i < len(endParts); i++ {
			var versionPart, startNumVersion, endNumVersion []string
			if i != 0 {
				startNumVersion = startParts[0:i:i]
				endNumVersion = endParts[0:i:i]
			}
			if endParts[i] != startParts[i] && flag == true {
				flag = false
				endPartInt, err := strconv.Atoi(endParts[i])
				if err != nil {
					return nil
				}
				startPartInt, err := strconv.Atoi(startParts[i])
				if err != nil {
					return nil
				}
				if endPartInt > startPartInt+1 {
					versionPart = ToReg(startParts[i], endParts[i], false, false)
					for _, part := range versionPart {
						item := append(startNumVersion, part)
						for j := 0; j < len(startParts)-1-i; j++ {
							item = append(item, "[1-9]?[0-9]")
						}
						res = append(res, strings.Join(item, "."))
					}

				}
				continue
			}

			if !flag {
				if i == len(endParts)-1 {
					flag = true
				}
				versionPart = ToReg(startParts[i], "99", flag, true)
				for _, part := range versionPart {
					item := append(startNumVersion, part)
					for j := 0; j < len(startParts)-1-i; j++ {
						item = append(item, "[1-9]?[0-9]")
					}
					res = append(res, strings.Join(item, "."))
				}
				versionPart = ToReg("0", endParts[i], true, flag)
				for _, part := range versionPart {
					item := append(endNumVersion, part)
					for j := 0; j < len(endParts)-1-i; j++ {
						item = append(item, "[1-9]?[0-9]")
					}
					res = append(res, strings.Join(item, "."))
				}
			}
		}

	}

	return res
}

func ToReg(start, end string, startFlag, endFlag bool) []string {
	var res []string
	startInt, err := strconv.Atoi(start)
	if err != nil {
		return nil
	}
	endInt, err := strconv.Atoi(end)
	if err != nil {
		return nil
	}

	if startInt > endInt {
		log.Error("The incoming parameters do not conform to the specification: start > end ")
		return res
	}

	if !startFlag {
		startInt = startInt + 1
	}
	if !endFlag {
		endInt = endInt - 1
	}

	if startInt < 10 && endInt < 10 {
		res = append(res, fmt.Sprintf("[%d-%d]", startInt, endInt))
		return res
	}

	if startInt < 10 {
		endTen := endInt / 10
		res = append(res, fmt.Sprintf("[%d-%d]", startInt, 9))
		if endInt > 20 {
			res = append(res, fmt.Sprintf("[%d-%d][%d-%d]", 1, endTen-1, 0, 9))
		}
		res = append(res, fmt.Sprintf("%d[%d-%d]", endTen, 0, endInt%10))
		return res
	}
	startTen := startInt / 10
	endTen := endInt / 10

	if startTen == endTen {
		res = append(res, fmt.Sprintf("%d[%d-%d]", startTen, startTen%10, endTen%10))
		return res
	}

	if startTen+1 == endTen {
		res = append(res, fmt.Sprintf("%d[%d-%d]", startTen, startInt%10, 9))
		res = append(res, fmt.Sprintf("%d[%d-%d]", endTen, 0, endInt%10))
		return res
	}

	res = append(res, fmt.Sprintf("%d[%d-%d]", startTen, startInt%10, 9))
	res = append(res, fmt.Sprintf("[%d-%d][%d-%d]", startTen+1, endTen-1, 0, 9))
	res = append(res, fmt.Sprintf("%d[%d-%d]", endTen, 0, endInt%10))
	return res
}

func VersionCompare(v1, v2 string) (int, float64) {
	res, err := utils.VersionCompare(v1, v2)
	if err != nil {
		log.Errorf("compare version error : %v", err)
		return -2, 0
	}
	return res, 1

}

// IsNum 判断是否是数字
func IsNum(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func (n Nodes) Version() []string {

	rule, _ := regexp.Compile("^([\\w\\d]+[.-]+)*([\\w\\d]*)$")
	if len(n.Children) > 0 {
		var inside []string
		for _, insideNode := range n.Children {
			inside = append(inside, insideNode.Version()...)
		}
		return inside
	} else {
		var versions []string
		for _, m := range n.CpeMatch {
			if m.VersionEndExcluding != "" {
				if !rule.MatchString(m.VersionEndExcluding) {
					versions = append(versions, m.VersionEndExcluding)
				}
			}
			if m.VersionEndIncluding != "" {
				if !rule.MatchString(m.VersionEndIncluding) {
					versions = append(versions, m.VersionEndIncluding)
				}
			}
			if m.VersionStartExcluding != "" {
				if !rule.MatchString(m.VersionStartExcluding) {
					versions = append(versions, m.VersionStartExcluding)
				}
			}
			if m.VersionStartIncluding != "" {
				if !rule.MatchString(m.VersionStartIncluding) {
					versions = append(versions, m.VersionStartIncluding)
				}
			}
			cpe, err := ParseToCPE(m.Cpe23URI)
			if err != nil {
				log.Error(err)
				panic(err)
			}
			if cpe.Version != "*" && cpe.Version != "-" {
				if !rule.MatchString(cpe.Version) {
					versions = append(versions, cpe.Version)
				}
			}
		}

		return versions
	}
}

//todo  CWE中文分类映射
