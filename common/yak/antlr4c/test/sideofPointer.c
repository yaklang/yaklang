#include <stdio.h>

typedef struct hash_entry
{
    struct hash_entry *next;
    char *filename; /* File where the word was found */
    char token[0];  /* Flexible array member for token data */
} hash_entry_t;

#define TABLE_SIZE (4 * 16384)
static hash_entry_t *hash_bad_spellings[TABLE_SIZE];

static inline void add_bad_spelling(const char *word,
                                    const size_t len,
                                    const char *filename)
{
    hash_entry_t **head = &hash_bad_spellings[stress_hash_mulxror64(word, len)];
    hash_entry_t *he;

    for (he = *head; he; he = he->next)
    {
        if (!strcmp(he->token, word))
            return;
    }

    size_t filename_len = strlen(filename) + 1;
    he = malloc(sizeof(*he) + len + filename_len);
    if (UNLIKELY(!he))
        out_of_memory();

    he->next = *head;
    *head = he;
    he->filename = he->token + len;
    memcpy(he->filename, filename, filename_len);
    memcpy(he->token, word, len);
}