#include <stdint.h>
#include <stdio.h>
#include <ctype.h>
#include <string.h>
#include <assert.h>

#include "tokenizer.h"

void tokenizer_set_filename(struct tokenizer *t, const char* fn) {
	t->filename = fn;
}

#define ARRAY_SIZE(X) (sizeof(X)/sizeof(X[0]))

off_t tokenizer_ftello(struct tokenizer *t) {
	return ftello(t->input)-t->getc_buf.buffered;
}

static int tokenizer_ungetc(struct tokenizer *t, int c)
{
	++t->getc_buf.buffered;
	assert(t->getc_buf.buffered<ARRAY_SIZE(t->getc_buf.buf));
	assert(t->getc_buf.cnt > 0);
	--t->getc_buf.cnt;
	assert(t->getc_buf.buf[t->getc_buf.cnt % ARRAY_SIZE(t->getc_buf.buf)] == c);
	return c;
}
static int tokenizer_getc(struct tokenizer *t)
{
	int c;
	if(t->getc_buf.buffered) {
		t->getc_buf.buffered--;
		c = t->getc_buf.buf[(t->getc_buf.cnt) % ARRAY_SIZE(t->getc_buf.buf)];
	} else {
		c = getc(t->input);
		t->getc_buf.buf[t->getc_buf.cnt % ARRAY_SIZE(t->getc_buf.buf)] = c;
	}
	++t->getc_buf.cnt;
	return c;
}

int tokenizer_peek(struct tokenizer *t) {
	if(t->peeking) return t->peek_token.value;
	int ret = tokenizer_getc(t);
	if(ret != EOF) tokenizer_ungetc(t, ret);
	return ret;
}

int tokenizer_peek_token(struct tokenizer *t, struct token *tok) {
	int ret = tokenizer_next(t, tok);
	t->peek_token = *tok;
	t->peeking = 1;
	return ret;
}

void tokenizer_register_custom_token(struct tokenizer*t, int tokentype, const char* str) {
	assert(tokentype >= TT_CUSTOM && tokentype < TT_CUSTOM + MAX_CUSTOM_TOKENS);
	int pos = tokentype - TT_CUSTOM;
	t->custom_tokens[pos] = str;
	if(pos+1 > t->custom_count) t->custom_count = pos+1;
}

const char* tokentype_to_str(enum tokentype tt) {
	switch((unsigned) tt) {
		case TT_IDENTIFIER: return "iden";
		case TT_WIDECHAR_LIT: return "widechar";
		case TT_WIDESTRING_LIT: return "widestring";
		case TT_SQSTRING_LIT: return "single-quoted string";
		case TT_DQSTRING_LIT: return "double-quoted string";
		case TT_ELLIPSIS: return "ellipsis";
		case TT_HEX_INT_LIT: return "hexint";
		case TT_OCT_INT_LIT: return "octint";
		case TT_DEC_INT_LIT: return "decint";
		case TT_FLOAT_LIT: return "float";
		case TT_SEP: return "separator";
		case TT_UNKNOWN: return "unknown";
		case TT_OVERFLOW: return "overflow";
		case TT_EOF: return "eof";
	}
	return "????";
}

static int has_ul_tail(const char *p) {
	char tail[4];
	int tc = 0, c;
	while(tc < 4 ) {
		if(!*p) break;
		c = tolower(*p);
		if(c == 'u' || c == 'l') {
			tail[tc++] = c;
		} else {
			return 0;
		}
		p++;
	}
	if(tc == 1) return 1;
	if(tc == 2) {
		if(!memcmp(tail, "lu", 2)) return 1;
		if(!memcmp(tail, "ul", 2)) return 1;
		if(!memcmp(tail, "ll", 2)) return 1;
	}
	if(tc == 3) {
		if(!memcmp(tail, "llu", 3)) return 1;
		if(!memcmp(tail, "ull", 3)) return 1;
	}
	return 0;
}

static int is_hex_int_literal(const char *s) {
	if(s[0] == '-') s++;
	if(s[0] == '0' && (s[1] == 'x' || s[1] == 'X')) {
		const char* p = s+2;
		while(*p) {
			if(!strchr("0123456789abcdef", tolower(*p))) {
				if(p == s+2) return 0;
				return has_ul_tail(p);
			}
			p++;
		}
		return 1;
	}
	return 0;
}

static int is_plus_or_minus(int c) {
	return c == '-' || c == '+';
}

static int is_dec_int_literal(const char *str) {
	const char *s = str;
	if(is_plus_or_minus(s[0])) s++;
	if(s[0] == '0') {
		if(s[1] == 0) return 1;
		if(isdigit(s[1])) return 0;
	}
	while(*s) {
		if(!isdigit(*s)) {
			if(s > str && (is_plus_or_minus(str[0]) ? s > str+1 : 1)) return has_ul_tail(s);
			else return 0;
		}
		s++;
	}
	return 1;
}

