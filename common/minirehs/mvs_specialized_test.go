package minirehs

import (
	"math/rand"
	"regexp"
	"testing"
)

func TestMVSSpecializedAlwaysOnEquivalence(t *testing.T) {
	cases := []struct {
		expr string
		kind mvsSpecializedKind
		seed []string
	}{
		{mvsCNIDExpr, mvsSpecializedCNID, []string{"x11010519491231002X!", "11010519491231002X", "x123456789012345!"}},
		{mvsMACExpr, mvsSpecializedMAC, []string{"00:11:22:aa:BB:fF", "x00:11:22:33:44:55", "-00:11:22:33:44:55"}},
		{mvsPhoneExpr, mvsSpecializedPhone, []string{"13800138000", "+8613800138000", "14800138000"}},
		{mvsJSONExpr, mvsSpecializedJSON, []string{"{}", "{\n}\n", "x{}"}},
		{mvsAWSRegionExpr, mvsSpecializedAWSRegion, []string{"us-east-1", "us-gov-west-2", "cn--3", "xx-us-northeast-9-yy"}},
		{mvsWindowsPathExpr, mvsSpecializedWindowsPath, []string{`C:\Windows\a.txt`, `z:/tmp/a.go`, `C:\a.txt`}},
	}

	rng := rand.New(rand.NewSource(0x5eed))
	alphabet := []byte("abcXYZ019_:/\\.-{}+!\n")
	for _, tc := range cases {
		t.Run(tc.kind.String(), func(t *testing.T) {
			re := regexp.MustCompile(tc.expr)
			inputs := append([]string(nil), tc.seed...)
			for i := 0; i < 20000; i++ {
				n := rng.Intn(80)
				buf := make([]byte, n)
				for j := range buf {
					buf[j] = alphabet[rng.Intn(len(alphabet))]
				}
				if len(tc.seed) > 0 && i%7 == 0 {
					s := tc.seed[rng.Intn(len(tc.seed))]
					at := rng.Intn(len(buf) + 1)
					buf = append(buf[:at], append([]byte(s), buf[at:]...)...)
				}
				inputs = append(inputs, string(buf))
			}
			for _, input := range inputs {
				got := tc.kind.exists([]byte(input))
				want := re.MatchString(input)
				if got != want {
					t.Fatalf("expr=%q input=%q specialized=%v regexp=%v", tc.expr, input, got, want)
				}
				fused := mvsSpecializedMask([]byte(input), uint64(1)<<tc.kind)&(uint64(1)<<tc.kind) != 0
				if fused != want {
					t.Fatalf("expr=%q input=%q fused=%v regexp=%v", tc.expr, input, fused, want)
				}
			}
		})
	}
}

func (k mvsSpecializedKind) String() string {
	switch k {
	case mvsSpecializedCNID:
		return "cn-id"
	case mvsSpecializedMAC:
		return "mac"
	case mvsSpecializedPhone:
		return "phone"
	case mvsSpecializedJSON:
		return "json"
	case mvsSpecializedAWSRegion:
		return "aws-region"
	case mvsSpecializedWindowsPath:
		return "windows-path"
	default:
		return "unknown"
	}
}
