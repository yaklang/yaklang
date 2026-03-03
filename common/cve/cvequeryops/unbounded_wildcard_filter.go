package cvequeryops

import "github.com/yaklang/yaklang/common/cve/cveresources"

func shouldSkipUnboundedWildcardOnly(config cveresources.Configurations, queryCPE []cveresources.CPE) bool {
	queryProducts := queryProductsFromCPE(queryCPE)
	if len(queryProducts) == 0 {
		return false
	}

	hasWildcardOnly := false
	stack := append([]cveresources.Nodes(nil), config.Nodes...)
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if len(node.Children) > 0 {
			stack = append(stack, node.Children...)
		}

		for _, match := range node.CpeMatch {
			if !match.Vulnerable {
				continue
			}

			matchCPE, err := cveresources.ParseToCPE(match.Cpe23URI)
			if err != nil {
				continue
			}
			if _, ok := queryProducts[matchCPE.Product]; !ok {
				continue
			}

			if isUnboundedWildcardMatch(match, matchCPE) {
				hasWildcardOnly = true
				continue
			}

			return false
		}
	}

	return hasWildcardOnly
}

func queryProductsFromCPE(queryCPE []cveresources.CPE) map[string]struct{} {
	products := make(map[string]struct{}, len(queryCPE))
	for _, cpe := range queryCPE {
		if cpe.Product == "" || cpe.Product == "*" {
			continue
		}
		products[cpe.Product] = struct{}{}
	}
	return products
}

func isUnboundedWildcardMatch(match cveresources.CpeMatch, matchCPE *cveresources.CPE) bool {
	if matchCPE.Version != "*" {
		return false
	}

	return match.VersionStartIncluding == "" &&
		match.VersionStartExcluding == "" &&
		match.VersionEndIncluding == "" &&
		match.VersionEndExcluding == ""
}
