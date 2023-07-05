package cveresources

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
	"strings"
)

// 可能有的情况 lib5 -> lib 剔除不必要的数字以及其他符号
// lib-2.1.1 -> lib 版本和产品混合
func generalFix(ProductName string, Products []ProductsTable) (string, error) {
	/*
		1.简写 iis
		2.语义切割后模糊匹配(提取出纯字符的名字,尝试把)  lib
	*/

	//提取单词
	ruleForFuzz, err := regexp.Compile(`[a-zA-Z]+`)
	if err != nil {
		return "", utils.Errorf("Regular pattern compile failed: %s", err)
	}

	ruleForAbbr, err := regexp.Compile("^([a-zA-Z\\d]+[_|-])+[a-zA-Z\\d]+$") //简写的正则
	if err != nil {
		return "", utils.Errorf("Regular pattern compile failed: %s", err)
	}

	for _, productItem := range Products {
		inputParts := ruleForFuzz.FindAllString(ProductName, -1)
		itemParts := ruleForFuzz.FindAllString(productItem.Product, -1)
		if FuzzCheck(inputParts, itemParts) {
			return productItem.Product, nil
		}
		if ruleForAbbr.MatchString(ProductName) && (AbbrCheck(ProductName, productItem, "-") || AbbrCheck(ProductName, productItem, "_")) {
			return productItem.Product, nil
		}
	}

	return "", utils.Errorf("Unknown product name")
}

// FuzzCheck 模糊检查
func FuzzCheck(input []string, data []string) bool {
	for _, part := range input {
		for _, dataPart := range data {
			if part == dataPart {
				return true
			}
		}
	}
	return false
}

// AbbrCheck 简写检查
func AbbrCheck(name string, info ProductsTable, symbol string) bool {
	productArray := strings.Split(info.Product, symbol)

	var abbrProductName string
	for _, part := range productArray {
		if len(part) > 0 {
			abbrProductName = abbrProductName + part[0:1]
		}
	}

	return abbrProductName == name

}

//func similarityByPart(name string, info ProductsTable) bool {
//	nameArray := strings.Split(name, "_")
//	infoArray := strings.Split(info.Product, "_")
//
//	if len(nameArray) == len(infoArray) {
//		return false
//	}
//
//	if IsNum(infoArray[len(infoArray)-1]) {
//		return false
//	}
//
//	count := 0.0
//	for _, name := range nameArray {
//		for _, info := range infoArray {
//			if name == info {
//				count++
//			}
//		}
//	}
//
//	if count/float64(len(infoArray)) > 0.6 {
//		return true
//	}
//
//	return false
//}

func FixProductName(ProductName string, db *gorm.DB) (string, error) {
	var Products []ProductsTable
	resDb := db.Find(&Products)
	if resDb.Error != nil {
		log.Errorf("query database failed: %s", resDb.Error)
	}
	for _, product := range Products {
		if product.Product == ProductName {
			return ProductName, nil
		}
	}

	return generalFix(ProductName, Products)

}
