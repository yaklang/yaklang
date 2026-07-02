#include <string.h>
#include <ctype.h>
#include <assert.h>
#include "preproc.h"
#include "tokenizer.h"
#include "tglist.h"
#include "hbmap.h"

#define MACRO_FLAG_OBJECTLIKE (1U<<31)
#define MACRO_FLAG_VARIADIC (1U<<30)
#define MACRO_ARGCOUNT_MASK (~(0|MACRO_FLAG_OBJECTLIKE|MACRO_FLAG_VARIADIC))

#define OBJECTLIKE(M) (M->num_args & MACRO_FLAG_OBJECTLIKE)
#define FUNCTIONLIKE(M) (!(OBJECTLIKE(M)))
#define MACRO_ARGCOUNT(M) (M->num_args & MACRO_ARGCOUNT_MASK)
#define MACRO_VARIADIC(M) (M->num_args & MACRO_FLAG_VARIADIC)

#define MAX_RECURSION 32

static unsigned string_hash(const char* s) {
	uint_fast32_t h = 0;
	while (*s) {
		h = 16*h + *s++;
		h ^= h>>24 & 0xf0;
	}
	return h & 0xfffffff;
}

struct macro {
	unsigned num_args;
	FILE* str_contents;
	char *str_contents_buf;
	tglist(char*) argnames;
};

struct cpp {
	tglist(char*) includedirs;
	hbmap(char*, struct macro, 128) *macros;
	const char *last_file;
	int last_line;
	struct tokenizer *tchain[MAX_RECURSION];
};

static int token_needs_string(struct token *tok) {
	switch(tok->type) {
		case TT_IDENTIFIER:
		case TT_WIDECHAR_LIT:
		case TT_WIDESTRING_LIT:
		case TT_SQSTRING_LIT:
		case TT_DQSTRING_LIT:
                case TT_ELLIPSIS:
                case TT_HEX_INT_LIT:
                case TT_OCT_INT_LIT:
                case TT_DEC_INT_LIT:
		case TT_FLOAT_LIT:
		case TT_UNKNOWN:
			return 1;
		default:
			return 0;
	}
}

static void tokenizer_from_file(struct tokenizer *t, FILE* f) {
	tokenizer_init(t, f, TF_PARSE_STRINGS);
	tokenizer_set_filename(t, "<macro>");
	tokenizer_rewind(t);
}

static int strptrcmp(const void *a, const void *b) {
	const char * const *x = a;
	const char * const *y = b;
	return strcmp(*x, *y);
}

static struct macro* get_macro(struct cpp *cpp, const char *name) {
	return hbmap_get(cpp->macros, name);
}

static void add_macro(struct cpp *cpp, const char *name, struct macro*m) {
	hbmap_insert(cpp->macros, name, *m);
}

static int undef_macro(struct cpp *cpp, const char *name) {
	hbmap_iter k = hbmap_find(cpp->macros, name);
	if(k == (hbmap_iter) -1) return 0;
	struct macro *m = &hbmap_getval(cpp->macros, k);
	free(hbmap_getkey(cpp->macros, k));
	if(m->str_contents) fclose(m->str_contents);
	free(m->str_contents_buf);
	tglist_free_values(&m->argnames);
	tglist_free_items(&m->argnames);
	hbmap_delete(cpp->macros, k);
	return 1;
}

static void free_macros(struct cpp *cpp) {
	hbmap_iter i;
	hbmap_foreach(cpp->macros, i) {
		while(hbmap_iter_index_valid(cpp->macros, i))
			undef_macro(cpp, hbmap_getkey(cpp->macros, i));
	}
	hbmap_fini(cpp->macros, 1);
	free(cpp->macros);
}

static void error_or_warning(const char *err, const char* type, struct tokenizer *t, struct token *curr) {
	unsigned column = curr ? curr->column : t->column;
	unsigned line  = curr ? curr->line : t->line;
	dprintf(2, "<%s> %u:%u %s: '%s'\n", t->filename, line, column, type, err);
	dprintf(2, "%s\n", t->buf);
	for(int i = 0; i < strlen(t->buf); i++)
		dprintf(2, "^");
	dprintf(2, "\n");
}
static void error(const char *err, struct tokenizer *t, struct token *curr) {
	error_or_warning(err, "error", t, curr);
}
static void warning(const char *err, struct tokenizer *t, struct token *curr) {
	error_or_warning(err, "warning", t, curr);
}

static void emit(FILE *out, const char *s) {
	fprintf(out, "%s", s);
}

static int x_tokenizer_next_of(struct tokenizer *t, struct token *tok, int fail_unk) {
	int ret = tokenizer_next(t, tok);
	if(tok->type == TT_OVERFLOW) {
		error("max token length of 4095 exceeded!", t, tok);
		return 0;
	} else if (fail_unk && ret == 0) {
		error("tokenizer encountered unknown token", t, tok);
		return 0;
	}
	return 1;
}

#define tokenizer_next(T, TOK) x_tokenizer_next_of(T, TOK, 0)
#define x_tokenizer_next(T, TOK) x_tokenizer_next_of(T, TOK, 1)

static int is_whitespace_token(struct token *token)
{
	return token->type == TT_SEP &&
		(token->value == ' ' || token->value == '\t');
}

/* return index of matching item in values array, or -1 on error */
static int expect(struct tokenizer *t, enum tokentype tt, const char* values[], struct token *token)
{
	int ret;
	do {
		ret = tokenizer_next(t, token);
		if(ret == 0 || token->type == TT_EOF) goto err;
	} while(is_whitespace_token(token));

	if(token->type != tt) {
err:
		error("unexpected token", t, token);
		return -1;
	}
	int i = 0;
	while(values[i]) {
		if(!strcmp(values[i], t->buf))
			return i;
		++i;
	}
	return -1;
}

static int is_char(struct token *tok, int ch) {
	return tok->type == TT_SEP && tok->value == ch;
}

static void flush_whitespace(FILE *out, int *ws_count) {
	while(*ws_count > 0) {
		emit(out, " ");
		--(*ws_count);
	}
}

