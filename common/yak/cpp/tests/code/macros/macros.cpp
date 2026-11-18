#include "framework.hpp"
#include <string>

int foobar();
static int checks = 0;
static void cf_check(const char* expr, int ok) {
    if (ok) {
        ++checks;
    }
}
static int cf_register(void (*fn)(), const char* name) {
    fn();
    return 0;
}
static void cf_log(const char* level, ...) {}
const char* operator""_mark(const char* text, unsigned long) { return text; }

namespace {
int compute(const std::string& text) { return (int)text.size(); }
}

CF_TEST_CASE(test_compute) {
    CF_CHECK(compute("abc") == 3);
    CF_REQUIRE(compute("a,b") == 3);
    CF_CHECK(compute("a\"b") == 3);
}

int macro_features() {
    int total = CF_MULTI(1,
                         2);
    total += foobar();
    const char* text = CF_EXPECT_STR(1 + 2);
    const char* mark = CF_MARK(hello);
    const char* mode = CF_MODE;
    CF_LOG("level", total);
    return total + (int)text[0] + (int)mark[0] + (int)mode[0] + checks;
}

int foobar() { return 4; }
