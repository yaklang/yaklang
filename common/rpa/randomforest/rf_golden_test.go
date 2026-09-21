package randomforest

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/rpa/character"
	local "github.com/yaklang/yaklang/common/rpa/randomforest/internal/rf"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"testing"
)

func TestEmbeddedModelLeafGolden(t *testing.T) {
	raw, err := utils.GzipDeCompress(embeddedRFModelGz)
	if err != nil {
		t.Fatal(err)
	}
	b := &local.Forest{}
	if err = json.NewDecoder(bytes.NewReader(raw)).Decode(b); err != nil {
		t.Fatal(err)
	}
	inputs := []string{"http://example.com/", "http://example.com/index.php?a=select+1", "https://example.com/login", "javascript:void(0)", "/admin", "", "中文"}
	r := rand.New(rand.NewSource(42))
	alphabet := []byte("abc0123/:.?=&%_-+")
	for i := 0; i < 1000; i++ {
		s := make([]byte, r.Intn(100))
		for j := range s {
			s[j] = alphabet[r.Intn(len(alphabet))]
		}
		inputs = append(inputs, string(s))
	}
	h := sha256.New()
	for _, s := range inputs {
		v := character.String2Vec(s)
		for i := range b.Trees {
			encoded, _ := json.Marshal(local.PredicateTree(b.Trees[i], v))
			fmt.Fprintln(h, string(encoded))
		}
	}
	// Frozen from RF.go 46700521f302 over the same embedded model and inputs.
	const want = "d7136d4343c9ec95e7bf129ab489687131db14a05030206af3e3f7ecd7613d8c"
	if got := fmt.Sprintf("%x", h.Sum(nil)); got != want {
		t.Fatalf("leaf votes changed: %s", got)
	}
	t.Logf("compared %d inputs across %d trees", len(inputs), len(b.Trees))
}
