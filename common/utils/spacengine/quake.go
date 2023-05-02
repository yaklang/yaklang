package spacengine

import (
	fmt "fmt"
	"strings"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
)

func QuakeQuery(key string, filter string, maxPage, maxRecord int) (chan *NetSpaceEngineResult, error) {
	// build fofa client
	quakeClient := utils.NewQuake360Client(key)

	ch := make(chan *NetSpaceEngineResult)

	go func() {
		defer close(ch)

		var nextFinished bool
		var count int
		var page = 0
		for range make([]int, maxPage) {
			page++
			if nextFinished {
				break
			}

			log.Infof("start to query quake api for: %v", filter)
			rsp, err := quakeClient.QueryNext(filter)
			if err != nil {
				log.Errorf("quake client query next failed: %s", err)
				break
			}

			for _, d := range rsp {
				log.Infof("quake fetch: %v", utils.HostPort(d.IP, d.Port))

				var serviceProvider = d.Location.Isp
				if d.Location.Isp != d.Org {
					serviceProvider = fmt.Sprintf("%v[%v]", d.Location.Isp, d.Org)
				}
				serviceProvider = ServiceProviderToChineseName(serviceProvider)

				var lat, lng float64
				if len(d.Location.Gps) == 2 {
					lat, lng = d.Location.Gps[0], d.Location.Gps[1]
				}

				var host = d.Hostname
				if host == "" {
					host = d.IP
				}
				var isTls bool
				if len(d.Service.TLSJarm.JarmAns) > 0 || d.Service.TLS.HandshakeLog.ServerHello.Version.Name != "" {
					isTls = true
				}

				var fps []string
				if d.OsName != "" {
					if d.OsVersion != "" {
						fps = append(fps, fmt.Sprintf("%v[%v]", d.OsName, d.OsVersion))
					} else {
						fps = append(fps, d.OsName)
					}
				}
				for _, value := range d.Components {
					fps = append(fps, value.ProductCatalog...)
					var names []string
					if value.ProductVendor != "" {
						names = append(names, value.ProductVendor)
					}
					if value.ProductNameEn != "" {
						names = append(names, value.ProductNameEn)
					}

					if len(names) > 0 {
						if value.Version != "" {
							fps = append(fps, fmt.Sprintf("%v[%v]", strings.Join(names, "_"), value.Version))
						} else {
							fps = append(fps, strings.Join(names, "_"))
						}
					}
				}
				if d.Service.Name != "" {
					fps = append(fps, d.Service.Name)
				}
				if d.Service.Product != "" {
					if d.Service.Version != "" {
						fps = append(fps, fmt.Sprintf("%v[%v]", d.Service.Product, d.Service.Version))
					} else {
						fps = append(fps, d.Service.Product)
					}
				}
				fps = utils.RemoveRepeatStringSlice(fps)
				ch <- &NetSpaceEngineResult{
					Addr:            utils.HostPort(d.IP, d.Port),
					FromEngine:      "quake",
					Latitude:        lat,
					Longitude:       lng,
					HtmlTitle:       d.Service.HTTP.Title,
					Domains:         d.Hostname,
					Province:        d.Location.ProvinceCn,
					Url:             "",
					ConfirmHttps:    isTls,
					Host:            host,
					City:            d.Location.CityCn,
					Asn:             fmt.Sprint(d.Asn),
					Location:        strings.Join([]string{d.Location.CountryCn, d.Location.ProvinceCn, d.Location.CityCn}, "/"),
					ServiceProvider: serviceProvider,
					FromFilter:      filter,
					Fingerprints:    strings.Join(fps, "/"),
					Banner:          utils.ParseStringToVisible(d.Service.Banner),
				}

				count++
				if maxRecord > 0 && count >= maxRecord {
					nextFinished = true
				}
			}
		}

	}()
	return ch, nil
}
