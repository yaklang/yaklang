/* Grammar coverage for unexpanded BSD queue/hash macros and unused-value casts. */

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
