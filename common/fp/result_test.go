package fp

import (
	"testing"
)

func TestParseNmapStringByRegexp2Match(t *testing.T) {
	match, err := parseNmapMatch("match ssh m|^SSH-([\\d.]+)-OpenSSH_([\\w._-]+)[ -]{1,2}Ubuntu[ -_]([^\\r\\n]+)\\r?\\n| p/OpenSSH/ v/$2 Ubuntu $3/ i/Ubuntu Linux; protocol $1/ o/Linux/ cpe:/a:openbsd:openssh:$2/ cpe:/o:canonical:ubuntu_linux/ cpe:/o:linux:linux_kernel/")
	if err != nil {
		t.Logf("parse nmap match failed: %s", err)
		t.FailNow()
	}
	//SSH-2.0-OpenSSH_7.6p1 Ubuntu-4ubuntu0.3
	banner := []byte{0x53, 0x53, 0x48, 0x2d, 0x32, 0x2e, 0x30, 0x2d, 0x4f, 0x70, 0x65, 0x6e, 0x53, 0x53, 0x48, 0x5f, 0x37, 0x2e, 0x36, 0x70, 0x31, 0x20, 0x55, 0x62, 0x75, 0x6e, 0x74, 0x75, 0x2d, 0x34, 0x75, 0x62, 0x75, 0x6e, 0x74, 0x75, 0x30, 0x2e, 0x33, 0xd, 0xa}

	m, err := match.MatchRule.FindStringMatch(string(banner))
	if err != nil {
		t.Logf("match rule failed: %s", err)
		t.FailNow()
	}

	do := func(raw string, flag string) {
		if m != nil {
			result := parseNmapStringByRegexp2Match(raw, m)
			if result != flag {
				t.Logf("parse failed: %s expected: %s", result, flag)
				t.FailNow()
			}
		} else {
			t.Log("match failed")
			t.FailNow()
		}
	}

	for raw, flag := range map[string]string{
		"$1:$2:$3:$5":    "2.0:7.6p1:4ubuntu0.3:$5",
		"$1:$2:$3:$4":    "2.0:7.6p1:4ubuntu0.3:$4",
		"$1:$P(2):$3:$5": "2.0:7.6p1:4ubuntu0.3:$5",
	} {
		do(raw, flag)
	}
}

func TestParseNmapStringByRegexp2Match2(t *testing.T) {
	match, err := parseNmapMatch(`match ssh m|^SSH-([\d.]+)-OpenSSH_([^ ]+)[ -]{1,2}Ubuntu[ -_]([^\r\n]+)\r?\n| p/OpenSSH/ v/$2 Ubuntu $3/ i/Ubuntu Linux; protocol $1/ o/Linux/ cpe:/a:openbsd:openssh:$2/ cpe:/o:canonical:ubuntu_linux/ cpe:/o:linux:linux_kernel/`)
	if err != nil {
		t.Logf("parse nmap match failed: %s", err)
		t.FailNow()
	}
	//SSH-2.0-OpenSSH_7.6p1 Ubuntu-4ubuntu0.3
	banner := []byte{
		0x53, 0x53, 0x48, 0x2d, // SSH-
		0x32, 0x2e, 0x30, 0x2d, // 2.0-
		0x4f, 0x70, 0x65, 0x6e, 0x53, 0x53, 0x48, // OpenSSH
		0x5f,                                                                       // _
		0x37, 0x2e, 0x36 /* start add bad char*/, 0x00, 0xff /* end */, 0x70, 0x31, // 7.6\x00\xffp1
		0x20, 0x55, 0x62, 0x75, 0x6e, 0x74, 0x75, 0x2d, 0x34, 0x75, 0x62, 0x75, 0x6e, 0x74, 0x75, 0x30, 0x2e, 0x33, 0xd, 0xa}

	m, err := match.MatchRule.FindStringMatch(string(banner))
	if err != nil {
		t.Logf("match rule failed: %s", err)
		t.FailNow()
	}

	do := func(raw string, flag string) {
		if m != nil {
			result := parseNmapStringByRegexp2Match(raw, m)
			if result != flag {
				t.Logf("parse [%s] failed: %#v expected: %#v", raw, result, flag)
				t.FailNow()
			}
		} else {
			t.Logf("match failed (from %s expect %s)", raw, flag)
			t.FailNow()
		}
	}

	for raw, flag := range map[string]string{
		"$1:$P(2):$3:$5":                         "2.0:7.6p1:4ubuntu0.3:$5",
		"$1:$P(2):$3:$4":                         "2.0:7.6p1:4ubuntu0.3:$4",
		"$P(1):$P(2):$3:$4:$P(6)":                "2.0:7.6p1:4ubuntu0.3:$4:$P(6)",
		`$P(1):$P(2):$SUBST(3,".","_"):$4:$P(6)`: "2.0:7.6p1:4ubuntu0_3:$4:$P(6)",
	} {
		do(raw, flag)
	}
}

func TestParseNmapStringByRegexp2Match3(t *testing.T) {
	match, err := parseNmapMatch(`match ssh m|^SSH-([\d]+).*?-OpenSSH_([^ ]+)[ -]{1,2}Ubuntu[ -_]([^\r\n]+)\r?\n| p/OpenSSH/ v/$2 Ubuntu $3/ i/Ubuntu Linux; protocol $1/ o/Linux/ cpe:/a:openbsd:openssh:$2/ cpe:/o:canonical:ubuntu_linux/ cpe:/o:linux:linux_kernel/`)
	if err != nil {
		t.Logf("parse nmap match failed: %s", err)
		t.FailNow()
	}
	//SSH-22222.0-OpenSSH_7.6p1 Ubuntu-4ubuntu0.3
	banner := []byte{
		0x53, 0x53, 0x48, 0x2d, // SSH-
		0x32, 0x2e, 0x30, 0x2d, // 22222.0-
		0x4f, 0x70, 0x65, 0x6e, 0x53, 0x53, 0x48, // OpenSSH
		0x5f,                                                                       // _
		0x37, 0x2e, 0x36 /* start add bad char*/, 0x00, 0xff /* end */, 0x70, 0x31, // 7.6\x00\xffp1
		0x20, 0x55, 0x62, 0x75, 0x6e, 0x74, 0x75, 0x2d, 0x34, 0x75, 0x62, 0x75, 0x6e, 0x74, 0x75, 0x30, 0x2e, 0x33, 0xd, 0xa}

	m, err := match.MatchRule.FindStringMatch(string(banner))
	if err != nil {
		t.Logf("match rule failed: %s", err)
		t.FailNow()
	}

	do := func(raw string, flag string) {
		if m != nil {
			result := parseNmapStringByRegexp2Match(raw, m)
			if result != flag {
				t.Logf("parse [%s] failed: %#v expected: %#v", raw, result, flag)
				t.FailNow()
			}
		} else {
			t.Logf("match failed (from %s expect %s)", raw, flag)
			t.FailNow()
		}
	}

	for raw, flag := range map[string]string{
		`$I(1,"<"):$P(2):$SUBST(3,".","_"):$4:$P(6)`: "50:7.6p1:4ubuntu0_3:$4:$P(6)",
		`$I(1,">"):$P(2):$SUBST(3,".","_"):$4:$P(6)`: "12800:7.6p1:4ubuntu0_3:$4:$P(6)",
	} {
		do(raw, flag)
	}
}
