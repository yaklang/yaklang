#ifndef TOKENIZER_H
#define TOKENIZER_H

#define MAX_TOK_LEN 4096
#define MAX_UNGETC 8

#include <stdint.h>
#include <stddef.h>
#include <stdio.h>

struct tokenizer_getc_buf {
	int buf[MAX_UNGETC];
	size_t cnt, buffered;
};

enum markertype {
	MT_SINGLELINE_COMMENT_START = 0,
	MT_MULTILINE_COMMENT_START = 1,
	MT_MULTILINE_COMMENT_END = 2,
	MT_MAX = MT_MULTILINE_COMMENT_END
};

#define MAX_CUSTOM_TOKENS 32

enum tokentype {
	TT_IDENTIFIER = 1,
	TT_SQSTRING_LIT,
	TT_DQSTRING_LIT,
	TT_ELLIPSIS,
	TT_HEX_INT_LIT,
	TT_OCT_INT_LIT,
	TT_DEC_INT_LIT,
	TT_FLOAT_LIT,
	TT_SEP,
	/* errors and similar */
	TT_UNKNOWN,
	TT_OVERFLOW,
	TT_WIDECHAR_LIT,
	TT_WIDESTRING_LIT,
	TT_EOF,
	TT_CUSTOM = 1000 /* start user defined tokentype values */
};

const char* tokentype_to_str(enum tokentype tt);

struct token {
	enum tokentype type;
	uint32_t line;
	uint32_t column;
	int value;
};

enum tokenizer_flags {
	TF_PARSE_STRINGS = 1 << 0,
	TF_PARSE_WIDE_STRINGS = 1 << 1,
};

struct tokenizer {
	FILE *input;
	uint32_t line;
	uint32_t column;
	int flags;
	int custom_count;
	int peeking;
	const char *custom_tokens[MAX_CUSTOM_TOKENS];
	char buf[MAX_TOK_LEN];
	size_t bufsize;
	struct tokenizer_getc_buf getc_buf;
	const char* marker[MT_MAX+1];
	const char* filename;
	struct token peek_token;
};

void tokenizer_init(struct tokenizer *t, FILE* in, int flags);
void tokenizer_set_filename(struct tokenizer *t, const char*);
void tokenizer_set_flags(struct tokenizer *t, int flags);
int tokenizer_get_flags(struct tokenizer *t);
off_t tokenizer_ftello(struct tokenizer *t);
void tokenizer_register_marker(struct tokenizer*, enum markertype, const char*);
void tokenizer_register_custom_token(struct tokenizer*, int tokentype, const char*);
int tokenizer_next(struct tokenizer *t, struct token* out);
int tokenizer_peek_token(struct tokenizer *t, struct token* out);
int tokenizer_peek(struct tokenizer *t);
void tokenizer_skip_until(struct tokenizer *t, const char *marker);
int tokenizer_skip_chars(struct tokenizer *t, const char *chars, int *count);
int tokenizer_read_until(struct tokenizer *t, const char* marker, int stop_at_nl);
int tokenizer_rewind(struct tokenizer *t);

#ifdef __GNUC__
#pragma GCC diagnostic ignored "-Wunknown-pragmas"
#endif
#pragma RcB2 DEP "tokenizer.c"

#endif

