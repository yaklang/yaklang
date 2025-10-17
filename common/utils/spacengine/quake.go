package spacengine

import (
	fmt "fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/spacengine/base"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine/quake"
)

func QuakeQuery(key string, filter string, maxPage, maxRecord int, domains ...string) (chan *base.NetSpaceEngineResult, error) {
	return QuakeQueryWithConfig(key, filter, maxPage, maxRecord, nil, domains...)
}

func QuakeQueryWithConfig(key string, filter string, maxPage, maxRecord int, config *base.QueryConfig, domains ...string) (chan *base.NetSpaceEngineResult, error) {
	if config == nil {
		config = &base.QueryConfig{}
	}
	var client *quake.QuakeClient
	if len(domains) > 0 && domains[0] != "" {
		client = quake.NewClientEx(key, domains[0])
	} else {
		client = quake.NewClient(key)
	}

	ch := make(chan *base.NetSpaceEngineResult)

	go func() {
		defer close(ch)

		var nextFinished bool
		var count int
		for page := 0; page < maxPage; page++ {
			if nextFinished {
				break
			}

			log.Infof("start to query quake api for: %v", filter)
			size := 10
			if maxRecord-count < 10 {
				size = maxRecord - count
			}
			result, err := client.Query(filter, page*10, size)
			if err != nil {
				log.Errorf("quake client query next failed: %s", err)
				break
			}
			data := result.Get("data").Array()

			// 如果当前页没有数据，停止翻页
			if len(data) == 0 {
				nextFinished = true
				break
			}

			for _, d := range data {
				if nextFinished {
					break
				}
				ip, port := d.Get("ip").String(), d.Get("port").String()
				log.Infof("quake fetch: %v", utils.HostPort(ip, port))

				rService := d.Get("service")
				rLocation := d.Get("location")
				rComponents := d.Get("components").Array()

				isp, org := rLocation.Get("isp").String(), d.Get("org").String()
				serviceProvider := isp
				if isp != org {
					serviceProvider = fmt.Sprintf("%v[%v]", isp, org)
				}
				serviceProvider = ServiceProviderToChineseName(serviceProvider)

				var lat, lng float64
				gps := rLocation.Get("gps").Array()
				if len(gps) == 2 {
					lat, lng = gps[0].Float(), gps[1].Float()
				}

				host := d.Get("hostname").String()
				if host == "" {
					host = d.Get("ip").String()
				}
				domain := d.Get("domain").String()
				var isTls bool

				if len(rService.Get("tls-jarm.jarm_ans").Array()) > 0 || rService.Get("tls.handshake_log.server_hello.version.name").String() != "" {
					isTls = true
				}

				var fps []string
				osName, osVersion := d.Get("os_name").String(), d.Get("os_version").String()

				if osName != "" {
					if osVersion != "" {
						fps = append(fps, fmt.Sprintf("%v[%v]", osName, osVersion))
					} else {
						fps = append(fps, osName)
					}
				}

				for _, c := range rComponents {
					c.Get("product_catalog").ForEach(func(_, value gjson.Result) bool {
						fps = append(fps, value.String())
						return true
					})

					var names []string
					productVendor := c.Get("product_vendor").String()
					if productVendor != "" {
						names = append(names, productVendor)
					}
					product_name_en := c.Get("product_name_en").String()

					if product_name_en != "" {
						names = append(names, product_name_en)
					}

					if len(names) > 0 {
						version := c.Get("version").String()
						if version != "" {
							fps = append(fps, fmt.Sprintf("%v[%v]", strings.Join(names, "_"), version))
						} else {
							fps = append(fps, strings.Join(names, "_"))
						}
					}
				}

				serviceName := rService.Get("name").String()
				if serviceName != "" {
					fps = append(fps, serviceName)
				}
				serviceProduct := rService.Get("product").String()
				serviceVersion := rService.Get("version").String()

				if serviceProduct != "" {
					if serviceVersion != "" {
						fps = append(fps, fmt.Sprintf("%v[%v]", serviceProduct, serviceVersion))
					} else {
						fps = append(fps, serviceProduct)
					}
				}
				fps = utils.RemoveRepeatStringSlice(fps)
				country, province, city := rLocation.Get("country_cn").String(), rLocation.Get("province_cn").String(), rLocation.Get("city_cn").String()
				ch <- &base.NetSpaceEngineResult{
					Addr:            utils.HostPort(ip, port),
					FromEngine:      "quake",
					Latitude:        lat,
					Longitude:       lng,
					HtmlTitle:       rService.Get("http.title").String(),
					Domains:         domain,
					Province:        province,
					Url:             "",
					ConfirmHttps:    isTls,
					Host:            host,
					City:            city,
					Asn:             d.Get("asn").String(),
					Location:        strings.Join([]string{country, province, city}, "/"),
					ServiceProvider: serviceProvider,
					FromFilter:      filter,
					Fingerprints:    strings.Join(fps, "/"),
					Banner:          utils.ParseStringToVisible(rService.Get("banner").String()),
				}

				count++
				if maxRecord > 0 && count >= maxRecord {
					nextFinished = true
				}
			}

			// 在翻页之间应用随机延迟
			if !nextFinished && page < maxPage-1 {
				base.ApplyRandomDelay(config.RandomDelayRange)
			}
		}
	}()
	return ch, nil
}