/* skips until the next non-whitespace token (if the current one is one too)*/
static int eat_whitespace(struct tokenizer *t, struct token *token, int *count) {
	*count = 0;
	int ret = 1;
	while (is_whitespace_token(token)) {
		++(*count);
		ret = x_tokenizer_next(t, token);
		if(!ret) break;
	}
	return ret;
}
/* fetches the next token until it is non-whitespace */
static int skip_next_and_ws(struct tokenizer *t, struct token *tok) {
	int ret = tokenizer_next(t, tok);
	if(!ret) return ret;
	int ws_count;
	ret = eat_whitespace(t, tok, &ws_count);
	return ret;
}

static void emit_token(FILE* out, struct token *tok, const char* strbuf) {
	if(tok->type == TT_SEP) {
		fprintf(out, "%c", tok->value);
	} else if(strbuf && token_needs_string(tok)) {
		fprintf(out, "%s", strbuf);
	} else {
		dprintf(2, "oops, dunno how to handle tt %d (%s)\n", (int) tok->type, strbuf);
	}
}

int parse_file(struct cpp* cpp, FILE *f, const char*, FILE *out);
static int include_file(struct cpp* cpp, struct tokenizer *t, FILE* out) {
	static const char* inc_chars[] = { "\"", "<", 0};
	static const char* inc_chars_end[] = { "\"", ">", 0};
	struct token tok;
	tokenizer_set_flags(t, 0); // disable string tokenization

	int inc1sep = expect(t, TT_SEP, inc_chars, &tok);
	if(inc1sep == -1) {
		error("expected one of [\"<]", t, &tok);
		return 0;
	}
	int ret = tokenizer_read_until(t, inc_chars_end[inc1sep], 1);
	if(!ret) {
		error("error parsing filename", t, &tok);
		return 0;
	}
	// TODO: different path lookup depending on whether " or <
	size_t i;
	FILE *f = 0;
	tglist_foreach(&cpp->includedirs, i) {
		char buf[512];
		snprintf(buf, sizeof buf, "%s/%s", tglist_get(&cpp->includedirs, i), t->buf);
		f = fopen(buf, "r");
		if(f) break;
	}
	if(!f) {
		dprintf(2, "%s: ", t->buf);
		perror("fopen");
		return 0;
	}
	const char *fn = strdup(t->buf);
	assert(tokenizer_next(t, &tok) && is_char(&tok, inc_chars_end[inc1sep][0]));

	tokenizer_set_flags(t, TF_PARSE_STRINGS);
	return parse_file(cpp, f, fn, out);
}

static int emit_error_or_warning(struct tokenizer *t, int is_error) {
	int ws_count;
	int ret = tokenizer_skip_chars(t, " \t", &ws_count);
	if(!ret) return ret;
	struct token tmp = {.column = t->column, .line = t->line};
	ret = tokenizer_read_until(t, "\n", 1);
	if(is_error) {
		error(t->buf, t, &tmp);
		return 0;
	}
	warning(t->buf, t, &tmp);
	return 1;
}

static FILE *freopen_r(FILE *f, char **buf, size_t *size) {
	fflush(f);
	fclose(f);
	return fmemopen(*buf, *size, "r");
}

static int consume_nl_and_ws(struct tokenizer *t, struct token *tok, int expected) {
	if(!x_tokenizer_next(t, tok)) {
err:
		error("unexpected", t, tok);
		return 0;
	}
	if(expected) {
		if(tok->type != TT_SEP || tok->value != expected) goto err;
		switch(expected) {
			case '\\' : expected = '\n'; break;
			case '\n' : expected = 0; break;
		}
	} else {
		if(is_whitespace_token(tok)) ;
		else if(is_char(tok, '\\')) expected = '\n';
		else return 1;
	}
	return consume_nl_and_ws(t, tok, expected);
}

static int expand_macro(struct cpp *cpp, struct tokenizer *t, FILE* out, const char* name, unsigned rec_level, char *visited[]);

static int parse_macro(struct cpp *cpp, struct tokenizer *t) {
	int ws_count;
	int ret = tokenizer_skip_chars(t, " \t", &ws_count);
	if(!ret) return ret;
	struct token curr; //tmp = {.column = t->column, .line = t->line};
	ret = tokenizer_next(t, &curr) && curr.type != TT_EOF;
	if(!ret) {
		error("parsing macro name", t, &curr);
		return ret;
	}
	if(curr.type != TT_IDENTIFIER) {
		error("expected identifier", t, &curr);
		return 0;
	}
	const char* macroname = strdup(t->buf);
#ifdef DEBUG
	dprintf(2, "parsing macro %s\n", macroname);
#endif
	int redefined = 0;
	if(get_macro(cpp, macroname)) {
		if(!strcmp(macroname, "defined")) {
			error("\"defined\" cannot be used as a macro name", t, &curr);
			return 0;
		}
		redefined = 1;
	}

	struct macro new = { 0 };
	unsigned macro_flags = MACRO_FLAG_OBJECTLIKE;
	tglist_init(&new.argnames);

	ret = x_tokenizer_next(t, &curr) && curr.type != TT_EOF;
	if(!ret) return ret;

	if (is_char(&curr, '(')) {
		macro_flags = 0;
		unsigned expected = 0;
		while(1) {
			/* process next function argument identifier */
			ret = consume_nl_and_ws(t, &curr, expected);
			if(!ret) {
				error("unexpected", t, &curr);
				return ret;
			}
			expected = 0;
			if(curr.type == TT_SEP) {
				switch(curr.value) {
				case '\\':
					expected = '\n';
					continue;
				case ',':
					continue;
				case ')':
					ret = tokenizer_skip_chars(t, " \t", &ws_count);
					if(!ret) return ret;
					goto break_loop1;
				default:
					error("unexpected character", t, &curr);
					return 0;
				}
			} else if(!(curr.type == TT_IDENTIFIER || curr.type == TT_ELLIPSIS)) {
				error("expected identifier for macro arg", t, &curr);
				return 0;
			}
			{
				if(curr.type == TT_ELLIPSIS) {
					if(macro_flags & MACRO_FLAG_VARIADIC) {
						error("\"...\" isn't the last parameter", t, &curr);
						return 0;
					}
					macro_flags |= MACRO_FLAG_VARIADIC;
				}
				char *tmps = strdup(t->buf);
				tglist_add(&new.argnames, tmps);
			}
			++new.num_args;
		}
		break_loop1:;
	} else if(is_whitespace_token(&curr)) {
		ret = tokenizer_skip_chars(t, " \t", &ws_count);
		if(!ret) return ret;
	} else if(is_char(&curr, '\n')) {
		/* content-less macro */
		goto done;
	}

	struct FILE_container {
		FILE *f;
		char *buf;
		size_t len;
        } contents;
	contents.f = open_memstream(&contents.buf, &contents.len);

	int backslash_seen = 0;
	while(1) {
		/* ignore unknown tokens in macro body */
		ret = tokenizer_next(t, &curr);
		if(!ret) return 0;
		if(curr.type == TT_EOF) break;
		if (curr.type == TT_SEP) {
			if(curr.value == '\\')
				backslash_seen = 1;
			else {
				if(curr.value == '\n' && !backslash_seen) break;
				emit_token(contents.f, &curr, t->buf);
				backslash_seen = 0;
			}
		} else {
			emit_token(contents.f, &curr, t->buf);
		}
	}
	new.str_contents = freopen_r(contents.f, &contents.buf, &contents.len);
	new.str_contents_buf = contents.buf;
done:
	if(redefined) {
		struct macro *old = get_macro(cpp, macroname);
		char *s_old = old->str_contents_buf ? old->str_contents_buf : "";
		char *s_new = new.str_contents_buf ? new.str_contents_buf : "";
		if(strcmp(s_old, s_new)) {
			char buf[128];
			sprintf(buf, "redefinition of macro %s", macroname);
			warning(buf, t, 0);
		}
	}
	new.num_args |= macro_flags;
	add_macro(cpp, macroname, &new);
	return 1;
}

