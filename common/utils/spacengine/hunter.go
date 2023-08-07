package spacengine

import (
	"fmt"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine/hunter"
)

func resultToSpacengineList(filter string, result *gjson.Result) []*NetSpaceEngineResult {
	rData := result.Get("data.arr").Array()
	var results = make([]*NetSpaceEngineResult, len(rData))
	for index, d := range rData {
		isTls := false
		dataMap := d.Map()
		webTitle := dataMap["web_title"].String()
		company := dataMap["company"].String()
		os := dataMap["os"].String()
		banner := dataMap["banner"].String()
		url := dataMap["url"].String()
		protocol, baseProtocol := dataMap["protocol"].String(), dataMap["base_protocol"].String()
		isp := dataMap["isp"].String()
		country, province, city := dataMap["country"].String(), dataMap["province"].String(), dataMap["city"].String()
		components := dataMap["component"].Array()

		if utils.MatchAnyOfSubString(strings.ToLower(fmt.Sprintf("%s%s", protocol, baseProtocol)), "https", "tls") {
			isTls = true
		}

		host, domain := dataMap["ip"].String(), dataMap["domain"].String()

		if domain != "" {
			host = domain
		}
		var locations []string

		if country != "" {
			locations = append(locations, country)
		}
		if province != "" {
			locations = append(locations, province)
		}
		if city != "" {
			locations = append(locations, city)
		}
		if company != "" {
			locations = append(locations, company)
		}
		var fps []string
		if os != "" {
			fps = append(fps, os)
		}
		for _, c := range components {
			version, name := c.Get("version").String(), c.Get("name").String()
			if version != "" {
				fps = append(fps, fmt.Sprintf("%v[%v]", name, version))
			} else {
				fps = append(fps, name)
			}
		}
		fps = utils.RemoveRepeatStringSlice(fps)

		results[index] = &NetSpaceEngineResult{
			Addr:            utils.HostPort(host, host),
			FromEngine:      "hunter",
			HtmlTitle:       webTitle,
			Domains:         domain,
			Province:        province,
			Url:             url,
			ConfirmHttps:    isTls,
			Host:            host,
			City:            city,
			Asn:             baseProtocol,
			Location:        strings.Join(locations, "/"),
			ServiceProvider: isp,
			FromFilter:      filter,
			Fingerprints:    strings.Join(fps, "/"),
			Banner:          utils.ParseStringToVisible(banner),
		}
	}

	return results
}

func HunterQuery(name, key, query string, maxPage, pageSize, maxRecord int) (chan *NetSpaceEngineResult, error) {
	ch := make(chan *NetSpaceEngineResult)
	go func() {
		defer close(ch)

		var nextFinished bool
		var count int
		for page := 1; page <= maxPage; page++ {
			if nextFinished {
				break
			}

			log.Infof("Start to query hunter for %v", query)
			result, err := hunter.HunterQuery(name, key, query, page, pageSize)
			if err != nil {
				log.Errorf("hunter client query next failed: %s", err)
				break
			}

			for _, record := range resultToSpacengineList(query, result) {
				ch <- record
				count++
				if maxRecord > 0 && count >= maxRecord {
					nextFinished = true
					break
				}
			}

			if !nextFinished {
				time.Sleep(3 * time.Second)
			}
		}
	}()
	return ch, nil
}
