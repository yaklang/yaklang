package aiprojection

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// These digests were captured from aicache.Split and aicache.Observe at
// db6b63778d using the default 1024-byte threshold. They cover the cache
// observation shape and the provider-visible messages, including cache_control.
func TestLegacyAICacheOutputCompatibility(t *testing.T) {
	setProjectionThresholdMerge(t, 1024)
	static := "<|AI_CACHE_SYSTEM_high-static|>stable<|AI_CACHE_SYSTEM_END_high-static|>"
	cases := map[string]struct {
		prompt string
		hash   string
	}{
		"000001": {hash: "088202f5f7c34652be18ce4ae709839aecf06bb8797a722f9b7c2656adf8dfab"},
		"000005": {hash: "49711dcf781a93e0605e7db8c841c0665e47a841357d92891666066ca8d74a34"},
		"000010": {hash: "a4a34f23783087bca8399c44b0ce6b229467a160ecbf2acbb279d4126564477f"},
		"000020": {hash: "613d03e1132f148547ed40c84c8bbafe3cdb45d57cb05994c0de91e06068242a"},
		"000040": {hash: "d1375ac4b21231e8c814310770665bbee388be07cfd0fc65b8a4ba35bbc8b328"},
		"000045": {hash: "653ec1e7035f311ef4a13102b6c0726961a32642d5b14bbddf1ae19d4a27c043"},
		"000060": {hash: "daf90e6b64ce87e6a5fb8fd185e19fa9d40604ceaf8c03fddaaa1aba69f6a9f9"},
		"000073": {hash: "24484175059ab556bb16e2c27989893773ee8f77b71b30ac6a454b3e5fd3e045"},
		"legacy-static": {
			prompt: "<|PROMPT_SECTION_high-static|>stable<|PROMPT_SECTION_END_high-static|><|PROMPT_SECTION_dynamic_x|>question<|PROMPT_SECTION_dynamic_END_x|>",
			hash:   "621a1b5343ee7d6d7db52eb48754df6889a14268d82a18463a0de635bf0f098e",
		},
		"malformed": {
			prompt: static + "<|PROMPT_SECTION_dynamic_x|>unterminated",
			hash:   "4ca424d9bd3904e4705820c7db19ca49e5b7ae4f470060f019e0342a13a00f7c",
		},
		"unknown": {
			prompt: static + "<|FUTURE_SECTION_new|>opaque<|FUTURE_SECTION_END_new|>",
			hash:   "80edb2a8b613f6b67539567f730043ff4a18d85b1670ad29db5977ed13cb1e6f",
		},
		"five-boundaries": {
			prompt: static + "<|AI_CACHE_FROZEN_semi-dynamic|>" + strings.Repeat("A", 1200) + "<|AI_CACHE_FROZEN_END_semi-dynamic|>" +
				"<|AI_CACHE_SEMI_semi|>middle<|AI_CACHE_SEMI_END_semi|>" +
				"<|AI_CACHE_SEMI2_semi|>" + strings.Repeat("B", 1200) + "<|AI_CACHE_SEMI2_END_semi|>tail",
			hash: "203f5fed118635b4e07a43e0cfe853e2e7ee67317a585c461e6bfa047ef472d7",
		},
		"reasoning-replay": {
			prompt: static + "<|PROMPT_SECTION_timeline|><|TIMELINE_b3t1|>before\n<|TIMELINE_MODEL_THINKING_V1_r1|>\n{\"v\":1,\"reasoning_content\":\"thinking\",\"content\":\"{\\\"@action\\\":\\\"finish\\\"}\"}\n<|TIMELINE_MODEL_THINKING_V1_END_r1|>\nafter<|TIMELINE_END_b3t1|><|PROMPT_SECTION_END_timeline|>",
			hash:   "8b16200e62aa5bc7458be2bd2edea0243f76d42fb07a31e418a49a8da0aae8bc",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			prompt := tc.prompt
			if prompt == "" {
				data, err := os.ReadFile(filepath.Join("testdata", "fixtures", name+".txt"))
				require.NoError(t, err)
				prompt = strings.ReplaceAll(string(data), "\r\n", "\n")
			}
			parsed := Parse(prompt)
			projection := Project(ProjectionInput{Sections: parsed.Sections()})
			payload, err := json.Marshal(struct {
				Split    *PromptSplit
				Messages []aispec.ChatDetail
			}{parsed.CacheSplit(), projection.Messages})
			require.NoError(t, err)
			require.Equal(t, tc.hash, fmt.Sprintf("%x", sha256.Sum256(payload)))
		})
	}
}