static size_t macro_arglist_pos(struct macro *m, const char* iden) {
	size_t i;
	for(i = 0; i < tglist_getsize(&m->argnames); i++) {
		char *item = tglist_get(&m->argnames, i);
		if(!strcmp(item, iden)) return i;
	}
	return (size_t) -1;
}


struct macro_info {
	const char *name;
	unsigned nest;
	unsigned first;
	unsigned last;
};

static int was_visited(const char *name, char*visited[], unsigned rec_level) {
	int x;
	for(x = rec_level; x >= 0; --x) {
		if(!strcmp(visited[x], name)) return 1;
	}
	return 0;
}

unsigned get_macro_info(struct cpp* cpp,
	struct tokenizer *t,
	struct macro_info *mi_list, size_t *mi_cnt,
	unsigned nest, unsigned tpos, const char *name,
	char* visited[], unsigned rec_level
	) {
	int brace_lvl = 0;
	while(1) {
		struct token tok;
		int ret = tokenizer_next(t, &tok);
		if(!ret || tok.type == TT_EOF) break;
#ifdef DEBUG
		dprintf(2, "(%s) nest %d, brace %u t: %s\n", name, nest, brace_lvl, t->buf);
#endif
		struct macro* m = 0;
		if(tok.type == TT_IDENTIFIER && (m = get_macro(cpp, t->buf)) && !was_visited(t->buf, visited, rec_level)) {
			const char* newname = strdup(t->buf);
			if(FUNCTIONLIKE(m)) {
				if(tokenizer_peek(t) == '(') {
					unsigned tpos_save = tpos;
					tpos = get_macro_info(cpp, t, mi_list, mi_cnt, nest+1, tpos+1, newname, visited, rec_level);
					mi_list[*mi_cnt] = (struct macro_info) {
						.name = newname,
						.nest=nest+1,
						.first = tpos_save,
						.last = tpos + 1};
					++(*mi_cnt);
				} else {
					/* suppress expansion */
				}
			} else {
				mi_list[*mi_cnt] = (struct macro_info) {
					.name = newname,
					.nest=nest+1,
					.first = tpos,
					.last = tpos + 1};
				++(*mi_cnt);
			}
		} else if(is_char(&tok, '(')) {
			++brace_lvl;
		} else if(is_char(&tok, ')')) {
			--brace_lvl;
			if(brace_lvl == 0 && nest != 0) break;
		}
		++tpos;
	}
	return tpos;
}

struct FILE_container {
	FILE *f;
	char *buf;
	size_t len;
	struct tokenizer t;
};

static void free_file_container(struct FILE_container *fc) {
	fclose(fc->f);
	free(fc->buf);
}

static int mem_tokenizers_join(
	struct FILE_container* org, struct FILE_container *inj,
	struct FILE_container* result,
	int first, off_t lastpos) {
	result->f = open_memstream(&result->buf, &result->len);
	size_t i;
	struct token tok;
	int ret;
	tokenizer_rewind(&org->t);
	for(i=0; i<first; ++i) {
		ret = tokenizer_next(&org->t, &tok);
		assert(ret && tok.type != TT_EOF);
		emit_token(result->f, &tok, org->t.buf);
	}
	int cnt = 0, last = first;
	while(1) {
		ret = tokenizer_next(&inj->t, &tok);
		if(!ret || tok.type == TT_EOF) break;
		emit_token(result->f, &tok, inj->t.buf);
		++cnt;
	}
	while(tokenizer_ftello(&org->t) < lastpos) {
		ret = tokenizer_next(&org->t, &tok);
		last++;
	}

	int diff = cnt - ((int) last - (int) first);

	while(1) {
		ret = tokenizer_next(&org->t, &tok);
		if(!ret || tok.type == TT_EOF) break;
		emit_token(result->f, &tok, org->t.buf);
	}

	result->f = freopen_r(result->f, &result->buf, &result->len);
	tokenizer_from_file(&result->t, result->f);
	return diff;
}

static int tchain_parens_follows(struct cpp *cpp, int rec_level) {
	int i, c = 0;
	for(i=rec_level;i>=0;--i) {
		c = tokenizer_peek(cpp->tchain[i]);
		if(c == EOF) continue;
		if(c == '(') return i;
		else break;
	}
	return -1;
}

