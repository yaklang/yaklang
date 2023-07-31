package yakdns

/*
DoH is a DNS over HTTPS resolver

It is used to resolve a domain name to an IP address via http(s) / tls
default situation:

1. http://1.1.1.1/dns-query to resolve domain name to ip address
2. https://dns.google/dns-query to resolve domain name to ip address
3. https://cloudflare-dns.com/dns-query to resolve domain name to ip address
4. https://dns.alidns.com/dns-query to resolve domain name to ip address

actually http://ip/dns-query have json api.
but... only few api source can be used.
*/
