#pragma once

// Minimal test-framework macros. They mirror the doctest/Catch2 shapes that
// real headers use: token pasting, stringification, variadic arguments,
// multi-line macro bodies, a trailing line comment, and conditional defines.
#define CF_CAT_IMPL(a, b) a##b
#define CF_CAT(a, b) CF_CAT_IMPL(a, b)
#define CF_STRINGIFY(...) #__VA_ARGS__

#define CF_TEST_CASE(name) \
    static void name(); \
    static const int CF_CAT(name, _reg) = cf_register(&name, CF_STRINGIFY(name)); \
    static void name()

#define CF_CHECK(expr) \
    do { \
        cf_check(#expr, (expr) ? 1 : 0); \
    } while (false) // NOLINT(macro-comment)

#define CF_REQUIRE(expr) CF_CHECK(expr)

#define CF_EXPECT_STR(...) CF_STRINGIFY(__VA_ARGS__) "_tail"

#if defined(CF_USE_FAST)
#define CF_MODE "fast"
#elif defined(CF_USE_SMALL)
#define CF_MODE "small"
#else
#define CF_MODE "default"
#endif

#define CF_MULTI(a, b) \
    ((a) + \
     (b))

#define CF_MARK(x) #x##_mark

#define CF_LOG(level, ...) cf_log(level, __VA_ARGS__)
