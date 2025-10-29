package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBasic_BasicObject(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `
#include <stdio.h>
struct t {
    int b;
    int c;
};

int main() {
    struct t a = {0, 0};
    a.b = 1;
    a.c = 3;
    int d = a.c + a.b;
    return 0;
}
		`,
			`d #-> as $target`,
			map[string][]string{
				"target": {"3", "1"},
			},
			ssaapi.WithLanguage(ssaconfig.C),
		)
	})

	t.Run("simple cross function", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `
	#include <stdio.h>
	struct t {
		int b;
		int c;
	};

	struct void  f() {
		struct t result = {1, 3};
	}
	`, `result.b as $target `, map[string][]string{
			"target": {"1"},
		}, ssaapi.WithLanguage(ssaconfig.C))
	})

	t.Run("simple cross function extend", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `
	#include <stdio.h>
	struct t {
		int b;
		int c;
	};

	struct t f() {
		struct t result = {1, 3};
		return result;
	}

	int main() {
		struct t a = f();
		int d = a.c  + 2 ;
		return 0;
	}
			`, `d #-> as $target`,
			map[string][]string{
				"target": {"3", "2"},
			},
			ssaapi.WithLanguage(ssaconfig.C),
		)
	})
}

func TestBasic_BasicObjectEx(t *testing.T) {
	ssatest.CheckSyntaxFlowContain(t, `
#include <stdio.h>
#include <stdlib.h>

struct Queue {
    int mu;
};

struct Queue* NewQueue() {
    struct Queue* q = malloc(sizeof(struct Queue));
    q->mu = 1;
    return q;
}

int main() {
    struct Queue* a = NewQueue();
    int b = a->mu;
    return 0;
}
	`, `
	q.mu as $qmu 
	b #-> as $target
	`, map[string][]string{
		"target": {"1"},
	},
		ssaapi.WithLanguage(ssaconfig.C),
	)
}

func TestBasic_Phi(t *testing.T) {
	ssatest.CheckSyntaxFlowContain(t,
		`
#include <stdio.h>
int main() {
    int a = 0;
    if (a > 0) {
        a = 1;
    } else if (a > 1) {
        a = 2;
    } else {
        a = 4;
    }
    println(a);
    return 0;
}
		`, `
	a ?{opcode: phi} as $p
	$p #-> as $target
	`, map[string][]string{
			"p":      {"phi(a)[1,phi(a)[2,4]]", "phi(a)[2,4]"},
			"target": {"0", "1", "2", "4"},
		},
		ssaapi.WithLanguage(ssaconfig.C),
	)
}

func TestBasic_BasicStruct(t *testing.T) {
	ssatest.CheckSyntaxFlowContain(t,
		`
#include <stdio.h>
struct A {
    int a;
    int b;
    int c;
};

void println(int x) {}

int main() {
    struct A* t1 = malloc(sizeof(struct A));
    t1->a = 1;
    t1->b = 2;
    t1->c = 3;
    println(t1->a);
    return 0;
}
		`, `
	println(* #-> as $a)
	`, map[string][]string{
			"a": {"1"},
		},
		ssaapi.WithLanguage(ssaconfig.C),
	)
}

func TestParameter_MemberCall(t *testing.T) {
	t.Run("membercall normal", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("test.c", `
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

struct Request {
    char* url;
    char* query;
};

struct Response {
    void (*write)(struct Response*, char*, int);
};

void handler(struct Response* w, struct Request* r) {
    char* userInput = r->query;
    char* content = malloc(100);
    strcpy(content, userInput);
    w->write(w, content, strlen(content));
}
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs, `
			handler as $entry
			$entry<getFormalParams> as $output
		`, map[string][]string{
			"output": {"Parameter-r", "Parameter-w"},
		}, true, ssaapi.WithLanguage(ssaconfig.C),
		)
	})

	t.Run("method normal", func(t *testing.T) {
		code := `
#include <stdio.h>
#include <string.h>

struct Context {
    void (*header)(struct Context*, char*, char*);
};

void cors1(struct Context* c) {
    c->header(c, "Access-Control-Allow-Origin", "*");
    c->header(c, "Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE");
}
		`
		ssatest.CheckSyntaxFlow(t, code, `
			*.header()?{have: "Access-Control-Allow-Origin"} as $header
			$header<getCallee>(,,* #-> as $output)
		`, map[string][]string{
			"output": {"\"*\""},
		},
			ssaapi.WithLanguage(ssaconfig.C),
		)
	})
}

func TestStruct_Array(t *testing.T) {
	ssatest.CheckSyntaxFlowContain(t, `
#include <stdio.h>
struct Point {
    int x;
    int y;
};

int main() {
    struct Point points[3] = {{1,2}, {3,4}, {5,6}};
    int sum = points[0].x + points[1].y;
    return 0;
}
	`, `
	sum #-> as $target
	`, map[string][]string{
		"target": {"1", "4"},
	},
		ssaapi.WithLanguage(ssaconfig.C),
	)
}

func TestStruct_Pointer(t *testing.T) {
	ssatest.CheckSyntaxFlowContain(t, `
#include <stdio.h>
#include <stdlib.h>

struct Node {
    int data;
    struct Node* next;
};

int main() {
    struct Node* head = malloc(sizeof(struct Node));
    head->data = 10;
    head->next = NULL;
    int value = head->data;
    return 0;
}
	`, `
	value #-> as $target
	`, map[string][]string{
		"target": {"10"},
	},
		ssaapi.WithLanguage(ssaconfig.C),
	)
}

func TestStruct_Union(t *testing.T) {
	ssatest.CheckSyntaxFlowContain(t, `
#include <stdio.h>
union Data {
    int i;
    float f;
    char str[20];
};

int main() {
    union Data data;
    data.i = 10;
    int value = data.i;
    return 0;
}
	`, `
	value #-> as $target
	`, map[string][]string{
		"target": {"10"},
	},
		ssaapi.WithLanguage(ssaconfig.C),
	)
}

func TestStruct_Enum(t *testing.T) {
	ssatest.CheckSyntaxFlowContain(t, `
#include <stdio.h>
enum Color {
    RED = 1,
    GREEN = 2,
    BLUE = 3
};

int main() {
    enum Color c = RED;
    int value = c;
    return 0;
}
	`, `
	value #-> as $target
	`, map[string][]string{
		"target": {"1"},
	},
		ssaapi.WithLanguage(ssaconfig.C),
	)
}

func TestStruct_Typedef(t *testing.T) {
	ssatest.CheckSyntaxFlowContain(t, `
#include <stdio.h>
typedef struct {
    int x;
    int y;
} Point;

int main() {
    Point p = {1, 2};
    int sum = p.x + p.y;
    return 0;
}
	`, `
	sum #-> as $target
	`, map[string][]string{
		"target": {"1", "2"},
	},
		ssaapi.WithLanguage(ssaconfig.C),
	)
}
