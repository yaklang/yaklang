package spacengine

import (
	"fmt"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine/zoomeye"
)

func zoomeyeResultToSpacengineList(filter string, result *gjson.Result) []*NetSpaceEngineResult {
	rData := result.Get("matches").Array()

	var results = make([]*NetSpaceEngineResult, len(rData))
	for index, d := range rData {
		dataMap := d.Map()
		rGeoInfo := dataMap["geoinfo"]
		rPortInfo := dataMap["portinfo"]
		rProtocol := dataMap["protocol"]

		isTls := false
		protocol := ""
		host, port := dataMap["ip"].String(), rPortInfo.Get("port").Int()
		asn, isp := rGeoInfo.Get("asn").String(), rGeoInfo.Get("isp").String()
		continent, country, city := rGeoInfo.Get("continent.names.zh-CN").String(), rGeoInfo.Get("country.names.zh-CN").String(), rGeoInfo.Get("city.names.zh-CN").String()
		os := rPortInfo.Get("os").String()
		app, version := rPortInfo.Get("app").String(), rPortInfo.Get("version").String()
		latitule, longitude := rGeoInfo.Get("location.lat").Float(), rGeoInfo.Get("location.lon").Float()
		banner := rPortInfo.Get("banner").String()

		if rProtocol.Exists() {
			protocol = dataMap["protocol"].Str
		}

		if utils.MatchAnyOfSubString(strings.ToLower(protocol), "https", "tls") {
			isTls = true
		}

		var locations []string
		if continent != "" {
			locations = append(locations, continent)
		}
		if country != "" {
			locations = append(locations, country)
		}
		if city != "" {
			locations = append(locations, city)
		}

		var fps []string
		if os != "" {
			fps = append(fps, os)
		}

		if version != "" {
			fps = append(fps, fmt.Sprintf("%v[%v]", app, version))
		} else {
			fps = append(fps, app)
		}
		fps = utils.RemoveRepeatStringSlice(fps)

		results[index] = &NetSpaceEngineResult{
			Addr:            utils.HostPort(host, port),
			FromEngine:      "zoomeye",
			Latitude:        latitule,
			Longitude:       longitude,
			ConfirmHttps:    isTls,
			Host:            host,
			City:            city,
			Asn:             asn,
			Location:        strings.Join(locations, "/"),
			ServiceProvider: isp,
			FromFilter:      filter,
			Fingerprints:    strings.Join(fps, "/"),
			Banner:          utils.ParseStringToVisible(banner),
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
		for page := 1; page <= maxPage; page++ {
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

			if !nextFinished {
				time.Sleep(3 * time.Second)
			}
		}
	}()
	return ch, nil
}
