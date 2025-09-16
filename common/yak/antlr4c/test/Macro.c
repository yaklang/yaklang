#if defined(__GNUC__) || defined(__clang__)
#define ALIGNED(a) __attribute__((aligned(a)))
#define UNUSED __attribute__((unused))
#else
#define ALIGNED(a)
#define UNUSED
#endif

#include <stdbool.h>

static bool is_not_whitespace[256] ALIGNED(64);
static bool is_not_identifier[256] ALIGNED(64);