package stream_parser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestDiscardedTrialMatchesResultTrial(t *testing.T) {
	const parserName = "discarded-trial-result-test"
	previous := base.ParserRegistration(parserName)
	t.Cleanup(func() { base.RegisterParser(parserName, previous) })
	for _, failure := range []string{"none", "parse-error", "parse-panic", "result-error", "result-panic", "finish-panic", "nil-finish", "missing-type"} {
		t.Run(failure, func(t *testing.T) {
			var traces [2][]string
			for mode := 0; mode < 2; mode++ {
				root := giopBridgeInlineRoot(t, "Package:\n  Parent:\n    Value: uint8\nTrial: uint8\n")
				require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader([]byte{1}))))
				n := root.Children[0].Children[0]
				trace := func(value string) { traces[mode] = append(traces[mode], value) }
				base.RegisterParser(parserName, &discardedCallbackParser{result: func(*base.Node) (*base.NodeValue, error) {
					trace("result")
					if failure == "result-panic" {
						panic("result panic")
					}
					if failure == "result-error" {
						return nil, fmt.Errorf("result error")
					}
					return nil, nil
				}})
				yn := convertOperatorNode(n, func(target *base.Node) (func(bool), error) {
					trace("parse")
					if failure == "parse-panic" {
						panic("parse panic")
					}
					target.Cfg.SetItem("parser", parserName)
					if failure == "nil-finish" {
						return nil, nil
					}
					finish := func(recover bool) {
						trace(fmt.Sprint("finish ", recover))
						if failure == "finish-panic" {
							panic("finish panic")
						}
					}
					if failure == "parse-error" {
						return finish, fmt.Errorf("parse error")
					}
					return finish, nil
				})
				name := "Trial"
				if failure == "missing-type" {
					name = "Missing"
				}
				outer := discardedOutcome(func() error {
					var ok bool
					var message string
					var save, recovery func()
					if mode == 0 {
						_, trial := yn.TryProcessByType(name)
						ok, message = trial["OK"].(bool), trial["Message"].(string)
						save, recovery = trial["Save"].(func()), trial["Recovery"].(func())
					} else {
						trial := yn.tryProcessByTypeDiscard(name)
						ok, message, save, recovery = trial.ok, trial.message, trial.save, trial.recovery
					}
					trace(fmt.Sprintf("trial %t %s", ok, message))
					trace(discardedOutcome(func() error { save(); return nil }))
					trace(discardedOutcome(func() error { recovery(); return nil }))
					return nil
				})
				trace(outer)
				trace(fmt.Sprint("children ", len(n.Children)))
			}
			require.Equal(t, traces[0], traces[1])
		})
	}
}
