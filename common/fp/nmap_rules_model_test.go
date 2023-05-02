package fp

import (
	"testing"
	log "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func TestParseNmapProbe(t *testing.T) {
	text := `
Probe TCP WMSRequest q|\x01\0\0\xfd\xce\xfa\x0b\xb0\xa0\0\0\0MMS\x14\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\x12\0\0\0\x01\0\x03\0\xf0\xf0\xf0\xf0\x0b\0\x04\0\x1c\0\x03\0N\0S\0P\0l\0a\0y\0e\0r\0/\09\0.\00\0.\00\0.\02\09\08\00\0;\0 \0{\00\00\00\00\0A\0A\00\00\0-\00\0A\00\00\0-\00\00\0a\00\0-\0A\0A\00\0A\0-\00\00\00\00\0A\00\0A\0A\00\0A\0A\00\0}\0\0\0\xe0\x6d\xdf\x5f|
Probe TCP oracle-tns q|\0Z\0\0\x01\0\0\0\x016\x01,\0\0\x08\0\x7F\xFF\x7F\x08\0\0\0\x01\0 \0:\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\04\xE6\0\0\0\x01\0\0\0\0\0\0\0\0(CONNECT_DATA=(COMMAND=version))|
`
	results, err := ParseNmapProbe(text)
	if err != nil {
		t.Logf("parse nmap probe error: %s", err)
		t.Fail()
		return
	}

	if len(results) != 2 {
		t.Logf("parse 2 probes but %v", len(results))
		t.Fail()
		return
	}

	for _, result := range results {
		log.Infof("parsed %s probe[%s]: %#v", result.Proto, result.Name, result.Payload)
	}
}

func TestExtractBlockFromMatch(t *testing.T) {
	results := ExtractBlockFromMatch(`
m|^\x01\0\0.\xce\xfa\x0b\xb0.\0\0\0MMS .\0{7}.{9}\0\0\0\x01\0\x04\0\0\0\0\0\xf0\xf0\xf0\xf0\x0b\0\x04\0\x1c\0\x03\0\0\0\0\0\0\0\xf0\?\x01\0\0\0\x01\0\0\0\0\x80\0\0...\0.\0\0\0\0\0\0\0\0\0\0\0.\0\0\x00(\d)\0\.\x00(\d)\0\.\x00(\d)\0\.\x00(\d)\x00(\d)\x00(\d)\x00(\d)\0\0\0|s p/Microsoft Windows Media Services/ v/$1.$2.$3.$4$5$6$7/ o/Windows/ cpe:/a:microsoft:windows_media_services:$1.$2.$3.$4$5$6$7/a cpe:/o:microsoft:windows/a`)
	if len(results) != 6 {
		t.Logf("parsed result: %#v", results)
		t.Fail()
	}
}

func TestParseNmapMatch(t *testing.T) {
	text := `match wms m|^\x01\0\0.\xce\xfa\x0b\xb0.\0\0\0MMS .\0{7}.{9}\0\0\0\x01\0\x04\0\0\0\0\0\xf0\xf0\xf0\xf0\x0b\0\x04\0\x1c\0\x03\0\0\0\0\0\0\0\xf0\?\x01\0\0\0\x01\0\0\0\0\x80\0\0...\0.\0\0\0\0\0\0\0\0\0\0\0.\0\0\x00(\d)\0\.\x00(\d)\0\.\x00(\d)\0\.\x00(\d)\x00(\d)\x00(\d)\x00(\d)\0\0\0|s p/Microsoft Windows Media Services/ v/$1.$2.$3.$4$5$6$7/ o/Windows/ cpe:/a:microsoft:windows_media_services:$1.$2.$3.$4$5$6$7/a cpe:/o:microsoft:windows/a cpe:/a:asdfasdf:adsasdf/
match wms m|^\x01\0\0.\xce\xfa\x0b\xb0.\0\0\0MMS .\0{7}.{9}\0\0\0\x01\0\x04\0\0\0\0\0\xf0\xf0\xf0\xf0\x0b\0\x04\0\x1c\0\x03\0\0\0\0\0\0\0\xf0\?\x01\0\0\0\x01\0\0\0\0\x80\0\0...\0.\0\0\0\0\0\0\0\0\0\0\0.\0\0\x00(\d)\0\.\x00(\d)\x00(\d)\0\.\x00(\d)\x00(\d)\0\.\x00(\d)\x00(\d)\x00(\d)\x00(\d)\0\0\0|s p/Microsoft Windows Media Services/ v/$1.$2$3.$4$5.$6$7$8$9/ o/Windows/ cpe:/a:microsoft:windows_media_services:$1.$2$3.$4$5.$6$7$8$9/a cpe:/o:microsoft:windows/a
match http m|^HTTP/1\.0 200 OK\r\n(?:[^\r\n]+\r\n)*?Server: FlashCom/([\w._-]+)\r\n.*<html><head><title>Wowza Streaming Engine ([^<]*)</title></head>|s p/Adobe Flash Media Server/ v/$1/ i/Wowza Streaming Engine $2/ cpe:/a:adobe:flash_media_server:$1/ cpe:/a:wowza:wowza_media_server:$SUBST(2," ","_")/
match telnet m|^\xff\xfd\x01\xff\xfd\x1f\xff\xfd!\xff\xfb\x01\xff\xfb\x03\r\r\nMICROSENS G6 Micro-Switch\r\n\rMICROSENS-G6-MAC-([0-9A-TaskFunc-]{17}) login: | p/BusyBox telnetd/ v/1.00-pre7 - 1.14.0/ i/Microsens G6 switch; MAC: $1/ d/switch/ cpe:/a:busybox:busybox:1.00-pre7 - 1.14.0/a cpe:/h:microsens:g6/
match telnet m|^\xff\xfd\x01\xff\xfd\x1f\xff\xfd!\xff\xfb\x01\xff\xfb\x03\r\r\n\r\nPlease login: | p/BusyBox telnetd/ v/1.00-pre7 - 1.14.0/ i/Ruckus VF7811 WAP/ d/WAP/ cpe:/a:busybox:busybox:1.00-pre7 - 1.14.0/a cpe:/h:ruckus:vf7811/a
softmatch telnet m|^\xff\xfd\x01\xff\xfd\x1f\xff\xfd!\xff\xfb\x01\xff\xfb\x03[^\xff]| p/BusyBox telnetd/ v/1.00-pre7 - 1.14.0/ cpe:/a:busybox:busybox:1.00-pre7 - 1.14.0/a
match http m|^HTTP/1\.0 200 OK\r\n(?:[^\r\n]+\r\n)*?Server: FlashCom/([\w._-]+)\r\n.*<html><head><title>Wowza Media Server ([^<]*)</title></head>|s p/Adobe Flash Media Server/ v/$1/ i/Wowza Media Server $2/ cpe:/a:adobe:flash_media_server:$1/ cpe:/a:wowza:wowza_media_server:$SUBST(2," ","_")/
match http m|^HTTP/1\.0 200 OK\r\n(?:[^\r\n]+\r\n)*?Server: FlashCom/([\w._-]+)\r\n.*<html><head><title>Wowza Streaming Engine ([^<]*)</title></head>|s p/Adobe Flash Media Server/ v/$1/ i/Wowza Streaming Engine $2/ cpe:/a:adobe:flash_media_server:$1/ cpe:/a:wowza:wowza_media_server:$SUBST(2," ","_")/
`
	results, err := ParseNmapMatch(text)
	if err != nil {
		t.Logf("parse nmap match failed: %s", err)
		t.Fail()
		return
	}

	if len(results) != 8 {
		t.Logf("parse 2 match but %v", len(results))
		t.Fail()
		return
	}

	//for _, result := range results {
	//	_, _ = pp.Println(result)
	//	//t.Logf("parsed %s match[%s]: %#v", result.ServiceName, result.Version, result.MatchRule)
	//}
}

