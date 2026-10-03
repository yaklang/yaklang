package ai_test

// The gateway deliberately avoids a dependency cycle through aicommon. Yak and
// aid users load this executor automatically; standalone Go clients import it.
import _ "github.com/yaklang/yaklang/common/ai/aid/liteforge"
