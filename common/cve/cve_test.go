package cve

import (
	"embed"
	"io"
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/cve/cvequeryops"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/log"
)

//go:embed CIDate.db
var DbFs embed.FS

func TestMigrate(t *testing.T) {
	_migrateTable()
}

type productWithVersion struct {
	name    string
	version string
	target  string
}

func TestQueryCVEWithFixName(t *testing.T) {
	data := []productWithVersion{
		{
			name:    "httpd", // 硬编码修复测试
			version: "2.4.49",
			target:  "CVE-2021-42013",
		},
		{
			name:    "apt2", // 产品名冗杂修复
			version: "0.7.5",
			target:  "CVE-2009-1358",
		},
		{
			name:    "python3-e", // 产品名冗杂修复
			version: "2.2",
			target:  "CVE-2006-1542",
		},
		{
			name:    "linux-2019",
			version: "9.0",
			target:  "CVE-2003-0780",
		},
	}

	// 读 embed 文件
	DbFp, err := DbFs.Open("CIDate.db")
	if err != nil {
		log.Errorf("%v", err)
	}
	defer DbFp.Close()

	// 写到临时目录
	tempFp, err := os.CreateTemp("", "Date.db")
	if err != nil {
		log.Errorf("%v", err)
	}
	defer tempFp.Close()

	_, err = io.Copy(tempFp, DbFp)
	if err != nil {
		log.Errorf("%v", err)
	}

	M := cveresources.GetManager(tempFp.Name(), true)
	for _, datum := range data {
		cve := cvequeryops.QueryCVEYields(M.DB, cvequeryops.ProductWithVersion(datum.name, datum.version))
		count := 0
		for {
			flag := false
			select {
			case item, ok := <-cve:
				if !ok {
					flag = true
					break
				}
				if item.CVE != datum.target {
					panic("Mismatch: Redundant data: " + datum.name)
				} else {
					count++
				}
			}
			if flag {
				break
			}
		}
		if count < 1 {
			panic("Mismatch: Lack of data: " + datum.name)
		}
	}
}

func TestQueryCVEWithCVE2Configurations(t *testing.T) {
	tempFp, err := os.CreateTemp("", "cve2-config-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	defer tempFp.Close()
	defer os.Remove(tempFp.Name())

	M := cveresources.GetManager(tempFp.Name(), true)
	if err := M.DB.Save(cveresources.ProductsTable{
		Product: "openssh",
		Vendor:  "openbsd",
	}).Error; err != nil {
		t.Fatalf("insert product index: %v", err)
	}
	if err := M.DB.Create(&cveresources.CVE{
		CVE:      "CVE-2099-0001",
		Vendor:   "openbsd",
		Product:  "openssh",
		Severity: "HIGH",
		CPEConfigurations: cveresources.MarshalCheck([]cveresources.CVE2Configuration{
			{
				Nodes: []cveresources.CVE2Node{
					{
						Operator: "OR",
						CpeMatch: []cveresources.CVE2CpeMatch{
							{
								Vulnerable:            true,
								Criteria:              "cpe:2.3:a:openbsd:openssh:*:*:*:*:*:*:*:*",
								VersionStartIncluding: "8.9",
								VersionEndExcluding:   "9.6",
							},
						},
					},
				},
			},
		}),
	}).Error; err != nil {
		t.Fatalf("insert cve: %v", err)
	}

	cves := cvequeryops.QueryCVEYields(M.DB, cvequeryops.ProductWithVersion("openssh", "9.2p1"))
	var matched []string
	for item := range cves {
		matched = append(matched, item.CVE)
	}

	if len(matched) != 1 || matched[0] != "CVE-2099-0001" {
		t.Fatalf("expected CVE-2099-0001, got %#v", matched)
	}
}

func TestCVE2ToCVEIndexesNestedProducts(t *testing.T) {
	tempFp, err := os.CreateTemp("", "cve2-products-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	defer tempFp.Close()
	defer os.Remove(tempFp.Name())

	M := cveresources.GetManager(tempFp.Name(), true)
	record := &cveresources.CVEVulnerability{
		Cve: cveresources.CVE2Data{
			ID:           "CVE-2099-0002",
			Published:    "2026-05-26T00:00:00.000",
			LastModified: "2026-05-26T00:00:00.000",
			Descriptions: []cveresources.CVE2Description{
				{Lang: "en", Value: "nested openssh test"},
			},
			Configurations: []cveresources.CVE2Configuration{
				{
					Nodes: []cveresources.CVE2Node{
						{
							Operator: "OR",
							Children: []cveresources.CVE2Node{
								{
									Operator: "OR",
									CpeMatch: []cveresources.CVE2CpeMatch{
										{
											Vulnerable: true,
											Criteria:   "cpe:2.3:a:openbsd:openssh:*:*:*:*:*:*:*:*",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	cve, err := record.ToCVE(M.DB)
	if err != nil {
		t.Fatalf("convert cve: %v", err)
	}

	if cve.Product != "openssh" || cve.Vendor != "openbsd" {
		t.Fatalf("expected openbsd/openssh, got %s/%s", cve.Vendor, cve.Product)
	}

	var product cveresources.ProductsTable
	if err := M.DB.Where("product = ?", "openssh").First(&product).Error; err != nil {
		t.Fatalf("expected openssh product index: %v", err)
	}
	if product.Vendor != "openbsd" {
		t.Fatalf("expected openbsd vendor index, got %s", product.Vendor)
	}
}

func TestFixProductNameUsesCVEProductsWhenProductIndexIsEmpty(t *testing.T) {
	tempFp, err := os.CreateTemp("", "cve-product-fallback-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	defer tempFp.Close()
	defer os.Remove(tempFp.Name())

	M := cveresources.GetManager(tempFp.Name(), true)
	if err := M.DB.Create(&cveresources.CVE{
		CVE:     "CVE-2099-0003",
		Vendor:  "openbsd",
		Product: "openssh",
	}).Error; err != nil {
		t.Fatalf("insert cve: %v", err)
	}

	products, err := cveresources.FixProductName("openssh", M.DB)
	if err != nil {
		t.Fatalf("expected product lookup to use cves.product: %v", err)
	}
	if len(products) != 1 || products[0] != "openssh" {
		t.Fatalf("expected openssh, got %#v", products)
	}
}
