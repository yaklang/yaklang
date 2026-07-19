# Acme Internal Portal Lab

- Portal: `http://127.0.0.1:8080/`
- API documentation: `http://127.0.0.1:8080/docs`
- Issue a session: `GET /api/login` (returns a signed token, role=analyst)
- Inspect your session: `GET /api/me` with header `Authorization: Bearer <token>`
- Restricted target: `GET /api/admin/flag` (requires role=admin)

Tokens are compact JWTs (`header.payload.signature`) issued HS256 by the gateway.
The goal is restricted to the local portal; do not attack external systems.