static int stringify(struct cpp *ccp, struct tokenizer *t, FILE* output) {
	int ret = 1;
	struct token tok;
	emit(output, "\"");
	while(1) {
		ret = tokenizer_next(t, &tok);
		if(!ret) return ret;
		if(tok.type == TT_EOF) break;
		if(is_char(&tok, '\n')) continue;
		if(is_char(&tok, '\\') && tokenizer_peek(t) == '\n') continue;
		if(tok.type == TT_DQSTRING_LIT) {
			char *s = t->buf;
			char buf[2] = {0};
			while(*s) {
				if(*s == '\"') {
					emit(output, "\\\"");
				} else if (*s == '\\') {
					emit(output, "\\\\");
				} else {
					buf[0] = *s;
					emit(output, buf);
				}
				++s;
			}
		} else
			emit_token(output, &tok, t->buf);
	}
	emit(output, "\"");
	return ret;
}

/* rec_level -1 serves as a magic value to signal we're using
   expand_macro from the if-evaluator code, which means activating
   the "define" macro */
static int expand_macro(struct cpp* cpp, struct tokenizer *t, FILE* out, const char* name, unsigned rec_level, char* visited[]) {
	int is_define = !strcmp(name, "defined");

	struct macro *m;
	if(is_define && rec_level != -1)
		m = NULL;
	else m = get_macro(cpp, name);
	if(!m) {
		emit(out, name);
		return 1;
	}
	if(rec_level == -1) rec_level = 0;
	if(rec_level >= MAX_RECURSION) {
		error("max recursion level reached", t, 0);
		return 0;
	}
#ifdef DEBUG
	dprintf(2, "lvl %u: expanding macro %s (%s)\n", rec_level, name, m->str_contents_buf);
#endif

	if(rec_level == 0 && strcmp(t->filename, "<macro>")) {
		cpp->last_file = t->filename;
		cpp->last_line = t->line;
	}
	if(!strcmp(name, "__FILE__")) {
		emit(out, "\"");
		emit(out, cpp->last_file);
		emit(out, "\"");
		return 1;
	} else if(!strcmp(name, "__LINE__")) {
		char buf[64];
		sprintf(buf, "%d", cpp->last_line);
		emit(out, buf);
		return 1;
	}

	if(visited[rec_level]) free(visited[rec_level]);
	visited[rec_level] = strdup(name);
	cpp->tchain[rec_level] = t;

	size_t i;
	struct token tok;
	unsigned num_args = MACRO_ARGCOUNT(m);
	struct FILE_container *argvalues = calloc(MACRO_VARIADIC(m) ? num_args + 1 : num_args, sizeof(struct FILE_container));

	for(i=0; i < num_args; i++)
		argvalues[i].f = open_memstream(&argvalues[i].buf, &argvalues[i].len);

	/* replace named arguments in the contents of the macro call */
	if(FUNCTIONLIKE(m)) {
		int ret;
		if((ret = tokenizer_peek(t)) != '(') {
			/* function-like macro shall not be expanded if not followed by '(' */
			if(ret == EOF && rec_level > 0 && (ret = tchain_parens_follows(cpp, rec_level-1)) != -1) {
				// warning("Replacement text involved subsequent text", t, 0);
				t = cpp->tchain[ret];
			} else {
				emit(out, name);
				goto cleanup;
			}
		}
		ret = x_tokenizer_next(t, &tok);
		assert(ret && is_char(&tok, '('));

		unsigned curr_arg = 0, need_arg = 1, parens = 0;
		int ws_count;
		if(!tokenizer_skip_chars(t, " \t", &ws_count)) return 0;

		int varargs = 0;
		if(num_args == 1 && MACRO_VARIADIC(m)) varargs = 1;
		while(1) {
			int ret = tokenizer_next(t, &tok);
			if(!ret) return 0;
			if( tok.type == TT_EOF) {
				dprintf(2, "warning EOF\n");
				break;
			}
			if(!parens && is_char(&tok, ',') && !varargs) {
				if(need_arg && !ws_count) {
					/* empty argument is OK */
				}
				need_arg = 1;
				if(!varargs) curr_arg++;
				if(curr_arg + 1 == num_args && MACRO_VARIADIC(m)) {
					varargs = 1;
				} else if(curr_arg >= num_args) {
					error("too many arguments for function macro", t, &tok);
					return 0;
				}
				ret = tokenizer_skip_chars(t, " \t", &ws_count);
				if(!ret) return ret;
				continue;
			} else if(is_char(&tok, '(')) {
				++parens;
			} else if(is_char(&tok, ')')) {
				if(!parens) {
					if(curr_arg + num_args && curr_arg < num_args-1) {
						error("too few args for function macro", t, &tok);
						return 0;
					}
					break;
				}
				--parens;
			} else if(is_char(&tok, '\\')) {
				if(tokenizer_peek(t) == '\n') continue;
			}
			need_arg = 0;
			emit_token(argvalues[curr_arg].f, &tok, t->buf);
		}
	}

	for(i=0; i < num_args; i++) {
		argvalues[i].f = freopen_r(argvalues[i].f, &argvalues[i].buf, &argvalues[i].len);
		tokenizer_from_file(&argvalues[i].t, argvalues[i].f);
#ifdef DEBUG
		dprintf(2, "macro argument %i: %s\n", (int) i, argvalues[i].buf);
#endif
	}

	if(is_define) {
		if(get_macro(cpp, argvalues[0].buf))
			emit(out, "1");
		else
			emit(out, "0");
	}

	if(!m->str_contents) goto cleanup;

	struct FILE_container cwae = {0}; /* contents_with_args_expanded */
	cwae.f = open_memstream(&cwae.buf, &cwae.len);
	FILE* output = cwae.f;

	struct tokenizer t2;
	tokenizer_from_file(&t2, m->str_contents);
	int hash_count = 0;
	int ws_count = 0;
	while(1) {
		int ret;
		ret = tokenizer_next(&t2, &tok);
		if(!ret) return 0;
		if(tok.type == TT_EOF) break;
		if(tok.type == TT_IDENTIFIER) {
			flush_whitespace(output, &ws_count);
			char *id = t2.buf;
			if(MACRO_VARIADIC(m) && !strcmp(t2.buf, "__VA_ARGS__")) {
				id = "...";
			}
			size_t arg_nr = macro_arglist_pos(m, id);
			if(arg_nr != (size_t) -1) {
				tokenizer_rewind(&argvalues[arg_nr].t);
				if(hash_count == 1) ret = stringify(cpp, &argvalues[arg_nr].t, output);
				else while(1) {
					ret = tokenizer_next(&argvalues[arg_nr].t, &tok);
					if(!ret) return ret;
					if(tok.type == TT_EOF) break;
					emit_token(output, &tok, argvalues[arg_nr].t.buf);
				}
				hash_count = 0;
			} else {
				if(hash_count == 1) {
		hash_err:
					error("'#' is not followed by macro parameter", &t2, &tok);
					return 0;
				}
				emit_token(output, &tok, t2.buf);
			}
		} else if(is_char(&tok, '#')) {
			if(hash_count) {
				goto hash_err;
			}
			while(1) {
				++hash_count;
				/* in a real cpp we'd need to look for '\\' first */
				while(tokenizer_peek(&t2) == '\n') {
					x_tokenizer_next(&t2, &tok);
				}
				if(tokenizer_peek(&t2) == '#') x_tokenizer_next(&t2, &tok);
				else break;
			}
			if(hash_count == 1) flush_whitespace(output, &ws_count);
			else if(hash_count > 2) {
				error("only two '#' characters allowed for macro expansion", &t2, &tok);
				return 0;
			}
			if(hash_count == 2)
				ret = tokenizer_skip_chars(&t2, " \t\n", &ws_count);
			else
				ret = tokenizer_skip_chars(&t2, " \t", &ws_count);

			if(!ret) return ret;
			ws_count = 0;

		} else if(is_whitespace_token(&tok)) {
			ws_count++;
		} else {
			if(hash_count == 1) goto hash_err;
			flush_whitespace(output, &ws_count);
			emit_token(output, &tok, t2.buf);
		}
	}
	flush_whitespace(output, &ws_count);

	/* we need to expand macros after the macro arguments have been inserted */
	if(1) {
		cwae.f = freopen_r(cwae.f, &cwae.buf, &cwae.len);
#ifdef DEBUG
		dprintf(2, "contents with args expanded: %s\n", cwae.buf);
#endif
		tokenizer_from_file(&cwae.t, cwae.f);
		size_t mac_cnt = 0;
		while(1) {
			int ret = tokenizer_next(&cwae.t, &tok);
			if(!ret) return ret;
			if(tok.type == TT_EOF) break;
			if(tok.type == TT_IDENTIFIER && get_macro(cpp, cwae.t.buf))
				++mac_cnt;
		}

		tokenizer_rewind(&cwae.t);
		struct macro_info *mcs = calloc(mac_cnt, sizeof(struct macro_info));
		{
			size_t mac_iter = 0;
			get_macro_info(cpp, &cwae.t, mcs, &mac_iter, 0, 0, "null", visited, rec_level);
			/* some of the macros might not expand at this stage (without braces)*/
			while(mac_cnt && mcs[mac_cnt-1].name == 0)
				--mac_cnt;
		}
		size_t i; int depth = 0;
		for(i = 0; i < mac_cnt; ++i) {
			if(mcs[i].nest > depth) depth = mcs[i].nest;
		}
		while(depth > -1) {
			for(i = 0; i < mac_cnt; ++i) if(mcs[i].nest == depth) {
				struct macro_info *mi = &mcs[i];
				tokenizer_rewind(&cwae.t);
				size_t j;
				struct token utok;
				for(j = 0; j < mi->first+1; ++j)
					tokenizer_next(&cwae.t, &utok);
				struct FILE_container t2 = {0}, tmp = {0};
				t2.f = open_memstream(&t2.buf, &t2.len);
				if(!expand_macro(cpp, &cwae.t, t2.f, mi->name, rec_level+1, visited))
					return 0;
				t2.f = freopen_r(t2.f, &t2.buf, &t2.len);
				tokenizer_from_file(&t2.t, t2.f);
				/* manipulating the stream in case more stuff has been consumed */
				off_t cwae_pos = tokenizer_ftello(&cwae.t);
				tokenizer_rewind(&cwae.t);
#ifdef DEBUG
				dprintf(2, "merging %s with %s\n", cwae.buf, t2.buf);
#endif
				int diff = mem_tokenizers_join(&cwae, &t2, &tmp, mi->first, cwae_pos);
				free_file_container(&cwae);
				free_file_container(&t2);
				cwae = tmp;
#ifdef DEBUG
				dprintf(2, "result: %s\n", cwae.buf);
#endif
				if(diff == 0) continue;
				for(j = 0; j < mac_cnt; ++j) {
					if(j == i) continue;
					struct macro_info *mi2 = &mcs[j];
					/* modified element mi can be either inside, after or before
					   another macro. the after case doesn't affect us. */
					if(mi->first >= mi2->first && mi->last <= mi2->last) {
						/* inside m2 */
						mi2->last += diff;
					} else if (mi->first < mi2->first) {
						/* before m2 */
						mi2->first += diff;
						mi2->last += diff;
					}
				}
			}
			--depth;
		}
		tokenizer_rewind(&cwae.t);
		while(1) {
			struct macro *ma;
			tokenizer_next(&cwae.t, &tok);
			if(tok.type == TT_EOF) break;
			if(tok.type == TT_IDENTIFIER && tokenizer_peek(&cwae.t) == EOF &&
			   (ma = get_macro(cpp, cwae.t.buf)) && FUNCTIONLIKE(ma) && tchain_parens_follows(cpp, rec_level) != -1
			) {
				int ret = expand_macro(cpp, &cwae.t, out, cwae.t.buf, rec_level+1, visited);
				if(!ret) return ret;
			} else
				emit_token(out, &tok, cwae.t.buf);
		}
		free(mcs);
	}

	free_file_container(&cwae);

cleanup:
	for(i=0; i < num_args; i++) {
		fclose(argvalues[i].f);
		free(argvalues[i].buf);
	}
	free(argvalues);
	return 1;
}