static int is_float_literal(const char *str) {
	const char *s = str;
	if(is_plus_or_minus(s[0])) s++;
	int got_dot = 0, got_e = 0, got_digits = 0;
	while(*s) {
		int l = tolower(*s);
		if(*s == '.') {
			if(got_dot) return 0;
			got_dot = 1;
		} else if(l == 'f') {
			if(s[1] == 0 && (got_dot || got_e) && got_digits) return 1;
			return 0;
		} else if (isdigit(*s)) {
			got_digits = 1;
		} else if(l == 'e') {
			if(!got_digits) return 0;
			s++;
			if(is_plus_or_minus(*s)) s++;
			if(!isdigit(*s)) return 0;
			got_e = 1;
		} else return 0;
		s++;
	}
	if(got_digits && (got_e || got_dot)) return 1;
	return 0;
}

static int is_valid_float_until(const char*s, const char* until) {
	int got_digits = 0, got_dot = 0;
	while(s < until) {
		if(isdigit(*s)) got_digits = 1;
		else if(*s == '.') {
			if(got_dot) return 0;
			got_dot = 1;
		} else return 0;
		++s;
	}
	return got_digits | (got_dot << 1);
}

static int is_oct_int_literal(const char *s) {
	if(s[0] == '-') s++;
	if(s[0] != '0') return 0;
	while(*s) {
		if(!strchr("01234567", *s)) return 0;
		s++;
	}
	return 1;
}

static int is_identifier(const char *s) {
	static const char ascmap[128] = {
	['0'] = 2, ['1'] = 2, ['2'] = 2, ['3'] = 2,
	['4'] = 2, ['5'] = 2, ['6'] = 2, ['7'] = 2,
	['8'] = 2, ['9'] = 2, ['A'] = 1, ['B'] = 1,
	['C'] = 1, ['D'] = 1, ['E'] = 1, ['F'] = 1,
	['G'] = 1, ['H'] = 1, ['I'] = 1, ['J'] = 1,
	['K'] = 1, ['L'] = 1, ['M'] = 1, ['N'] = 1,
	['O'] = 1, ['P'] = 1, ['Q'] = 1, ['R'] = 1,
	['S'] = 1, ['T'] = 1, ['U'] = 1, ['V'] = 1,
	['W'] = 1, ['X'] = 1, ['Y'] = 1, ['Z'] = 1,
	['_'] = 1, ['a'] = 1, ['b'] = 1, ['c'] = 1,
	['d'] = 1, ['e'] = 1, ['f'] = 1, ['g'] = 1,
	['h'] = 1, ['i'] = 1, ['j'] = 1, ['k'] = 1,
	['l'] = 1, ['m'] = 1, ['n'] = 1, ['o'] = 1,
	['p'] = 1, ['q'] = 1, ['r'] = 1, ['s'] = 1,
	['t'] = 1, ['u'] = 1, ['v'] = 1, ['w'] = 1,
	['x'] = 1, ['y'] = 1, ['z'] = 1,
	};
	if((*s) & 128) return 0;
	if(ascmap[(unsigned) *s] != 1) return 0;
	++s;
	while(*s) {
		if((*s) & 128) return 0;
		if(!ascmap[(unsigned) *s])
			return 0;
		s++;
	}
	return 1;
}

static enum tokentype categorize(const char *s) {
	if(is_hex_int_literal(s)) return TT_HEX_INT_LIT;
	if(is_dec_int_literal(s)) return TT_DEC_INT_LIT;
	if(is_oct_int_literal(s)) return TT_OCT_INT_LIT;
	if(is_float_literal(s)) return TT_FLOAT_LIT;
	if(is_identifier(s)) return TT_IDENTIFIER;
	return TT_UNKNOWN;
}


static int is_sep(int c) {
	static const char ascmap[128] = {
		['\t'] = 1, ['\n'] = 1, [' '] = 1, ['!'] = 1,
		['\"'] = 1, ['#'] = 1, ['%'] = 1, ['&'] = 1,
		['\''] = 1, ['('] = 1, [')'] = 1, ['*'] = 1,
		['+'] = 1, [','] = 1, ['-'] = 1, ['.'] = 1,
		['/'] = 1, [':'] = 1, [';'] = 1, ['<'] = 1,
		['='] = 1, ['>'] = 1, ['?'] = 1, ['['] = 1,
		['\\'] = 1, [']'] = 1, ['{'] = 1, ['|'] = 1,
		['}'] = 1, ['~'] = 1, ['^'] = 1,
	};
	return !(c&128) && ascmap[c];
}

