package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestType_Enum(t *testing.T) {
	t.Run("enum with attribute", func(t *testing.T) {
		test.CheckPrintlnValue(`
typedef enum {
    CURLSSLBACKEND_NONE = 0,
    CURLSSLBACKEND_OPENSSL = 1,
    CURLSSLBACKEND_POLARSSL __attribute__((deprecated("since 7.69.0"))) = 6,
    CURLSSLBACKEND_WOLFSSL = 7
} curl_sslbackend;

int main() {
    curl_sslbackend backend = CURLSSLBACKEND_OPENSSL;
    println(backend);
    return 0;
}
		`, []string{"1"}, t)
	})

	t.Run("enum basic", func(t *testing.T) {
		test.CheckPrintlnValue(`
enum Color {
    RED,
    GREEN,
    BLUE
};

int main() {
    enum Color c = RED;
    println(c);
    c = BLUE;
    println(c);
    return 0;
}
		`, []string{"0", "2"}, t)
	})

	t.Run("enum with explicit values", func(t *testing.T) {
		test.CheckPrintlnValue(`
enum Status {
    PENDING = 10,
    RUNNING = 20,
    COMPLETED = 30
};

int main() {
    enum Status s = PENDING;
    println(s);
    s = COMPLETED;
    println(s);
    return 0;
}
		`, []string{"10", "30"}, t)
	})

	t.Run("enum typedef", func(t *testing.T) {
		test.CheckPrintlnValue(`
typedef enum {
    STATE_IDLE,
    STATE_ACTIVE,
    STATE_ERROR
} State;

int main() {
    State s = STATE_ACTIVE;
    println(s);
    return 0;
}
		`, []string{"1"}, t)
	})
}

func TestType_BasicTypes(t *testing.T) {
	t.Run("int types", func(t *testing.T) {
		test.CheckPrintlnValue(`
int main() {
    int a = 10;
    short b = 20;
    long c = 30;
    long long d = 40;
    
    println(a);
    println(b);
    println(c);
    println(d);
    return 0;
}
		`, []string{"10", "20", "30", "40"}, t)
	})

	t.Run("char type", func(t *testing.T) {
		test.CheckPrintlnValue(`
int main() {
    char c = 'A';
    unsigned char uc = 255;
    
    println(c);
    println(uc);
    return 0;
}
		`, []string{"65", "255"}, t)
	})

	t.Run("float types", func(t *testing.T) {
		test.CheckPrintlnValue(`
int main() {
    float f = 3.14;
    double d = 2.718;
    long double ld = 1.414;
    
    println(f);
    println(d);
    println(ld);
    return 0;
}
		`, []string{"3.14", "2.718", "1.414"}, t)
	})

	t.Run("signed unsigned", func(t *testing.T) {
		test.CheckPrintlnValue(`
int main() {
    signed int si = -100;
    unsigned int ui = 200;
    
    println(si);
    println(ui);
    return 0;
}
		`, []string{"-100", "200"}, t)
	})
}

func TestType_Struct(t *testing.T) {
	t.Run("basic struct", func(t *testing.T) {
		test.CheckPrintlnValue(`
struct Point {
    int x;
    int y;
};

int main() {
    struct Point p;
    p.x = 10;
    p.y = 20;
    
    println(p.x);
    println(p.y);
    return 0;
}
		`, []string{"10", "20"}, t)
	})

	t.Run("struct typedef", func(t *testing.T) {
		test.CheckPrintlnValue(`
typedef struct {
    int id;
    char name[10];
} Person;

int main() {
    Person p;
    p.id = 1;
    println(p.id);
    return 0;
}
		`, []string{"1"}, t)
	})

	t.Run("nested struct", func(t *testing.T) {
		test.CheckPrintlnValue(`
struct Address {
    int street;
    int zip;
};

struct Person {
    int id;
    struct Address addr;
};

int main() {
    struct Person p;
    p.id = 1;
    p.addr.street = 123;
    p.addr.zip = 456;
    
    println(p.id);
    println(p.addr.street);
    println(p.addr.zip);
    return 0;
}
		`, []string{"1", "123", "456"}, t)
	})
}

func TestType_Union(t *testing.T) {
	t.Run("basic union", func(t *testing.T) {
		test.CheckPrintlnValue(`
union Data {
    int i;
    float f;
    char c;
};

int main() {
    union Data d;
    d.i = 100;
    println(d.i);
    d.f = 3.14;
    println(d.f);
    return 0;
}
		`, []string{"100", "3.14"}, t)
	})
}

