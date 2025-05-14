package java

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestErrorHandler_CatchBlock(t *testing.T) {

	rule := `
*?{opcode:try} as $try 
$try.catch as $catch 
$catch<scanInstruction> as $inst 
$catch?{!<scanInstruction>} as $no_code_catch
	`

	format := `
package org.joychou.config;

public class WebSocketsProxyEndpoint extends Endpoint {
	%s 
}
`

	check := func(t *testing.T, functionCode string, want map[string][]string) {
		code := fmt.Sprintf(format, functionCode)
		log.Infof("code: %s", code)
		ssatest.CheckSyntaxFlow(t, code, rule, want, ssaapi.WithLanguage(ssaapi.JAVA))
	}

	t.Run("test normal", func(t *testing.T) {
		check(t, `
	public void onMessage(ByteBuffer message) {
		try {
			process(message, session);
		} catch (Exception ignored) {
		}
	}
		`, map[string][]string{
			"inst":          {}, // must empty
			"no_code_catch": {"BasicBlock-error.catch-5"},
		})
	})

	t.Run("test with instruction in catch ", func(t *testing.T) {
		check(t, `
	public void onMessage2(ByteBuffer message) {
		try {
			process(message, session);
		} catch (Exception exception) {
			exception.printStackTrace();
		}
	}`, map[string][]string{
			"no_catch": {}, // empty
		})
	})

	t.Run("test with code after catch", func(t *testing.T) {
		check(t, `
	public void onMessage0(ByteBuffer message) {
		try {
			process(message, session);
		} catch (Exception ignored) {
		}
		print("a");
	}`, map[string][]string{
			"inst":          {}, // must empty
			"no_code_catch": {"BasicBlock-error.catch-5"},
		})
	})

	t.Run("test with if in catch", func(t *testing.T) {
		check(t, `
	public void onMessage1(ByteBuffer message) {
		try {
			process(message, session);
		} catch (Exception ignored) {
			if (a) {
			}
		}
	}`, map[string][]string{
			"no_code_catch": {}, // empty
		})
	})

}

func TestErrorHandler_Exception(t *testing.T) {

	rule := `
*?{opcode:try} as $try
$try.exception as $exception 
$exception<typeName> as $type_name
	`
	code := `
package org.joychou.config;
public class WebSocketsProxyEndpoint extends Endpoint {
	public void onMessage2(ByteBuffer b) {
		try {
			process(b, session);
		} catch (Exception eeeeee) {
			eeeeee.printStackTrace();
		}
	}
}
	`

	ssatest.CheckSyntaxFlow(t, code, rule, map[string][]string{
		"type_name": {`"Exception"`},
	}, ssaapi.WithLanguage(ssaapi.JAVA))

}
