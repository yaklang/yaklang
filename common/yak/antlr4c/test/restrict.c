#include <stdio.h>

#if defined(__GNUC__) || defined(__clang__)
#define ALIGNED(a) __attribute__((aligned(a)))
#define UNUSED __attribute__((unused))
#else
#define ALIGNED(a)
#define UNUSED
#endif

typedef struct
{
    unsigned char *ptr;      /* current data position */
    unsigned char *data;     /* Start of the data */
    unsigned char *data_end; /* End of the data */
} parser_t;

static inline char *parse_hash(parser_t *restrict p UNUSED,
                               char *ch UNUSED)
{
    skip_macros(p);
    return 0;
}
