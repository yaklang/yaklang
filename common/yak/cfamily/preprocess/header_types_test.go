package preprocess

import "testing"

func TestIsHeaderTypeStmt(t *testing.T) {
	keep := []string{
		"typedef unsigned int demo_size_t;",
		"typedef struct demo_node demo_node_t;",
		"typedef void (*handler)(int);",
		"struct demo_node { int value; };",
		"struct demo_node;",
		"enum { A = 1, B = 2 };",
		"union demo_u { int a; char b; };",
	}
	drop := []string{
		"int demo_library_func(int x);",
		"static inline int demo_inline(void) { return 1; }",
		"extern int errno;",
		"struct demo_node *demo_new(void);",
		"void *malloc(unsigned long);",
	}
	for _, stmt := range keep {
		if !isHeaderTypeStmt(stmt) {
			t.Fatalf("expected type stmt: %s", stmt)
		}
	}
	for _, stmt := range drop {
		if isHeaderTypeStmt(stmt) {
			t.Fatalf("did not expect type stmt: %s", stmt)
		}
	}
}
