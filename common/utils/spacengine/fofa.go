package spacengine

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils/spacengine/base"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine/fofa"
	"github.com/yaklang/yaklang/common/utils/suspect"
)

func FofaQuery(email string, fofaKey string, filter string, maxPage, pageSize, maxRecord int, domains ...string) (chan *base.NetSpaceEngineResult, error) {
	var client *fofa.FofaClient
	if len(domains) > 0 && domains[0] != "" {
		client = fofa.NewClientEx(email, fofaKey, domains[0])
	} else {
		client = fofa.NewClient(email, fofaKey)
	}
	_, err := client.UserProfile()
	if err != nil {
		return nil, err
	}

	ch := make(chan *base.NetSpaceEngineResult)
	go func() {
		defer close(ch)

		var nextFinished bool
		var count int

		_ = count
		for page := 1; page <= maxPage; page++ {
			if nextFinished {
				break
			}
			result, err := client.Query(
				page,
				pageSize,
				filter,
			)
			if err != nil {
				log.Error(err)
				return
			}
			rResults := result.Get("results").Array()

			for _, r := range rResults {
				if nextFinished {
					break
				}
				rData := r.Array()
				if len(rData) < 7 {
					continue
				}

				ip := rData[2].String()
				port := rData[4].String()
				log.Infof("fofa fetch: %s",
					utils.HostPort(ip, port),
				)

				finalHost := ip
				domains := []string{}
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

				confirmHttps := false
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

				ch <- &base.NetSpaceEngineResult{
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