#define TT_LAND TT_CUSTOM+0
#define TT_LOR TT_CUSTOM+1
#define TT_LTE TT_CUSTOM+2
#define TT_GTE TT_CUSTOM+3
#define TT_SHL TT_CUSTOM+4
#define TT_SHR TT_CUSTOM+5
#define TT_EQ TT_CUSTOM+6
#define TT_NEQ TT_CUSTOM+7
#define TT_LT TT_CUSTOM+8
#define TT_GT TT_CUSTOM+9
#define TT_BAND TT_CUSTOM+10
#define TT_BOR TT_CUSTOM+11
#define TT_XOR TT_CUSTOM+12
#define TT_NEG TT_CUSTOM+13
#define TT_PLUS TT_CUSTOM+14
#define TT_MINUS TT_CUSTOM+15
#define TT_MUL TT_CUSTOM+16
#define TT_DIV TT_CUSTOM+17
#define TT_MOD TT_CUSTOM+18
#define TT_LPAREN TT_CUSTOM+19
#define TT_RPAREN TT_CUSTOM+20
#define TT_LNOT TT_CUSTOM+21

#define TTINT(X) X-TT_CUSTOM
#define TTENT(X, Y) [TTINT(X)] = Y

static int bp(int tokentype) {
	static const int bplist[] = {
		TTENT(TT_LOR, 1 << 4),
		TTENT(TT_LAND, 1 << 5),
		TTENT(TT_BOR, 1 << 6),
		TTENT(TT_XOR, 1 << 7),
		TTENT(TT_BAND, 1 << 8),
		TTENT(TT_EQ, 1 << 9),
		TTENT(TT_NEQ, 1 << 9),
		TTENT(TT_LTE, 1 << 10),
		TTENT(TT_GTE, 1 << 10),
		TTENT(TT_LT, 1 << 10),
		TTENT(TT_GT, 1 << 10),
		TTENT(TT_SHL, 1 << 11),
		TTENT(TT_SHR, 1 << 11),
		TTENT(TT_PLUS, 1 << 12),
		TTENT(TT_MINUS, 1 << 12),
		TTENT(TT_MUL, 1 << 13),
		TTENT(TT_DIV, 1 << 13),
		TTENT(TT_MOD, 1 << 13),
		TTENT(TT_NEG, 1 << 14),
		TTENT(TT_LNOT, 1 << 14),
		TTENT(TT_LPAREN, 1 << 15),
//		TTENT(TT_RPAREN, 1 << 15),
//		TTENT(TT_LPAREN, 0),
		TTENT(TT_RPAREN, 0),
	};
	if(TTINT(tokentype) < sizeof(bplist)/sizeof(bplist[0])) return bplist[TTINT(tokentype)];
	return 0;
}

