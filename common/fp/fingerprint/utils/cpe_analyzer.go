package utils

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/schema"
	"sync"
)

type CPEAnalyzer struct {
	WebsiteURLs  []string
	cpeOriginMap *sync.Map
	cpeMap       *sync.Map
}

func NewCPEAnalyzer(urls ...string) *CPEAnalyzer {
	return &CPEAnalyzer{
		WebsiteURLs:  urls,
		cpeMap:       new(sync.Map),
		cpeOriginMap: new(sync.Map),
	}
}

func (c *CPEAnalyzer) Feed(url string, cpes ...*schema.CPE) {
	for _, cpe := range cpes {
		c.cpeMap.Store(cpe.String(), cpe)

		if _, ok := c.cpeOriginMap.Load(cpe.String()); !ok {
			c.cpeOriginMap.Store(cpe.String(), url)
		}
	}
}

func (c *CPEAnalyzer) AvailableCPE() []*schema.CPE {
	var cpes []*schema.CPE
	c.cpeMap.Range(func(key, value interface{}) bool {
		cpes = append(cpes, value.(*schema.CPE))
		return true
	})
	return cpes
}

func (c *CPEAnalyzer) IsProductExisted(product string) bool {
	for _, cpe := range c.AvailableCPE() {
		if cpe.Product == product {
			return true
		}
	}
	return false
}

func (c *CPEAnalyzer) GetCPEsByProduct(product string) []*schema.CPE {
	var cpes []*schema.CPE
	for _, cpe := range c.AvailableCPE() {
		if cpe.Product == product {
			cpes = append(cpes, cpe)
		}
	}
	return cpes
}

func (c *CPEAnalyzer) GetVersionByProduct(product string) (string, error) {
	cpes := c.GetCPEsByProduct(product)
	if len(cpes) > 1 {
		return "", errors.Errorf("failed: %s", "multi version")
	} else if len(cpes) <= 0 {
		return "", errors.New("no product cpe")
	}

	return cpes[0].Version, nil
}

func (c *CPEAnalyzer) QueryOrigins(cpes ...*schema.CPE) map[*schema.CPE]string {
	results := map[*schema.CPE]string{}
	for _, value := range cpes {
		if raw, ok := c.cpeOriginMap.Load(value.String()); ok {
			origin := raw.(string)
			results[value] = origin
		}
	}
	return results
}
