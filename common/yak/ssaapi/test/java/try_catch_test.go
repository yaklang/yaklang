package java

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestErrorHandler_CatchBlock(t *testing.T) {

	rule := `
*?{opcode:try} as $try 
$try.catch.body as $catch_body
$catch_body<scanInstruction> as $inst 
$catch_body?{!<scanInstruction>} as $no_code_catch
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
		ssatest.CheckSyntaxFlow(t, code, rule, want, ssaapi.WithLanguage(ssaconfig.JAVA))
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

	t.Run("test exception", func(t *testing.T) {
		rule := `
*?{opcode:catch} as $catch
$catch.exception as $exception
		`
		ssatest.CheckSyntaxFlow(t, code, rule, map[string][]string{
			"exception": {`Undefined-eeeeee`},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test exception with type", func(t *testing.T) {

		rule := `
*?{opcode:catch} as $catch
$catch.exception as $exception
$exception<typeName> as $type_name
		`
		ssatest.CheckSyntaxFlow(t, code, rule, map[string][]string{
			"type_name": {`"Exception"`},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test exception with type and users", func(t *testing.T) {
		// test with type and users
		rule := `
*?{opcode:catch} as $catch
$catch.exception as $exception
$exception<getUsers>?{!opcode:catch} as $users
		`
		ssatest.CheckSyntaxFlow(t, code, rule, map[string][]string{
			"users": {`Undefined-eeeeee.printStackTrace(Undefined-eeeeee)`},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

}

func TestErrorHandler_SourceCode(t *testing.T) {
	rule := `
*?{opcode:try} as $try
$try.catch as $catch
$catch.body  as $catch_body
$catch.exception as $exception

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
	ssatest.CheckSyntaxFlowSource(t, code, rule, map[string][]string{
		"catch_body": {`catch (Exception eeeeee) {
			eeeeee.printStackTrace();
		}`,
		},
		"exception": {`eeeeee`},
	}, ssaapi.WithLanguage(ssaconfig.JAVA))

}

func TestErrorHandler_Function_Throw(t *testing.T) {
	rule := `
*?{opcode:function} as $function 
$function.throws as $throws
`

	t.Run("test method exception", func(t *testing.T) {
		code := `
package org.aa.com;
public class AA{
	public void onMessage2(ByteBuffer b) throws Exception {
	}
}
		`
		ssatest.CheckSyntaxFlow(t, code, rule, map[string][]string{
			"throws": {`"Exception"`},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test method exception source code", func(t *testing.T) {
		code := `
package org.aa.com;
public class AA{
	public void onMessage2(ByteBuffer b) throws Exception {
	}
}
		`
		ssatest.CheckSyntaxFlowSource(t, code, rule, map[string][]string{
			"throws": {`Exception`},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test constructor exception", func(t *testing.T) {
		code := `
package org.aa.com;
public class AA{
	AA() throws Exception {
	}
}
		`
		ssatest.CheckSyntaxFlow(t, code, rule, map[string][]string{
			"throws": {`"Exception"`},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test interface function exception", func(t *testing.T) {
		code := `
package org.aa.com;
public interface AA{
	int A() throws Exception;
}
		`

		ssatest.CheckSyntaxFlow(t, code, rule, map[string][]string{
			"throws": {`"Exception"`},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func TestErrorHandler_Throw(t *testing.T) {
	t.Run("test throw", func(t *testing.T) {
		code := `
package org.aa.com;
public class AA{
	public void onMessage2(ByteBuffer b) throws Exception {
		throw new Exception("test");
	}
}
	`
		rule := `
*?{opcode:throw} as $throw
`
		ssatest.CheckSyntaxFlow(t, code, rule, map[string][]string{
			"throw": {"panic(Exception(Undefined-Exception,\"test\"))"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test throw with source code", func(t *testing.T) {
		code := `
package org.aa.com;
public class AA{
	public void onMessage2(ByteBuffer b) throws Exception {
		throw new Exception("test");
	}
}
	`
		rule := `
*?{opcode:throw} as $throw
`
		ssatest.CheckSyntaxFlowSource(t, code, rule, map[string][]string{
			"throw": {`throw new Exception("test");`},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test throw Block", func(t *testing.T) {
		code := `
package org.aa.com;
public class AA{
	public void onMessage2(ByteBuffer b) throws Exception {
		throw new Exception("block");
		try {
			throw new Exception("try");
		} catch (Exception e) {
			throw new Exception("catch");
		} finally {
			throw new Exception("finally");
		}
	}
}
	`
		rule := `
*?{opcode:try} as $try
$try.finally as $finally 
$finally<scanInstruction>?{opcode:throw} as $throw 
`
		ssatest.CheckSyntaxFlowSource(t, code, rule, map[string][]string{
			"throw": {`throw new Exception("finally");`},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
