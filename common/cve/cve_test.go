package cve

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"yaklang.io/yaklang/common/cve/cvequeryops"
)

func TestQueryCWE(t *testing.T) {
	err := TranslatingCWE("/Users/v1ll4n/yakit-projects/openai-key.txt", 1, "")
	if err != nil {
		panic(err)
	}
	//escdb := consts.GetGormCVEDescriptionDatabase()
	//descdb.AutoMigrate(&cveresources.CWE{})
	//for cwe := range cveresources.YieldCWEs(consts.GetGormCVEDatabase().Model(&cveresources.CWE{}), context.Background()) {
	//	cwe := cwe
	//
	//	cwe, err := MakeOpenAITranslateCWE(cwe, getKey(), `http://127.0.0.1:7890`)
	//	if err != nil {
	//		panic(err)
	//	}
	//	err = cveresources.CreateOrUpdateCWE(descdb, cwe.IdStr, cwe)
	//	if err != nil {
	//		log.Error(err)
	//	}
	//}
}

func TestQuery(t *testing.T) {
	_, num := cvequeryops.Query("./date.db", cvequeryops.CVE("CVE-2017-0144"))
	if num != 1 {
		fmt.Println("option cve Fail")
	}

	resCve, num := cvequeryops.Query("./date.db", cvequeryops.CWE("CWE-89"))
	for _, cve := range resCve {
		if !cve.CWE("CWE-89") {
			fmt.Println("option CWE Fail ")
			break
		}
	}

	resCve, num = cvequeryops.Query("./date.db", cvequeryops.Product("php"))
	fmt.Println(len(resCve))

	resCve, num = cvequeryops.Query("./date.db", cvequeryops.Product("iis"))
	for _, cve := range resCve {
		if !strings.Contains(cve.Product, "internet_information_server") {
			fmt.Println("option product Fail ")
			break
		}
	}

	resCve, num = cvequeryops.Query("./date.db", cvequeryops.Vendor("apple"))
	for _, cve := range resCve {
		if !strings.Contains(cve.Vendor, "apple") {
			fmt.Println("option vendor Fail ")
			break
		}
	}

	resCve, num = cvequeryops.Query("./date.db", cvequeryops.Before(2022, 1, 3))
	for _, cve := range resCve {
		formatTime := "2022-01-03 00:00:00"
		testTime, err := time.Parse("2006-01-02 15:04:05", formatTime)
		if err != nil {
			panic("parse time error")
		}
		if !cve.PublishedDate.Before(testTime) {
			fmt.Println("option before Fail ")
			break
		}
	}

	resCve, num = cvequeryops.Query("./date.db", cvequeryops.After(2022, 1, 3))
	for _, cve := range resCve {
		formatTime := "2022-01-03 00:00:00"
		testTime, err := time.Parse("2006-01-02 15:04:05", formatTime)
		if err != nil {
			panic("parse time error")
		}
		if !cve.PublishedDate.After(testTime) {
			fmt.Println("option after Fail ")
			break
		}
	}
}

func TestFunc(t *testing.T) {
	cvequeryops.MakeCtScript("php", "./date.db", "php", "./")
}

func TestMigrate(t *testing.T) {
	_migrateTable()
}