func TestParseNmapServiceProbesTxt(t *testing.T) {
	text := `
# The Exclude directive takes a comma separated list of ports.
# The format is exactly the same as the -p switch.
Exclude T:9100-9107

# This is the NULL probe that just compares any banners given to us
##############################NEXT PROBE##############################
Probe TCP NULL q||
# Wait for at least 5 seconds for data.  Otherwise an Nmap default is used.
totalwaitms 5000
# Windows 2003
match ftp m/^220[ -]Microsoft FTP Service\r\n/ p/Microsoft ftpd/
match ftp m/^220 ProFTPD (\d\S+) Server/ p/ProFTPD/ v/$1/
softmatch ftp m/^220 [-.\w ]+ftp.*\r\n$/i
match ident m|^flock\(\) on closed filehandle .*midentd| p/midentd/ i/broken/
match imap m|^\* OK Welcome to Binc IMAP v(\d[-.\w]+)| p/Binc IMAPd/ v$1/
softmatch imap m/^\* OK [-.\w ]+imap[-.\w ]+\r\n$/i
match lucent-fwadm m|^0001;2$| p/Lucent Secure Management Server/
match meetingmaker m/^\xc1,$/ p/Meeting Maker calendaring/
# lopster 1.2.0.1 on Linux 1.1
match napster m|^1$| p/Lopster Napster P2P client/

Probe UDP Help q|help\r\n\r\n|
rarity 3
ports 7,13,37
match chargen m|@ABCDEFGHIJKLMNOPQRSTUVWXYZ|
match echo m|^help\r\n\r\n$|
`
	probes, matches, _ := ParseNmapServiceProbesTxt(text)
	if len(probes) != 2 {
		t.Log("probes parse failed")
		t.Fail()
		return
	}

	if len(matches) != 11 {
		t.Log("matches parse failed")
		t.Fail()
		return
	}
}

func TestParseNmapServiceProbeToRuleMap_DefaultPorts(t *testing.T) {
	in := `## 这个数据包可以发送 RDP 的指纹检测数据包
Probe TCP RdpSSLHybridAndHybridEx q|\x03\x00\x003.\xe0\x00\x00\x00\x00\x00\x43\x6f\x6f\x6b\x69\x65\x3a\x20\x6d\x73\x74\x73\x68\x61\x73\x68\x3dadministrator\x0d\x0a\x01\x00\x08\x00\x0b\x00\x00\x00|
ports 3388,3389

## 这里是测试数据包
match rdp m|\x03.*| cpe:/a:microsoft:rdp1/a
match rdp m|\x03\x00\x00\x13\x0e\xd0\x00\x00\x124\x00\x02\x00\x08\x00\x02\x00\x00\x00| cpe:/a:microsoft:rdp/a
`
	probes, err := ParseNmapServiceProbeToRuleMap([]byte(in))
	if err != nil {
		t.Errorf("parse failed: %s", err)
		t.FailNow()
	}

	executed := false
	for probe, _ := range probes {
		log.Infof("%v", probe.DefaultPorts)
		executed = true
		if !(utils.IntArrayContains(probe.DefaultPorts, 3389) && utils.IntArrayContains(probe.DefaultPorts, 3388)) {
			t.Error("parse ports failed: no default ports")
			t.FailNow()
		}
	}

	if !executed {
		t.Error("parsed failed")
		t.FailNow()
	}
}
