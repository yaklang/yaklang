package tests

import "testing"
import "github.com/stretchr/testify/require"

func TestComplex_ObjectFactor_MethodCall(t *testing.T) {
	check(t, `
	check = () => {
		f = () => {
			this = {
				"key": 1,
				"set": (i) => { this.key = i },
				"get": () => this.key,
			}
			return this
		}
		a = f()
		a.set(2)
		return a.get()
	}
	`, 2)
}

func TestComplex_ObjectFactor_MultipleInstances(t *testing.T) {
	check(t, `
	check = () => {
		f = () => {
			this = {
				"key": 1,
				"set": (i) => { this.key = i },
				"get": () => this.key,
			}
			return this
		}

		a = f()
		b = f()

		a.set(2)
		b.set(3)

		// Ensure instances don't bleed state.
		return a.get() * 10 + b.get()
	}
	`, 23)
}

func TestComplex_Closure_FreeValue(t *testing.T) {
	check(t, `
	check = () => {
		a = 41
		f = () => a + 1
		return f()
	}
	`, 42)
}

func TestComplex_Closure_CaptureParameter(t *testing.T) {
	check(t, `
	check = () => {
		f = (a) => {
			inner = () => a + 1
			return inner()
		}
		return f(41)
	}
	`, 42)
}

func TestComplex_Defer_PrintOrder(t *testing.T) {
	checkPrintBinary(t, `
	check = () => {
		defer println(1)
		println(2)
		return 0
	}
	`, 2, 1)
}

func TestComplex_TryCatchFinally_Panic(t *testing.T) {
	checkPrintBinary(t, `
	check = () => {
		try {
			panic(7)
			println(999)
		} catch err {
			println(1)
		} finally {
			println(2)
		}
		return 0
	}
	`, 1, 2)
}

func TestComplex_TryCatchFinally_PanicValue(t *testing.T) {
	checkPrintBinary(t, `
	check = () => {
		try {
			panic(7)
		} catch err {
			println(err)
		} finally {
			println(2)
		}
		return 0
	}
	`, 7, 2)
}

func TestComplex_TryCatchFinally_StringPanicValue(t *testing.T) {
	code := `
	check = () => {
		try {
			panic("boom")
		} catch err {
			println(err)
		} finally {
			println("done")
		}
		return 0
	}
	`
	output := runBinaryWithEnv(t, code, "check", nil)
	require.Equal(t, "boom\ndone\n", output)
}

func TestComplex_CatchRecover_StringPanicValue(t *testing.T) {
	code := `
	check = () => {
		try {
			panic("boom")
		} catch err {
			println(recover())
			println(err)
		}
		return 0
	}
	`
	output := runBinaryWithEnv(t, code, "check", nil)
	require.Equal(t, "boom\nboom\n", output)
}
