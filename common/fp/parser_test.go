package fp

import "testing"

func TestParseNmapServiceProbesRuleMap(t *testing.T) {
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
	probes, err := ParseNmapServiceProbeToRuleMap([]byte(text))
	if err != nil {
		t.Logf("parse test rule failed: %s", err)
		t.FailNow()
	}

	if len(probes) != 2 {
		t.Logf("rule count %v is not right(2)", len(probes))
		t.FailNow()
	}

	flag := false
	for p, matches := range probes {
		if p.Name == "NULL" && p.Proto == TCP {
			if len(matches) == 9 {
				flag = true
			}
		}
	}

	if !flag {
		t.FailNow()
	}
}
