# Internal Report Gateway Lab

- Portal: `http://127.0.0.1:8080/`
- Export endpoint: `GET /api/admin/export`
- Normal role header: `X-User-Role: analyst`

The application was recently moved behind a reverse proxy that supplies the
same role header to the backend.

