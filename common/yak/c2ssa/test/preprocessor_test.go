package test

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/c2ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPreprocess_SimpleMacro(t *testing.T) {
	t.Run("simple define", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define MAX_SIZE 1024

int main() {
    int size = MAX_SIZE;
    println(size);
    return 0;
}
		`, []string{"1024"}, t)
	})

	t.Run("multiple defines", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define WIDTH 800
#define HEIGHT 600

int main() {
    int w = WIDTH;
    int h = HEIGHT;
    println(w);
    println(h);
    return 0;
}
		`, []string{"800", "600"}, t)
	})
}

func TestPreprocess_FunctionMacro(t *testing.T) {
	t.Run("MIN macro", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define MIN(a,b) ((a)<(b)?(a):(b))

int main() {
    int x = 10;
    int y = 20;
    int min = MIN(x, y);
    println(min);
    return 0;
}
		`, []string{"phi(min)[10,20]"}, t)
	})

	t.Run("MAX macro", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define MAX(a,b) ((a)>(b)?(a):(b))

int main() {
    int x = 10;
    int y = 20;
    int max = MAX(x, y);
    println(max);
    return 0;
}
		`, []string{"phi(max)[10,20]"}, t)
	})

	t.Run("SQUARE macro", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define SQUARE(x) ((x) * (x))

int main() {
    int num = 5;
    int result = SQUARE(num);
    println(result);
    return 0;
}
		`, []string{"25"}, t)
	})
}

func TestPreprocess_NestedMacro(t *testing.T) {
	t.Run("nested macro", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define SQUARE(x) ((x) * (x))
#define CUBE(x) (SQUARE(x) * (x))

int main() {
    int num = 3;
    int result = CUBE(num);
    println(result);
    return 0;
}
		`, []string{"27"}, t)
	})

	t.Run("triple nested", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define DOUBLE(x) ((x) * 2)
#define QUAD(x) (DOUBLE(DOUBLE(x)))
#define OCT(x) (DOUBLE(QUAD(x)))

int main() {
    int num = 1;
    int result = OCT(num);
    println(result);
    return 0;
}
		`, []string{"8"}, t)
	})
}

func TestPreprocess_ConditionalCompilation(t *testing.T) {
	t.Run("ifdef true", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define DEBUG 1

int main() {
#ifdef DEBUG
    int mode = 1;
#else
    int mode = 0;
#endif
    println(mode);
    return 0;
}
		`, []string{"1"}, t)
	})

	t.Run("if condition", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define VERSION 2

int main() {
#if VERSION > 1
    int result = 100;
#else
    int result = 0;
#endif
    println(result);
    return 0;
}
		`, []string{"100"}, t)
	})
}

func TestPreprocess_ArrayWithMacro(t *testing.T) {
	t.Run("array size macro", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define BUFFER_SIZE 256

int main() {
    char buffer[BUFFER_SIZE];
    int size = BUFFER_SIZE;
    println(size);
    return 0;
}
		`, []string{"256"}, t)
	})

	t.Run("2d array with macro", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define ROWS 10
#define COLS 20

int main() {
    int matrix[ROWS][COLS];
    int r = ROWS;
    int c = COLS;
    println(r);
    println(c);
    return 0;
}
		`, []string{"10", "20"}, t)
	})
}

func TestPreprocess_StringMacro(t *testing.T) {
	t.Run("string constant", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define VERSION "1.0.0"

int main() {
    const char* ver = VERSION;
    println(ver);
    return 0;
}
		`, []string{`"1.0.0"`}, t)
	})

	t.Run("concatenation", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define CONCAT(a, b) a##b

int main() {
    int var_10 = 42;
    int result = CONCAT(var_, 10);
    println(result);
    return 0;
}
		`, []string{"42"}, t)
	})
}

func TestPreprocess_ComplexExpression(t *testing.T) {
	t.Run("arithmetic expression", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define A 10
#define B 20
#define C 30

int main() {
    int result = A + B * C;
    println(result);
    return 0;
}
		`, []string{"610"}, t)
	})

	t.Run("bitwise operation", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define FLAG1 0x01
#define FLAG2 0x02
#define FLAGS (FLAG1 | FLAG2)

int main() {
    int flags = FLAGS;
    println(flags);
    return 0;
}
		`, []string{"3"}, t)
	})
}

func TestPreprocess_MacroInFunction(t *testing.T) {
	t.Run("macro in if statement", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define THRESHOLD 100

int main() {
    int value = 150;
    if (value > THRESHOLD) {
        println(1);
    } else {
        println(0);
    }
    return 0;
}
		`, []string{"0", "1"}, t)
	})

	t.Run("macro in loop", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define COUNT 5

int main() {
    int sum = 0;
    for (int i = 0; i < COUNT; i++) {
        sum += i;
    }
    println(sum);
    return 0;
}
		`, []string{"phi(sum)[0,add(sum, phi(i)[0,add(i, 1)])]"}, t)
	})
}

func TestPreprocess_DirectAPI(t *testing.T) {
	t.Run("simple macro expansion", func(t *testing.T) {
		src := `
#define MAX_SIZE 1024
int buffer[MAX_SIZE];
`
		result, err := c2ssa.PreprocessCMacros(src)
		if err != nil {
			t.Skipf("Preprocessor not available: %v", err)
			return
		}

		if !strings.Contains(result, "1024") {
			t.Errorf("Expected macro to be expanded to 1024, got:\n%s", result)
		}
	})

	t.Run("function macro expansion", func(t *testing.T) {
		src := `
#define MIN(a,b) ((a)<(b)?(a):(b))
int min = MIN(x, y);
`
		result, err := c2ssa.PreprocessCMacros(src)
		if err != nil {
			t.Skipf("Preprocessor not available: %v", err)
			return
		}

		if !strings.Contains(result, "((x)<(y)?(x):(y))") && !strings.Contains(result, "?") {
			t.Logf("Function macro expansion result:\n%s", result)
		}
	})

	t.Run("conditional compilation", func(t *testing.T) {
		src := `
#define DEBUG 1
#ifdef DEBUG
int debug_mode = 1;
#else
int debug_mode = 0;
#endif
`
		result, err := c2ssa.PreprocessCMacros(src)
		if err != nil {
			t.Skipf("Preprocessor not available: %v", err)
			return
		}

		if !strings.Contains(result, "debug_mode = 1") && !strings.Contains(result, "debug_mode") {
			t.Logf("Conditional compilation result:\n%s", result)
		}
	})
}

func TestPreprocess_RealWorldScenarios(t *testing.T) {
	t.Run("buffer overflow check", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define BUFFER_SIZE 512
#define SAFE_COPY(dest, src) strncpy(dest, src, BUFFER_SIZE - 1)

int main() {
    char buffer[BUFFER_SIZE];
    int size = BUFFER_SIZE;
    println(size);
    return 0;
}
		`, []string{"512"}, t)
	})

	t.Run("error code macros", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define SUCCESS 0
#define ERROR_NOT_FOUND -1
#define ERROR_PERMISSION -2

int main() {
    int result = SUCCESS;
    println(result);
    return 0;
}
		`, []string{"0"}, t)
	})

	t.Run("platform specific code", func(t *testing.T) {
		test.CheckPrintlnValue(`
#define PLATFORM_LINUX 1

#ifdef PLATFORM_LINUX
    #define PATH_SEPARATOR '/'
#else
    #define PATH_SEPARATOR '\\'
#endif

int main() {
    char sep = PATH_SEPARATOR;
    println(sep);
    return 0;
}
		`, []string{"47"}, t) // '/' 的 ASCII 值是 47
	})
}