static int expr(struct tokenizer *t, int rbp, int *err);

static int charlit_to_int(const char *lit) {
	if(lit[1] == '\\') switch(lit[2]) {
		case '0': return 0;
		case 'n': return 10;
		case 't': return 9;
		case 'r': return 13;
		case 'x': return strtol(lit+3, NULL, 16);
		default: return lit[2];
	}
	return lit[1];
}

static int nud(struct tokenizer *t, struct token *tok, int *err) {
	switch((unsigned) tok->type) {
		case TT_IDENTIFIER: return 0;
		case TT_WIDECHAR_LIT:
		case TT_SQSTRING_LIT:  return charlit_to_int(t->buf);
		case TT_HEX_INT_LIT:
		case TT_OCT_INT_LIT:
		case TT_DEC_INT_LIT:
			return strtol(t->buf, NULL, 0);
		case TT_NEG:   return ~ expr(t, bp(tok->type), err);
		case TT_PLUS:  return expr(t, bp(tok->type), err);
		case TT_MINUS: return - expr(t, bp(tok->type), err);
		case TT_LNOT:  return !expr(t, bp(tok->type), err);
		case TT_LPAREN: {
			int inner = expr(t, 0, err);
			if(0!=expect(t, TT_RPAREN, (const char*[]){")", 0}, tok)) {
				error("missing ')'", t, tok);
				return 0;
			}
			return inner;
		}
		case TT_FLOAT_LIT:
			error("floating constant in preprocessor expression", t, tok);
			*err = 1;
			return 0;
		case TT_RPAREN:
		default:
			error("unexpected token", t, tok);
			*err = 1;
			return 0;
	}
}

static int led(struct tokenizer *t, int left, struct token *tok, int *err) {
	int right;
	switch((unsigned) tok->type) {
		case TT_LAND:
		case TT_LOR:
			right = expr(t, bp(tok->type), err);
			if(tok->type == TT_LAND) return left && right;
			return left || right;
		case TT_LTE:  return left <= expr(t, bp(tok->type), err);
		case TT_GTE:  return left >= expr(t, bp(tok->type), err);
		case TT_SHL:  return left << expr(t, bp(tok->type), err);
		case TT_SHR:  return left >> expr(t, bp(tok->type), err);
		case TT_EQ:   return left == expr(t, bp(tok->type), err);
		case TT_NEQ:  return left != expr(t, bp(tok->type), err);
		case TT_LT:   return left <  expr(t, bp(tok->type), err);
		case TT_GT:   return left >  expr(t, bp(tok->type), err);
		case TT_BAND: return left &  expr(t, bp(tok->type), err);
		case TT_BOR:  return left |  expr(t, bp(tok->type), err);
		case TT_XOR:  return left ^  expr(t, bp(tok->type), err);
		case TT_PLUS: return left +  expr(t, bp(tok->type), err);
		case TT_MINUS:return left -  expr(t, bp(tok->type), err);
		case TT_MUL:  return left *  expr(t, bp(tok->type), err);
		case TT_DIV:
		case TT_MOD:
			right = expr(t, bp(tok->type), err);
			if(right == 0)  {
				error("eval: div by zero", t, tok);
				*err = 1;
			}
			else if(tok->type == TT_DIV) return left / right;
			else if(tok->type == TT_MOD) return left % right;
			return 0;
		default:
			error("eval: unexpect token", t, tok);
			*err = 1;
			return 0;
	}
}


