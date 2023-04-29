package webfingerprint

import (
	"github.com/pkg/errors"
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

func (c *CPEAnalyzer) Feed(url string, cpes ...*CPE) {
	for _, cpe := range cpes {
		c.cpeMap.Store(cpe.String(), cpe)

		if _, ok := c.cpeOriginMap.Load(cpe.String()); !ok {
			c.cpeOriginMap.Store(cpe.String(), url)
		}
	}
}

func (c *CPEAnalyzer) AvailableCPE() []*CPE {
	var cpes []*CPE
	c.cpeMap.Range(func(key, value interface{}) bool {
		cpes = append(cpes, value.(*CPE))
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

func (c *CPEAnalyzer) GetCPEsByProduct(product string) []*CPE {
	var cpes []*CPE
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

func (c *CPEAnalyzer) QueryOrigins(cpes ...*CPE) map[*CPE]string {
	results := map[*CPE]string{}
	for _, value := range cpes {
		if raw, ok := c.cpeOriginMap.Load(value.String()); ok {
			origin := raw.(string)
			results[value] = origin
		}
	}
	return results
}