func TestType_Pointer(t *testing.T) {
	t.Run("pointer to int", func(t *testing.T) {
		test.CheckPrintlnValue(`
int main() {
    int a = 10;
    int *p = &a;
    
    println(a);
    println(*p);
    *p = 20;
    println(a);
    return 0;
}
		`, []string{"10", "10", "20"}, t)
	})

	t.Run("pointer to pointer", func(t *testing.T) {
		test.CheckPrintlnValue(`
int main() {
    int a = 10;
    int *p = &a;
    int **pp = &p;
    
    println(a);
    println(*p);
    println(**pp);
    return 0;
}
		`, []string{"10", "10", "10"}, t)
	})

	t.Run("pointer to struct", func(t *testing.T) {
		test.CheckPrintlnValue(`
struct Point {
    int x;
    int y;
};

int main() {
    struct Point p;
    p.x = 10;
    p.y = 20;
    
    struct Point *ptr = &p;
    println(ptr->x);
    println(ptr->y);
    return 0;
}
		`, []string{"10", "20"}, t)
	})
}

func TestType_Array(t *testing.T) {
	t.Run("basic array", func(t *testing.T) {
		test.CheckPrintlnValue(`
int main() {
    int arr[5] = {1, 2, 3, 4, 5};
    
    println(arr[0]);
    println(arr[2]);
    println(arr[4]);
    return 0;
}
		`, []string{"1", "3", "5"}, t)
	})

	t.Run("array of structs", func(t *testing.T) {
		test.CheckPrintlnValue(`
struct Point {
    int x;
    int y;
};

int main() {
    struct Point points[3];
    points[0].x = 1;
    points[0].y = 2;
    points[1].x = 3;
    points[1].y = 4;
    
    println(points[0].x);
    println(points[1].y);
    return 0;
}
		`, []string{"1", "4"}, t)
	})

	t.Run("multidimensional array", func(t *testing.T) {
		test.CheckPrintlnValue(`
int main() {
    int matrix[2][3] = {{1, 2, 3}, {4, 5, 6}};
    
    println(matrix[0][0]);
    println(matrix[0][2]);
    println(matrix[1][1]);
    return 0;
}
		`, []string{"1", "3", "5"}, t)
	})
}

func TestType_FunctionPointer(t *testing.T) {
	t.Run("function pointer", func(t *testing.T) {
		test.CheckPrintlnValue(`
int add(int a, int b) {
    return a + b;
}

int main() {
    int (*func_ptr)(int, int) = add;
    int result = func_ptr(5, 3);
    println(result);
    return 0;
}
		`, []string{"Function-add(5,3)"}, t)
	})
}

func TestType_Typedef(t *testing.T) {
	t.Run("typedef int", func(t *testing.T) {
		test.CheckPrintlnValue(`
typedef int MyInt;

int main() {
    MyInt a = 10;
    MyInt b = 20;
    println(a + b);
    return 0;
}
		`, []string{"30"}, t)
	})

	t.Run("typedef pointer", func(t *testing.T) {
		test.CheckPrintlnValue(`
typedef int* IntPtr;

int main() {
    int a = 10;
    IntPtr p = &a;
    println(*p);
    return 0;
}
		`, []string{"10"}, t)
	})

	t.Run("typedef struct", func(t *testing.T) {
		test.CheckPrintlnValue(`
typedef struct {
    int x;
    int y;
} Point;

int main() {
    Point p;
    p.x = 5;
    p.y = 10;
    println(p.x);
    println(p.y);
    return 0;
}
		`, []string{"5", "10"}, t)
	})
}

func TestType_Complex(t *testing.T) {
	t.Run("complex type combination", func(t *testing.T) {
		test.CheckPrintlnValue(`
typedef struct {
    int id;
    char name[20];
} Person;

typedef Person* PersonPtr;

int main() {
    Person p;
    p.id = 1;
    PersonPtr ptr = &p;
    println(ptr->id);
    return 0;
}
		`, []string{"1"}, t)
	})

	t.Run("array of pointers", func(t *testing.T) {
		test.CheckPrintlnValue(`
int main() {
    int a = 1, b = 2, c = 3;
    int *arr[3] = {&a, &b, &c};
    
    println(*arr[0]);
    println(*arr[1]);
    println(*arr[2]);
    return 0;
}
		`, []string{"1", "2", "3"}, t)
	})

	t.Run("pointer to array", func(t *testing.T) {
		test.CheckPrintlnValue(`
int main() {
    int arr[5] = {1, 2, 3, 4, 5};
    int (*ptr)[5] = &arr;
    
    println((*ptr)[0]);
    println((*ptr)[2]);
    return 0;
}
		`, []string{"1", "3"}, t)
	})
}

func TestType_Qualifiers(t *testing.T) {
	t.Run("const type", func(t *testing.T) {
		test.CheckPrintlnValue(`
int main() {
    const int a = 10;
    println(a);
    return 0;
}
		`, []string{"10"}, t)
	})

	t.Run("volatile type", func(t *testing.T) {
		test.CheckPrintlnValue(`
int main() {
    volatile int a = 20;
    println(a);
    return 0;
}
		`, []string{"20"}, t)
	})

	t.Run("const pointer", func(t *testing.T) {
		test.CheckPrintlnValue(`
int main() {
    int a = 10;
    const int *p = &a;
    println(*p);
    return 0;
}
		`, []string{"10"}, t)
	})
}
