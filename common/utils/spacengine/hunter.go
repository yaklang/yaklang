package spacengine

import (
	"fmt"
	"strings"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/spacengine/hunter"
)

func resultToSpacengineList(filter string, h *hunter.HunterResult) []*NetSpaceEngineResult {
	var results = make([]*NetSpaceEngineResult, len(h.Data.Arr))
	for index, result := range h.Data.Arr {
		isTls := false
		if utils.MatchAnyOfSubString(strings.ToLower(fmt.Sprint(result.Protocol)+fmt.Sprint(result.BaseProtocol)), "https", "tls") {
			isTls = true
		}

		var host = result.IP
		if result.Domain != "" {
			host = result.Domain
		}

		var locations []string
		if result.Country != "" {
			locations = append(locations, result.Country)
		}
		if result.Province != "" {
			locations = append(locations, result.Province)
		}
		if result.City != "" {
			locations = append(locations, result.City)
		}
		if result.Company != "" {
			locations = append(locations, result.Company)
		}
		var fps []string
		if result.Os != "" {
			fps = append(fps, result.Os)
		}
		for _, c := range result.Component {
			if c.Version != "" {
				fps = append(fps, fmt.Sprintf("%v[%v]", c.Name, c.Version))
			} else {
				fps = append(fps, c.Name)
			}
		}
		fps = utils.RemoveRepeatStringSlice(fps)

		results[index] = &NetSpaceEngineResult{
			Addr:            utils.HostPort(result.IP, result.Port),
			FromEngine:      "hunter",
			HtmlTitle:       result.WebTitle,
			Domains:         result.Domain,
			Province:        result.Province,
			Url:             result.URL,
			ConfirmHttps:    isTls,
			Host:            host,
			City:            result.City,
			Asn:             result.BaseProtocol,
			Location:        strings.Join(locations, "/"),
			ServiceProvider: result.Isp,
			FromFilter:      filter,
			Fingerprints:    strings.Join(fps, "/"),
			Banner:          utils.ParseStringToVisible(result.Banner),
		}
	}

	return results
}

func HunterQuery(name, key, query string, maxPage, maxRecord int) (chan *NetSpaceEngineResult, error) {
	ch := make(chan *NetSpaceEngineResult)
	go func() {
		defer close(ch)

		var nextFinished bool
		var count int
		var page int
		for range make([]int, maxPage) {
			page++
			if nextFinished {
				break
			}

			log.Infof("Start to query quake hunter for %v", query)
			result, err := hunter.HunterQuery(name, key, query, page, 10)
			if err != nil {
				log.Errorf("hunter client query next failed: %s", err)
				break
			}

			for _, record := range resultToSpacengineList(query, result) {
				ch <- record
				count++
				if maxRecord > 0 && count >= maxRecord {
					nextFinished = true
				}
			}

			time.Sleep(3 * time.Second)
		}
	}()
	return ch, nil
}
