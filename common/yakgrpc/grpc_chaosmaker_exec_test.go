package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

const testRule = `
alert http $EXTERNAL_NET any -> $HTTP_SERVERS any (msg:"ET SCAN Possible Scanning for Vulnerable JBoss"; flow:established,to_server; http.method; content:"POST"; http.uri; content:"/invoker/"; depth:9; content:"servlet/"; http.request_body; content:"org.jboss.invocation.MarshalledValue"; http.content_type; content:"application/x-java-serialized-object|3b|"; endswith; reference:url,blog.imperva.com/2015/12/zero-day-attack-strikes-again-java-zero-day-vulnerability-cve-2015-4852-tracked-by-imperva.html; classtype:web-application-attack; sid:2022240; rev:3; metadata:created_at 2015_12_09, updated_at 2020_06_09;)
alert http $EXTERNAL_NET any -> $HTTP_SERVERS any (msg:"ET SCAN Simple Slowloris Flooder"; flow:established,to_server; threshold:type limit, track by_src, count 1, seconds 300; http.method; content:"POST"; http.header; content:"Content-length|3a 20|5235|0d 0a|"; fast_pattern; http.header_names; content:!"User-Agent|0d 0a|"; reference:url,www.imperva.com/docs/HII_Denial_of_Service_Attacks-Trends_Techniques_and_Technologies.pdf; classtype:web-application-attack; sid:2016033; rev:5; metadata:created_at 2012_12_14, updated_at 2020_05_08;)
alert http any any -> any any  (msg: "Behinder3 PHP HTTP Request"; flow: established, to_server; content:".php"; http_uri;  pcre:"/[a-zA-Z0-9+/]{1000,}=/i"; flowbits:set,behinder3;noalert; classtype:shellcode-detect; sid: 3016017; rev: 1; metadata:created_at 2020_08_17,by al0ne;)
alert http any any -> any any (msg:"Exploit CVE-2020-17141 on Microsoft Exchange Server"; flow:to_server,established; content:"POST"; http_method; content:"/ews/Exchange.asmx"; startswith; http_uri; content:"<m:RouteComplaint "; http_client_body; content:"<m:Data>"; distance:0; http_client_body; base64_decode:bytes 300, offset 0, relative; base64_data; content:"<!DOCTYPE"; content:"SYSTEM"; distance:0; reference:cve,CVE-2020-17141; classtype:web-application-attack; sid:202017141; rev:1;)
alert tcp any any -> any 445 (msg: "ATTACK [PTsecurity] LSASS Remote Memory Corruption Attempt (MS16-137)"; flow: established, no_stream; content: "|FF|SMB|73 00 00 00 00|"; offset: 4; depth: 9; content: "|FF 00|"; offset: 37; depth: 2; content: "|01 00 00 00 00 00|"; offset: 45; depth: 6; content: "|00 00 00 00 D4 00 00 A0|"; distance: 2; within: 8; content: "|A1 84|"; distance: 2; within: 2; byte_test:1,!=,0xD1,0,relative; flowbits: set, CVE.2016-7237.Attempt; xbits:set,CVE.2016-7237.Attempt,track ip_dst,expire 15; reference: cve, 2016-7237; reference: url, g-laurent.blogspot.ru/2016/11/ms16-137-lsass-remote-memory-corruption.html; classtype: attempted-dos; reference: url, github.com/ptresearch/AttackDetection; sid: 10000532; rev: 2; )
`

func TestGRPCMUSTPASS_EXEC(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	stream, err := client.ExecuteChaosMakerRule(context.Background(), &ypb.ExecuteChaosMakerRuleRequest{
		Groups:                          nil,
		ExtraOverrideDestinationAddress: nil,
		Concurrent:                      0,
		TrafficDelayMinSeconds:          0,
		TrafficDelayMaxSeconds:          0,
		ExtraRepeat:                     0,
		GroupGapSeconds:                 0,
	})
	if err != nil {
		panic(err)
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}
		_ = msg
		// spew.Dump(msg)
	}
}
