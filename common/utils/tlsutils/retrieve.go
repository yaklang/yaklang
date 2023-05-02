package tlsutils

import (
	"crypto/tls"
	"fmt"
	"strings"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"
)

type TLSInspectResult struct {
	Description     string
	Raw             []byte
	RelativeDomains []string
	RelativeEmail   []string
	RelativeAccount []string
	RelativeURIs    []string
}

func (t TLSInspectResult) String() string {
	return t.Description
}

func (t TLSInspectResult) Show() {
	fmt.Println(t.Description)
}

func TLSInspect(addr string) ([]*TLSInspectResult, error) {
	host, port, _ := utils.ParseStringToHostPort(addr)
	if port <= 0 {
		port = 443
	}
	if host == "" {
		host = addr
	}

	conn, err := utils.GetAutoProxyConn(utils.HostPort(host, port), utils.GetProxyFromEnv(), 5*time.Second)
	//conn, err := net.DialTimeout("tcp", utils.HostPort(host, port), 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var results []*TLSInspectResult
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName: host,
		VerifyConnection: func(state tls.ConnectionState) error {
			for _, cert := range state.PeerCertificates {
				if cert == nil {
					continue
				}
				var domains []string

				var urls []string
				for _, u := range cert.URIs {
					urls = append(urls, u.String())
					host, _, _ := utils.ParseStringToHostPort(u.Hostname())
					if host == "" {
						host = u.Hostname()
					}
					if host == "" {
						continue
					}
					domains = append(domains, host)
				}

				domains = append(domains, cert.ExcludedURIDomains...)
				domains = append(domains, cert.PermittedURIDomains...)

				var emails []string
				domains = append(domains, cert.DNSNames...)
				domains = append(domains, cert.PermittedDNSDomains...)
				domains = append(domains, cert.ExcludedDNSDomains...)
				emails = append(emails, cert.EmailAddresses...)
				emails = append(emails, cert.PermittedEmailAddresses...)
				emails = append(emails, cert.ExcludedEmailAddresses...)
				emails = utils.RemoveRepeatStringSlice(emails)
				var accounts []string
				for _, e := range emails {
					if strings.Contains(e, "@") {
						r := strings.Split(e, "@")
						domains = append(domains, r[1])
						accounts = append(accounts, r[0])
					} else {
						accounts = append(accounts, e)
					}
				}
				domains = utils.RemoveRepeatStringSlice(domains)
				text, err := CertificateText(cert)
				if err != nil {
					continue
				}

				result := TLSInspectResult{
					Description:     text,
					Raw:             cert.Raw,
					RelativeDomains: domains,
					RelativeEmail:   emails,
					RelativeAccount: utils.RemoveRepeatStringSlice(accounts),
					RelativeURIs:    utils.RemoveRepeatStringSlice(urls),
				}
				results = append(results, &result)
			}
			return nil
		},
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
		MaxVersion:         tls.VersionTLS13,
		KeyLogWriter:       nil,
	})
	err = tlsConn.HandshakeContext(utils.TimeoutContextSeconds(5))
	if err != nil {
		log.Errorf("TLSInspect: handshake error: %s", err)
	}
	return results, nil
}
