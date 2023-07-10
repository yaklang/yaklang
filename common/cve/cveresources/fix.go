package cveresources

import (
	"context"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
	"strings"
	"sync"
)

// 可能有的情况 lib5 -> lib 剔除不必要的数字以及其他符号
// lib-2.1.1 -> lib 版本和产品混合
func generalFix(wg *sync.WaitGroup, fixName chan string, ProductName string, Product ProductsTable) {
	/*
		1.简写 iis
		2.语义切割后模糊匹配(提取出纯字符的名字,尝试把)  lib
	*/

	//提取单词
	wg.Add(1)
	ruleForFuzz, err := regexp.Compile(`[a-zA-Z]+`)
	if err != nil {
		log.Errorf("Regular pattern compile failed: %s", err)
	}

	ruleForAbbr, err := regexp.Compile("^([a-zA-Z\\d]+[_|-])+[a-zA-Z\\d]+$") //简写的正则
	if err != nil {
		log.Errorf("Regular pattern compile failed: %s", err)
	}

	inputParts := ruleForFuzz.FindAllString(ProductName, -1)
	itemParts := ruleForFuzz.FindAllString(Product.Product, -1)
	if FuzzCheck(inputParts, itemParts) {
		fixName <- Product.Product
		return
	}
	if ruleForAbbr.MatchString(ProductName) && (AbbrCheck(ProductName, Product, "-") || AbbrCheck(ProductName, Product, "_")) {
		fixName <- Product.Product
		return
	}
	wg.Done()
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
	ProductName = strings.ToLower(ProductName)
	var Products []ProductsTable
	resDb := db.Where("product = ?", ProductName).Find(&Products)
	if resDb.Error != nil {
		log.Errorf("query database failed: %s", resDb.Error)
	}
	if len(Products) > 0 {
		return ProductName, nil
	}

	resDb = db.Find(&Products)
	if resDb.Error != nil {
		log.Errorf("query database failed: %s", resDb.Error)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ProductCh := make(chan ProductsTable, 5)
	fixName := make(chan string)
	wg := &sync.WaitGroup{}

	go func(p []ProductsTable) {
		for _, product := range Products {
			select {
			case ProductCh <- product:
			case <-ctx.Done():
				close(ProductCh)
				return
			}
		}
		wg.Wait()
		close(ProductCh)
		fixName <- ""
	}(Products)

	go func(Name string) {
		for {
			select {
			case info := <-ProductCh:
				go generalFix(wg, fixName, Name, info)
			case <-ctx.Done():
				return
			}
		}
	}(ProductName)

	for {
		select {
		case result := <-fixName:
			fmt.Print(result)
			cancel()
			if result == "" {
				return result, utils.Errorf("fix name error: %s [%s]", "Unknown name", ProductName)
			}
			return result, nil
		}
	}
}
