/* Grammar coverage for unexpanded BSD queue/hash macros, unused-value casts,
 * MSVC calling-convention function pointers, and declspec-like export macros.
 */

typedef void (__cdecl *_invalid_parameter_handler)(const wchar_t *, const wchar_t *, const wchar_t *, unsigned int, uintptr_t);
typedef void (__cdecl *_PVFV)(void);
typedef void (__cdecl * _PHNDLR)(int);
typedef void (WINAPI *GetSystemTimePreciseAsFileTime_fn_t)(LPFILETIME);

WEPOLL_EXPORT HANDLE epoll_create(int size);
WEPOLL_EXPORT HANDLE epoll_create1(int flags);
WEPOLL_EXPORT int epoll_wait(HANDLE ephnd, struct epoll_event *events, int maxevents, int timeout);

int evdns_base_nameserver_add(struct evdns_base *base, unsigned long int address);

typedef struct DECLSPEC_ALIGN (16) _M128A {
	ULONGLONG Low;
	LONGLONG High;
} M128A, *PM128A;

typedef struct {
	WORD Type;
} IMAGE_SYMBOL_EX, UNALIGNED *PIMAGE_SYMBOL_EX;

typedef struct {
	BYTE rgbReserved[12];
} IMAGE_AUX_SYMBOL_TOKEN_DEF,UNALIGNED *PIMAGE_AUX_SYMBOL_TOKEN_DEF;

typedef struct IXMLDOMNodeVtbl {
	BEGIN_INTERFACE
	HRESULT (__stdcall *QueryInterface)(IXMLDOMNode *This, const IID * const riid, void **ppvObject);
	ULONG (__stdcall *AddRef)(IXMLDOMNode *This);
	ULONG (__stdcall *Release)(IXMLDOMNode *This);
	END_INTERFACE
} IXMLDOMNodeVtbl;

static HT_HEAD(event_debug_map, event_debug_entry) global_debug_map = HT_INITIALIZER();
LIST_HEAD(client_list, client_tcp_connection) client_connections;
typedef TAILQ_HEAD(clients_s, client) clients_t;

struct client {
	char name[32];
	TAILQ_ENTRY(client) next;
};

enum EPOLL_EVENTS {
	(1U << 0) = (int)(1U << 0),
	(1U << 1) = (int)(1U << 1)
};

void unused_cast(int idle, int intvl, int cnt)
{
	(void) idle;
	(void) intvl;
	(void) cnt;
}

void foreach_braced(struct ctx *ctx)
{
	LIST_FOREACH(ev, &ctx->events, ev_io_next) {
		event_active_nolock_(ev, ev->ev_events, 1);
	}
	TAILQ_FOREACH(hook, head, next) {
		if (hook == handle) {
			break;
		}
	}
	LIST_FOREACH(bev, &g->members, rate_limiting->next_in_group) {
		bufferevent_suspend_read_(bev);
	}
}

struct evbuffer_chain {
	struct evbuffer_chain *next;
	size_t buffer_len;
	unsigned flags;
#define EVBUFFER_FILESEGMENT	0x0001  /**< A chain used for a file segment */
#define EVBUFFER_SENDFILE	0x0002	/**< a chain used with sendfile */
	/** a chain that mustn't be reallocated until un-pinned. */
#define EVBUFFER_MEM_PINNED_R	0x0010
	int refcnt;
	unsigned char *buffer;
};

struct event_layout {
	struct {
		struct event *le_next;
		struct event **le_prev;
	} ev_.ev_io.ev_io_next;
	short ev_.ev_signal.ev_ncalls;
	short *ev_.ev_signal.ev_pncalls;
};

void foreach_unbraced(struct ctx *ctx)
{
	LIST_FOREACH(ev, &ctx->events, ev_signal_next)
		event_active_nolock_(ev, EV_SIGNAL, ncalls);
}

void nested_member_foreach(struct ctl *ctl)
{
	TAILQ_FOREACH(ev, &ctl->events, ev_timeout_pos.ev_next_with_common_timeout) {
		if (ev->ev_fd == fd) {
			event_active_nolock_(ev, 0x01, 1);
		}
	}
}

int map_win_error(DWORD error)
{
	switch (error) {
		ERR__ERRNO_MAPPINGS(X)
	}
	return EINVAL;
}

/* Tables to implement ctypes-replacement EVUTIL_IS*() functions.
 * Extra stars before the terminator must still close the comment.
 **/
int after_star_star_slash(int idle)
{
	(void) idle;
	if (idle)
		return 0;
	return 1;
}

int load_ntdll_fn(void)
{
	void *fn_ptr = 0;
	NtCancelIoFileEx = (NTSTATUS(__stdcall*) (HANDLE FileHandle, PIO_STATUS_BLOCK IoStatusBlock))(nt__fn_ptr_cast_t) fn_ptr;
	NtCreateFile = (NTSTATUS(NTAPI*) (PHANDLE FileHandle, ACCESS_MASK DesiredAccess))(nt__fn_ptr_cast_t) fn_ptr;
	return 0;
}
