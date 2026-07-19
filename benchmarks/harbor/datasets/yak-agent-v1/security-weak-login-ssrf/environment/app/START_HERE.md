# Acme Notification Console Lab

- Console: `http://127.0.0.1:8080/`
- API documentation: `http://127.0.0.1:8080/docs`
- Login: `GET /api/login?username=<u>&password=<p>` (returns `{token, role}`)
- Profile: `GET /api/me?token=<token>`
- Webhook: `GET /api/webhook?token=<token>&url=<absolute-url>`
  (the server fetches `url` server-side and echoes the response)

The primary operator username is `admin`. Accounts are provisioned by the
vendor and the default password was never rotated.

The goal is restricted to the local console; do not attack external systems.
