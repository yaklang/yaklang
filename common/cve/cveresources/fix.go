package cveresources

import (
	"github.com/jinzhu/gorm"
	"regexp"
	"strings"
	"yaklang.io/yaklang/common/log"
)

func generalFix(ProductName string, Products []ProductsTable) string {

	rule, err := regexp.Compile("^([a-zA-Z1\\d]+_)+[a-zA-Z\\d]+$")
	if err != nil {
		log.Error(err)
	}

	for _, productItem := range Products {
		if ExtendCheck(ProductName, productItem) {
			return productItem.Product
		}

		if rule.MatchString(productItem.Product) && AbbrCheck(ProductName, productItem) {
			return productItem.Product
		}

		if rule.MatchString(ProductName) && strings.Contains(ProductName, productItem.Vendor) && ReduceCheck(ProductName, productItem) {
			return productItem.Product
		}

		if rule.MatchString(ProductName) && rule.MatchString(productItem.Product) && similarityByPart(ProductName, productItem) {
			return productItem.Product
		}
	}

	return ""
}

func AbbrCheck(name string, info ProductsTable) bool {
	productArray := strings.Split(info.Product, "_")

	var abbrProductName string
	for _, part := range productArray {
		abbrProductName = abbrProductName + part[0:1]
	}

	return abbrProductName == name
}

func ExtendCheck(name string, info ProductsTable) bool {
	extendProductName := name + "_" + info.Vendor
	if extendProductName == info.Product {
		return true
	}

	extendProductName = info.Vendor + "_" + name
	if extendProductName == info.Product {
		return true
	}

	return false
}

func ReduceCheck(name string, info ProductsTable) bool {
	nameArray := strings.Split(name, "_")
	for i := 0; i < len(nameArray); i++ {
		if nameArray[i] == info.Vendor {
			if strings.Join(append(nameArray[0:i], nameArray[i+1:]...), "") == info.Product {
				return true
			}
		}
	}
	return false
}

func similarityByPart(name string, info ProductsTable) bool {
	nameArray := strings.Split(name, "_")
	infoArray := strings.Split(info.Product, "_")

	if len(nameArray) == len(infoArray) {
		return false
	}

	if IsNum(infoArray[len(infoArray)-1]) {
		return false
	}

	count := 0.0
	for _, name := range nameArray {
		for _, info := range infoArray {
			if name == info {
				count++
			}
		}
	}

	if count/float64(len(infoArray)) > 0.6 {
		return true
	}

	return false
}

func FixProductName(ProductName string, db *gorm.DB) string {
	var Products []ProductsTable
	resDb := db.Find(&Products)
	if resDb.Error != nil {
		log.Error(resDb.Error)
	}
	for _, product := range Products {
		if product.Product == ProductName {
			return ProductName
		}
	}

	return generalFix(ProductName, Products)

}