static int apply_coords(struct tokenizer *t, struct token* out, char *end, int retval) {
	out->line = t->line;
	uintptr_t len = end - t->buf;
	out->column = t->column - len;
	if(len + 1 >= t->bufsize) {
		out->type = TT_OVERFLOW;
		return 0;
	}
	return retval;
}

static inline char *assign_bufchar(struct tokenizer *t, char *s, int c) {
	t->column++;
	*s = c;
	return s + 1;
}

static int get_string(struct tokenizer *t, char quote_char, struct token* out, int wide) {
	char *s = t->buf+1;
	int escaped = 0;
	char *end = t->buf + t->bufsize - 2;
	while(s < end) {
		int c = tokenizer_getc(t);
		if(c == EOF) {
			out->type = TT_EOF;
			*s = 0;
			return apply_coords(t, out, s, 0);
		}
		if(c == '\\') {
			c = tokenizer_getc(t);
			if(c == '\n') continue;
			tokenizer_ungetc(t, c);
			c = '\\';
		}
		if(c == '\n') {
			if(escaped) {
				escaped = 0;
				continue;
			}
			tokenizer_ungetc(t, c);
			out->type = TT_UNKNOWN;
			s = assign_bufchar(t, s, 0);
			return apply_coords(t, out, s, 0);
		}
		if(!escaped) {
			if(c == quote_char) {
				s = assign_bufchar(t, s, c);
				*s = 0;
				//s = assign_bufchar(t, s, 0);
				if(!wide)
					out->type = (quote_char == '"'? TT_DQSTRING_LIT : TT_SQSTRING_LIT);
				else
					out->type = (quote_char == '"'? TT_WIDESTRING_LIT : TT_WIDECHAR_LIT);
				return apply_coords(t, out, s, 1);
			}
			if(c == '\\') escaped = 1;
		} else {
			escaped = 0;
		}
		s = assign_bufchar(t, s, c);
	}
	t->buf[MAX_TOK_LEN-1] = 0;
	out->type = TT_OVERFLOW;
	return apply_coords(t, out, s, 0);
}

/* if sequence found, next tokenizer call will point after the sequence */
static int sequence_follows(struct tokenizer *t, int c, const char *which)
{
	if(!which || !which[0]) return 0;
	size_t i = 0;
	while(c == which[i]) {
		if(!which[++i]) break;
		c = tokenizer_getc(t);
	}
	if(!which[i]) return 1;
	while(i > 0) {
		tokenizer_ungetc(t, c);
		c = which[--i];
	}
	return 0;
}

int tokenizer_skip_chars(struct tokenizer *t, const char *chars, int *count) {
	assert(!t->peeking);
	int c;
	*count = 0;
	while(1) {
		c = tokenizer_getc(t);
		if(c == EOF) return 0;
		const char *s = chars;
		int match = 0;
		while(*s) {
			if(c==*s) {
				++(*count);
				match = 1;
				break;
			}
			++s;
		}
		if(!match) {
			tokenizer_ungetc(t, c);
			return 1;
		}
	}

}

int tokenizer_read_until(struct tokenizer *t, const char* marker, int stop_at_nl)
{
	int c, marker_is_nl = !strcmp(marker, "\n");
	char *s = t->buf;
	while(1) {
		c = tokenizer_getc(t);
		if(c == EOF) {
			*s = 0;
			return 0;
		}
		if(c == '\n') {
			t->line++;
			t->column = 0;
			if(stop_at_nl) {
				*s = 0;
				if(marker_is_nl) return 1;
				return 0;
			}
		}
		if(!sequence_follows(t, c, marker))
			s = assign_bufchar(t, s, c);
		else
			break;
	}
	*s = 0;
	size_t i;
	for(i=strlen(marker); i > 0; )
		tokenizer_ungetc(t, marker[--i]);
	return 1;
}
static int ignore_until(struct tokenizer *t, const char* marker, int col_advance)
{
	t->column += col_advance;
	int c;
	do {
		c = tokenizer_getc(t);
		if(c == EOF) return 0;
		if(c == '\n') {
			t->line++;
			t->column = 0;
		} else t->column++;
	} while(!sequence_follows(t, c, marker));
	t->column += strlen(marker)-1;
	return 1;
}

void tokenizer_skip_until(struct tokenizer *t, const char *marker)
{
	ignore_until(t, marker, 0);
}