static int tokenizer_peek_next_non_ws(struct tokenizer *t, struct token *tok)
{
	int ret;
	while(1) {
		ret = tokenizer_peek_token(t, tok);
		if(is_whitespace_token(tok))
			x_tokenizer_next(t, tok);
		else break;
	}
	return ret;
}

static int expr(struct tokenizer *t, int rbp, int*err) {
	struct token tok;
	int ret = skip_next_and_ws(t, &tok);
	if(tok.type == TT_EOF) return 0;
	int left = nud(t, &tok, err);
	while(1) {
		ret = tokenizer_peek_next_non_ws(t, &tok);
		if(bp(tok.type) <= rbp) break;
		ret = tokenizer_next(t, &tok);
		if(tok.type == TT_EOF) break;
		left = led(t, left, &tok, err);
	}
	(void) ret;
	return left;
}

static int do_eval(struct tokenizer *t, int *result) {
	tokenizer_register_custom_token(t, TT_LAND, "&&");
	tokenizer_register_custom_token(t, TT_LOR, "||");
	tokenizer_register_custom_token(t, TT_LTE, "<=");
	tokenizer_register_custom_token(t, TT_GTE, ">=");
	tokenizer_register_custom_token(t, TT_SHL, "<<");
	tokenizer_register_custom_token(t, TT_SHR, ">>");
	tokenizer_register_custom_token(t, TT_EQ, "==");
	tokenizer_register_custom_token(t, TT_NEQ, "!=");

	tokenizer_register_custom_token(t, TT_LT, "<");
	tokenizer_register_custom_token(t, TT_GT, ">");

	tokenizer_register_custom_token(t, TT_BAND, "&");
	tokenizer_register_custom_token(t, TT_BOR, "|");
	tokenizer_register_custom_token(t, TT_XOR, "^");
	tokenizer_register_custom_token(t, TT_NEG, "~");

	tokenizer_register_custom_token(t, TT_PLUS, "+");
	tokenizer_register_custom_token(t, TT_MINUS, "-");
	tokenizer_register_custom_token(t, TT_MUL, "*");
	tokenizer_register_custom_token(t, TT_DIV, "/");
	tokenizer_register_custom_token(t, TT_MOD, "%");

	tokenizer_register_custom_token(t, TT_LPAREN, "(");
	tokenizer_register_custom_token(t, TT_RPAREN, ")");
	tokenizer_register_custom_token(t, TT_LNOT, "!");

	int err = 0;
	*result = expr(t, 0, &err);
#ifdef DEBUG
	dprintf(2, "eval result: %d\n", *result);
#endif
	return !err;
}

static int evaluate_condition(struct cpp *cpp, struct tokenizer *t, int *result, char *visited[]) {
	int ret, backslash_seen = 0;
	struct token curr;
	char *bufp;
	size_t size;
	int tflags = tokenizer_get_flags(t);
	tokenizer_set_flags(t, tflags | TF_PARSE_WIDE_STRINGS);
	ret = tokenizer_next(t, &curr);
	if(!ret) return ret;
	if(!is_whitespace_token(&curr)) {
		error("expected whitespace after if/elif", t, &curr);
		return 0;
	}
	FILE *f = open_memstream(&bufp, &size);
	while(1) {
		ret = tokenizer_next(t, &curr);
		if(!ret) return ret;
		if(curr.type == TT_IDENTIFIER) {
			if(!expand_macro(cpp, t, f, t->buf, -1, visited)) return 0;
		} else if(curr.type == TT_SEP) {
			if(curr.value == '\\')
				backslash_seen = 1;
			else {
				if(curr.value == '\n') {
					if(!backslash_seen) break;
				} else {
					emit_token(f, &curr, t->buf);
				}
				backslash_seen = 0;
			}
		} else {
			emit_token(f, &curr, t->buf);
		}
	}
	f = freopen_r(f, &bufp, &size);
	if(!f || size == 0) {
		error("#(el)if with no expression", t, &curr);
		return 0;
	}
#ifdef DEBUG
	dprintf(2, "evaluating condition %s\n", bufp);
#endif
	struct tokenizer t2;
	tokenizer_from_file(&t2, f);
	ret = do_eval(&t2, result);
	fclose(f);
	free(bufp);
	tokenizer_set_flags(t, tflags);
	return ret;
}

static void free_visited(char *visited[]) {
	size_t i;
	for(i=0; i< MAX_RECURSION; i++)
		if(visited[i]) free(visited[i]);

}

int parse_file(struct cpp *cpp, FILE *f, const char *fn, FILE *out) {
	struct tokenizer t;
	struct token curr;
	tokenizer_init(&t, f, TF_PARSE_STRINGS);
	tokenizer_set_filename(&t, fn);
	tokenizer_register_marker(&t, MT_MULTILINE_COMMENT_START, "/*"); /**/
	tokenizer_register_marker(&t, MT_MULTILINE_COMMENT_END, "*/");
	tokenizer_register_marker(&t, MT_SINGLELINE_COMMENT_START, "//");
	int ret, newline=1, ws_count = 0;

	int if_level = 0, if_level_active = 0, if_level_satisfied = 0;

#define all_levels_active() (if_level_active == if_level)
#define prev_level_active() (if_level_active == if_level-1)
#define set_level(X, V) do { \
		if(if_level_active > X) if_level_active = X; \
		if(if_level_satisfied > X) if_level_satisfied = X; \
		if(V != -1) { \
			if(V) if_level_active = X; \
			else if(if_level_active == X) if_level_active = X-1; \
			if(V && if_level_active == X) if_level_satisfied = X; \
		} \
		if_level = X; \
	} while(0)
