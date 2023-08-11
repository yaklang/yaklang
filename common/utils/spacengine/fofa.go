package spacengine

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine/fofa"
	"github.com/yaklang/yaklang/common/utils/suspect"
)

func FofaQuery(email string, fofaKey string, filter string, maxPage, pageSize, maxRecord int) (chan *NetSpaceEngineResult, error) {
	// build fofa client
	client := fofa.NewFofaClient(email, fofaKey)
	userInfo, err := client.UserInfo()
	if err != nil {
		return nil, err
	}
	_ = userInfo

	ch := make(chan *NetSpaceEngineResult)
	go func() {
		defer close(ch)

		var nextFinished bool
		var count int

		_ = count
		for page := 1; page <= maxPage; page++ {
			if nextFinished {
				break
			}
			content, err := client.QueryAsJSON(
				page,
				pageSize,
				filter,
			)
			if err != nil {
				log.Error(err)
				return
			}
			result := gjson.ParseBytes(content)
			rResults := result.Get("results").Array()

			for _, r := range rResults {
				rData := r.Array()
				if len(rData) < 7 {
					continue
				}

				ip := rData[2].String()
				port := rData[4].String()
				log.Infof("fofa fetch: %s",
					utils.HostPort(ip, port),
				)

				var finalHost = ip
				var domains = []string{}
				urlIns := rData[0].String()
				if urlIns != "" {
					host, _, _ := utils.ParseStringToHostPort(urlIns)
					if host != "" {
						finalHost = host
						domains = append(domains, host)
					} else {
						domains = append(domains, urlIns)
					}
				}

				var confirmHttps = false
				if strings.HasPrefix(strings.ToLower(urlIns), "https://") {
					confirmHttps = true
				}

				if !suspect.IsFullURL(urlIns) {
					urlIns = ""
				}

				country := rData[5].String()
				city := rData[6].String()
				location := country
				if city != "" {
					location = fmt.Sprintf("%s %s", country, city)
				}

				ch <- &NetSpaceEngineResult{
					Addr:            utils.HostPort(ip, port),
					FromEngine:      "fofa",
					Latitude:        0,
					Longitude:       0,
					Url:             urlIns,
					ConfirmHttps:    confirmHttps,
					HtmlTitle:       rData[1].String(),
					Domains:         strings.Join(domains, "|"),
					City:            city,
					Asn:             "",
					Location:        location,
					ServiceProvider: "",
					FromFilter:      filter,
					Host:            finalHost,
				}

				count++
				if maxRecord > 0 && count >= maxRecord {
					nextFinished = true
					break
				}
			}
		}

	}()
	return ch, nil
}
