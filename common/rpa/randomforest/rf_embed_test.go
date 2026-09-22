package randomforest

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/rpa/character"
	"github.com/yaklang/yaklang/common/rpa/randomforest/internal/rf"
	"github.com/yaklang/yaklang/common/utils"
)

func TestLoadEmbeddedModel(t *testing.T) {
	sys := &UrlDetectSys{}
	assert.NoError(t, sys.LoadEmbeddedModel())
	assert.Contains(t, []string{"0", "1"}, sys.PredictX("http://example.com/index.php?a=select+1"))
}

func TestModelRoundTrip(t *testing.T) {
	sys := &UrlDetectSys{}
	assert.NoError(t, sys.LoadEmbeddedModel())
	path := t.TempDir() + "/model.json"
	assert.NoError(t, sys.DumpModel(path))
	loaded := &UrlDetectSys{}
	assert.NoError(t, loaded.LoadModel(path))
	assert.Equal(t, sys.model, loaded.model)
	// Overwriting a larger file must truncate the previous JSON.
	sys.model.Trees = sys.model.Trees[:1]
	assert.NoError(t, sys.DumpModel(path))
	assert.NoError(t, loaded.LoadModel(path))
	assert.Equal(t, sys.model, loaded.model)
	assert.Error(t, loaded.LoadModel(path+".missing"))
	assert.NoError(t, os.WriteFile(path, []byte("{broken"), 0600))
	assert.Error(t, loaded.LoadModel(path))
	assert.Error(t, sys.DumpModel(path+"/invalid"))
	assert.Error(t, (&UrlDetectSys{}).DumpModel(path))
}

func TestEmbeddedModelLeafGolden(t *testing.T) {
	raw, err := utils.GzipDeCompress(embeddedRFModelGz)
	if err != nil {
		t.Fatal(err)
	}
	b := &rf.Forest{}
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
			encoded, _ := json.Marshal(rf.PredicateTree(b.Trees[i], v))
			fmt.Fprintln(h, string(encoded))
		}
	}
	// Leaf-vote checksum for the embedded model (RF.go 46700521f302).
	const want = "d7136d4343c9ec95e7bf129ab489687131db14a05030206af3e3f7ecd7613d8c"
	if got := fmt.Sprintf("%x", h.Sum(nil)); got != want {
		t.Fatalf("leaf votes changed: %s", got)
	}
}

func TestPredictScore(t *testing.T) {
	sys := &UrlDetectSys{model: &rf.Forest{Trees: []*rf.Tree{{Root: &rf.TreeNode{Labels: map[string]int{"yes": 1}}}}}}
	samples := [][]interface{}{{}, {}}
	for _, tc := range []struct {
		labels []string
		want   float64
	}{
		{[]string{"yes", "yes"}, 1}, {[]string{"yes", "no"}, .5}, {[]string{"no", "no"}, 0},
	} {
		assert.Equal(t, tc.want, sys.PredictScore(samples, tc.labels))
	}
	assert.Zero(t, sys.PredictScore(nil, nil))
}
