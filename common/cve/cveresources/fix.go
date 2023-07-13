package cveresources

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
	"strings"
	"sync"
)

type ProductId struct {
	ProductName string
	Vendor      string
}

var CommonFix map[string]ProductId = map[string]ProductId{
	"httpd": {
		ProductName: "http_server",
		Vendor:      "apache",
	},
	"iis": {
		ProductName: "internet_information_server",
	},
}

func FixProductName(ProductName string, db *gorm.DB) ([]string, error) {
	ProductName = strings.ToLower(ProductName)
	var Products []ProductsTable
	resDb := db.Where("product = ?", ProductName).Find(&Products)
	if resDb.Error != nil {
		log.Errorf("query database failed: %s", resDb.Error)
	}
	if len(Products) > 0 {
		return []string{ProductName}, nil
	} else {
		rule, _ := regexp.Compile(`[a-zA-Z]{3,}(-[a-zA-Z]{3,})*`)
		ProductNameWords := rule.FindAllString(ProductName, -1)
		if len(ProductNameWords) > 0 {
			resDb = db.Where("product = ?", ProductNameWords[0]).Find(&Products)
			if resDb.Error != nil {
				log.Errorf("query database failed: %s", resDb.Error)
			}
			if len(Products) > 0 {
				return []string{ProductNameWords[0]}, nil
			}
		}
	}

	resDb = db.Find(&Products)
	if resDb.Error != nil {
		log.Errorf("query database failed: %s", resDb.Error)
	}

	fixName := make(chan string)
	wg := &sync.WaitGroup{}

	go func(p []ProductsTable) {
		//下发修复产品任务
		for _, product := range Products {
			wg.Add(1)
			go generalFix(wg, fixName, ProductName, product)
		}
		wg.Wait()
		fixName <- ""
	}(Products)

	var fixRes []string

	for {
		select {
		case result := <-fixName:
			fixRes = append(fixRes, result)
			if result == "" {
				close(fixName)
				if len(fixRes) > 1 {
					return fixRes, nil
				} else {
					return fixRes, utils.Errorf("fix name error: %s [%s]", "Unknown name", ProductName)
				}
			}

		}
	}
}

// 可能有的情况 lib5 -> lib 剔除不必要的数字以及其他符号
// lib-2.1.1 -> lib 版本和产品混合
func generalFix(wg *sync.WaitGroup, fixName chan string, ProductName string, Product ProductsTable) {
	/*
		1.简写 iis
		2.语义切割后模糊匹配(提取出纯字符的名字,尝试把)  lib
	*/

	//提取单词的规则
	//ruleForFuzz, err := regexp.Compile(`[a-zA-Z]{3,}`)
	//if err != nil {
	//	log.Errorf("Regular pattern compile failed: %s", err)
	//}

	ruleForAbbr, err := regexp.Compile("^([a-zA-Z\\d]+[_|-])+[a-zA-Z\\d]+$") //简写的正则
	if err != nil {
		log.Errorf("Regular pattern compile failed: %s", err)
	}

	//inputParts := ruleForFuzz.FindAllString(ProductName, -1)
	//itemParts := ruleForFuzz.FindAllString(Product.Product, -1)
	//if FuzzCheck(inputParts, itemParts) {
	//	fixName <- Product.Product
	//	return
	//}
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