int tokenizer_next(struct tokenizer *t, struct token* out) {
	char *s = t->buf;
	out->value = 0;
	int c = 0;
	if(t->peeking) {
		*out = t->peek_token;
		t->peeking = 0;
		return 1;
	}
	while(1) {
		c = tokenizer_getc(t);
		if(c == EOF) break;

		/* components of multi-line comment marker might be terminals themselves */
		if(sequence_follows(t, c, t->marker[MT_MULTILINE_COMMENT_START])) {
			ignore_until(t, t->marker[MT_MULTILINE_COMMENT_END], strlen(t->marker[MT_MULTILINE_COMMENT_START]));
			continue;
		}
		if(sequence_follows(t, c, t->marker[MT_SINGLELINE_COMMENT_START])) {
			ignore_until(t, "\n", strlen(t->marker[MT_SINGLELINE_COMMENT_START]));
			continue;
		}
		if(is_sep(c)) {
			if(s != t->buf && c == '\\' && !isspace(s[-1])) {
				c = tokenizer_getc(t);
				if(c == '\n') continue;
				tokenizer_ungetc(t, c);
				c = '\\';
			} else if(is_plus_or_minus(c) && s > t->buf+1 &&
				  (s[-1] == 'E' || s[-1] == 'e') && is_valid_float_until(t->buf, s-1)) {
				goto process_char;
			} else if(c == '.' && s != t->buf && is_valid_float_until(t->buf, s) == 1) {
				goto process_char;
			} else if(c == '.' && s == t->buf) {
				int jump = 0;
				c = tokenizer_getc(t);
				if(isdigit(c)) jump = 1;
				tokenizer_ungetc(t, c);
				c = '.';
				if(jump) goto process_char;
			}
			tokenizer_ungetc(t, c);
			break;
		}
		if((t->flags & TF_PARSE_WIDE_STRINGS) && s == t->buf && c == 'L') {
			c = tokenizer_getc(t);
			tokenizer_ungetc(t, c);
			tokenizer_ungetc(t, 'L');
			if(c == '\'' || c == '\"') break;
		}

process_char:;
		s = assign_bufchar(t, s, c);
		if(t->column + 1 >= MAX_TOK_LEN) {
			out->type = TT_OVERFLOW;
			return apply_coords(t, out, s, 0);
		}
	}
	if(s == t->buf) {
		if(c == EOF) {
			out->type = TT_EOF;
			return apply_coords(t, out, s, 1);
		}

		int wide = 0;
		c = tokenizer_getc(t);
		if((t->flags & TF_PARSE_WIDE_STRINGS) && c == 'L') {
			c = tokenizer_getc(t);
			assert(c == '\'' || c == '\"');
			wide = 1;
			goto string_handling;
		} else if (c == '.' && sequence_follows(t, c, "...")) {
			strcpy(t->buf, "...");
			out->type = TT_ELLIPSIS;
			return apply_coords(t, out, s+3, 1);
		}

		{
			int i;
			for(i = 0; i < t->custom_count; i++)
				if(sequence_follows(t, c, t->custom_tokens[i])) {
					const char *p = t->custom_tokens[i];
					while(*p) {
						s = assign_bufchar(t, s, *p);
						p++;
					}
					*s = 0;
					out->type = TT_CUSTOM + i;
					return apply_coords(t, out, s, 1);
				}
		}

string_handling:
		s = assign_bufchar(t, s, c);
		*s = 0;
		//s = assign_bufchar(t, s, 0);
		if(c == '"' || c == '\'')
			if(t->flags & TF_PARSE_STRINGS) return get_string(t, c, out, wide);
		out->type = TT_SEP;
		out->value = c;
		if(c == '\n') {
			apply_coords(t, out, s, 1);
			t->line++;
			t->column=0;
			return 1;
		}
		return apply_coords(t, out, s, 1);
	}
	//s = assign_bufchar(t, s, 0);
	*s = 0;
	out->type = categorize(t->buf);
	return apply_coords(t, out, s, out->type != TT_UNKNOWN);
}

void tokenizer_set_flags(struct tokenizer *t, int flags) {
	t->flags = flags;
}

int tokenizer_get_flags(struct tokenizer *t) {
	return t->flags;
}

void tokenizer_init(struct tokenizer *t, FILE* in, int flags) {
	*t = (struct tokenizer){ .input = in, .line = 1, .flags = flags, .bufsize = MAX_TOK_LEN};
}

void tokenizer_register_marker(struct tokenizer *t, enum markertype mt, const char* marker)
{
	t->marker[mt] = marker;
}

int tokenizer_rewind(struct tokenizer *t) {
	FILE *f = t->input;
	int flags = t->flags;
	const char* fn = t->filename;
	tokenizer_init(t, f, flags);
	tokenizer_set_filename(t, fn);
	return fseek(f, 0, SEEK_SET) == 0;
}
