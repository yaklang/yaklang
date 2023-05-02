package spacengine

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine/zoomeye"
)

func zoomeyeResultToSpacengineList(filter string, z *zoomeye.ZoomeyeResult) []*NetSpaceEngineResult {
	var results = make([]*NetSpaceEngineResult, len(z.Matches))
	for index, result := range z.Matches {
		isTls := false
		if utils.MatchAnyOfSubString(strings.ToLower(fmt.Sprint(result.Protocol)+fmt.Sprint(result.Protocol.Application)), "https", "tls") {
			isTls = true
		}

		var host = result.IP

		var locations []string
		if result.Geoinfo.Continent.Names.ZhCN != "" {
			locations = append(locations, result.Geoinfo.Continent.Names.ZhCN)
		}
		if result.Geoinfo.Country.Names.ZhCN != "" {
			locations = append(locations, result.Geoinfo.Country.Names.ZhCN)
		}
		if result.Geoinfo.City.Names.ZhCN != "" {
			locations = append(locations, result.Geoinfo.City.Names.ZhCN)
		}

		var fps []string
		if result.Portinfo.Os != "" {
			fps = append(fps, result.Portinfo.Os)
		}

		if result.Portinfo.Version != "" {
			fps = append(fps, fmt.Sprintf("%v[%v]", result.Portinfo.App, result.Portinfo.Version))
		} else {
			fps = append(fps, result.Portinfo.App)
		}
		fps = utils.RemoveRepeatStringSlice(fps)

		var latitule, longitude float64
		latitule, _ = strconv.ParseFloat(result.Geoinfo.Location.Lat, 64)
		longitude, _ = strconv.ParseFloat(result.Geoinfo.Location.Lon, 64)

		results[index] = &NetSpaceEngineResult{
			Addr:            utils.HostPort(result.IP, result.Portinfo.Port),
			FromEngine:      "zoomeye",
			Latitude:        latitule,
			Longitude:       longitude,
			ConfirmHttps:    isTls,
			Host:            host,
			City:            result.Geoinfo.City.Names.ZhCN,
			Asn:             result.Geoinfo.Asn,
			Location:        strings.Join(locations, "/"),
			ServiceProvider: result.Geoinfo.Isp,
			FromFilter:      filter,
			Fingerprints:    strings.Join(fps, "/"),
			Banner:          utils.ParseStringToVisible(result.Portinfo.Banner),
		}
	}

	return results
}

func ZoomeyeQuery(key, query string, maxPage, maxRecord int) (chan *NetSpaceEngineResult, error) {
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

			log.Infof("Start to query zoomeye for %v", query)
			result, err := zoomeye.ZoomeyeQuery(key, query, page)
			if err != nil {
				log.Errorf("zoomeye client query next failed: %s", err)
				break
			}

			for _, record := range zoomeyeResultToSpacengineList(query, result) {
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