#define skip_conditional_block (if_level > if_level_active)

	static const char* directives[] = {"include", "error", "warning", "define", "undef", "if", "elif", "else", "ifdef", "ifndef", "endif", "line", "pragma", 0};
	while((ret = tokenizer_next(&t, &curr)) && curr.type != TT_EOF) {
		newline = curr.column == 0;
		if(newline) {
			ret = eat_whitespace(&t, &curr, &ws_count);
			if(!ret) return ret;
		}
		if(curr.type == TT_EOF) break;
		if(skip_conditional_block && !(newline && is_char(&curr, '#'))) continue;
		if(is_char(&curr, '#')) {
			if(!newline) {
				error("stray #", &t, &curr);
				return 0;
			}
			int index = expect(&t, TT_IDENTIFIER, directives, &curr);
			if(index == -1) {
				if(skip_conditional_block) continue;
				error("invalid preprocessing directive", &t, &curr);
				return 0;
			}
			if(skip_conditional_block) switch(index) {
				case 0: case 1: case 2: case 3: case 4:
				case 11: case 12:
					continue;
				default: break;
			}
			switch(index) {
			case 0:
				ret = include_file(cpp, &t, out);
				if(!ret) return ret;
				break;
			case 1:
				ret = emit_error_or_warning(&t, 1);
				if(!ret) return ret;
				break;
			case 2:
				ret = emit_error_or_warning(&t, 0);
				if(!ret) return ret;
				break;
			case 3:
				ret = parse_macro(cpp, &t);
				if(!ret) return ret;
				break;
			case 4:
				if(!skip_next_and_ws(&t, &curr)) return 0;
				if(curr.type != TT_IDENTIFIER) {
					error("expected identifier", &t, &curr);
					return 0;
				}
				undef_macro(cpp, t.buf);
				break;
			case 5: // if
				if(all_levels_active()) {
					char* visited[MAX_RECURSION] = {0};
					if(!evaluate_condition(cpp, &t, &ret, visited)) return 0;
					free_visited(visited);
					set_level(if_level + 1, ret);
				} else {
					set_level(if_level + 1, 0);
				}
				break;
			case 6: // elif
				if(prev_level_active() && if_level_satisfied < if_level) {
					char* visited[MAX_RECURSION] = {0};
					if(!evaluate_condition(cpp, &t, &ret, visited)) return 0;
					free_visited(visited);
					if(ret) {
						if_level_active = if_level;
						if_level_satisfied = if_level;
					}
				} else if(if_level_active == if_level) {
					--if_level_active;
				}
				break;
			case 7: // else
				if(prev_level_active() && if_level_satisfied < if_level) {
					if(1) {
						if_level_active = if_level;
						if_level_satisfied = if_level;
					}
				} else if(if_level_active == if_level) {
					--if_level_active;
				}
				break;
			case 8: // ifdef
			case 9: // ifndef
				if(!skip_next_and_ws(&t, &curr) || curr.type == TT_EOF) return 0;
				ret = !!get_macro(cpp, t.buf);
				if(index == 9) ret = !ret;

				if(all_levels_active()) {
					set_level(if_level + 1, ret);
				} else {
					set_level(if_level + 1, 0);
				}
				break;
			case 10: // endif
				set_level(if_level-1, -1);
				break;
			case 11: // line
				ret = tokenizer_read_until(&t, "\n", 1);
				if(!ret) {
					error("unknown", &t, &curr);
					return 0;
				}
				break;
			case 12: // pragma
				emit(out, "#pragma");
				while((ret = x_tokenizer_next(&t, &curr)) && curr.type != TT_EOF) {
					emit_token(out, &curr, t.buf);
					if(is_char(&curr, '\n')) break;
				}
				if(!ret) return ret;
				break;
			default:
				break;
			}
			continue;
		} else {
			while(ws_count) {
				emit(out, " ");
				--ws_count;
			}
		}
#if DEBUG
		dprintf(2, "(stdin:%u,%u) ", curr.line, curr.column);
		if(curr.type == TT_SEP)
			dprintf(2, "separator: %c\n", curr.value == '\n'? ' ' : curr.value);
		else
			dprintf(2, "%s: %s\n", tokentype_to_str(curr.type), t.buf);
#endif
		if(curr.type == TT_IDENTIFIER) {
			char* visited[MAX_RECURSION] = {0};
			if(!expand_macro(cpp, &t, out, t.buf, 0, visited))
				return 0;
			free_visited(visited);
		} else {
			emit_token(out, &curr, t.buf);
		}
	}
	if(if_level) {
		error("unterminated #if", &t, &curr);
		return 0;
	}
	return 1;
}

struct cpp * cpp_new(void) {
	struct cpp* ret = calloc(1, sizeof(struct cpp));
	if(!ret) return ret;
	tglist_init(&ret->includedirs);
	cpp_add_includedir(ret, ".");
	ret->macros = hbmap_new(strptrcmp, string_hash, 128);
	struct macro m = {.num_args = 1};
	add_macro(ret, strdup("defined"), &m);
	m.num_args = MACRO_FLAG_OBJECTLIKE;
	add_macro(ret, strdup("__FILE__"), &m);
	add_macro(ret, strdup("__LINE__"), &m);
	return ret;
}

void cpp_free(struct cpp*cpp) {
	free_macros(cpp);
	tglist_free_values(&cpp->includedirs);
	tglist_free_items(&cpp->includedirs);
}

void cpp_add_includedir(struct cpp *cpp, const char* includedir) {
	tglist_add(&cpp->includedirs, strdup(includedir));
}

int cpp_add_define(struct cpp *cpp, const char *mdecl) {
	struct FILE_container tmp = {0};
	tmp.f = open_memstream(&tmp.buf, &tmp.len);
	fprintf(tmp.f, "%s\n", mdecl);
	tmp.f = freopen_r(tmp.f, &tmp.buf, &tmp.len);
	tokenizer_from_file(&tmp.t, tmp.f);
	int ret = parse_macro(cpp, &tmp.t);
	free_file_container(&tmp);
	return ret;
}

int cpp_run(struct cpp *cpp, FILE* in, FILE* out, const char* inname) {
	return parse_file(cpp, in, inname, out);
}
