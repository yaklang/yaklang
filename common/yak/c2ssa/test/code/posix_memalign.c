#include <stdio.h>
#define TOKEN_CHUNK_SIZE (32768)

typedef struct
{
    char *ptr;       /* the current end of the token during lexical analysis */
    char *token;     /* Collected token string */
    char *token_end; /* Pointer to the end of the token */
    size_t len;      /* Buffer length for the token */
} token_t;

static void token_new(token_t *t)
{
    int ret = posix_memalign((void **)&t->token, 64, TOKEN_CHUNK_SIZE);
    if (ret != 0)
        out_of_memory();
    t->len = TOKEN_CHUNK_SIZE;
    token_clear(t);
}