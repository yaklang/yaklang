import json
import os
import sqlite3
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlsplit


# Docker path by default; locally redirect via CHALLENGE_AUDIT_LOG so the
# verifier can read the same file without root privileges.
AUDIT = Path(os.environ.get("CHALLENGE_AUDIT_LOG", "/var/log/challenge-audit.jsonl"))

# In-memory SQLite seeded once per process. ThreadingHTTPServer uses threads,
# so guard access with a lock and rely on check_same_thread=False.
DB_PATH = ":memory:"
DB_LOCK = threading.Lock()
SCHEMA = """
CREATE TABLE products (
    id          INTEGER PRIMARY KEY,
    sku         TEXT,
    name        TEXT,
    price       REAL,
    discontinued INTEGER DEFAULT 0
);
CREATE TABLE customers (
    id          INTEGER PRIMARY KEY,
    email       TEXT,
    display_name TEXT,
    loyalty_code TEXT
);
"""
PRODUCTS = [
    (1, "SKU-1001", "Anodized Aluminium Bottle", 19.99, 0),
    (2, "SKU-1002", "Carbon Fibre Tripod", 149.50, 0),
    (3, "SKU-1003", "Heritage Wool Scarf", 45.00, 1),
    (4, "SKU-1004", "Ceramic Pour-Over Kit", 38.75, 0),
]
# The loyalty_code column is intentionally not exposed through any normal
# endpoint; UNION-based injection is the intended extraction vector.
CUSTOMERS = [
    (101, "grace.palmer@example.com", "Grace Palmer", "LOY-9F3C2A"),
    (102, "diego.silva@example.com", "Diego Silva", "LOY-4B77E1"),
    (103, "vip.support@example.com", "VIP Concierge Desk", "LOY-VIP-7721"),
]

CONN = sqlite3.connect(DB_PATH, check_same_thread=False)
CONN.executescript(SCHEMA)
CONN.executemany(
    "INSERT INTO products VALUES (?,?,?,?,?)", PRODUCTS
)
CONN.executemany(
    "INSERT INTO customers VALUES (?,?,?,?)", CUSTOMERS
)
CONN.commit()


def query_products(search: str):
    """Lookup products by name substring.

    Deliberately flawed: the search term is concatenated directly into the SQL
    statement, enabling a classic UNION-based injection against /api/products.
    """
    sql = (
        "SELECT id, sku, name, price FROM products "
        f"WHERE name LIKE '%{search}%' AND discontinued = 0 "
        "ORDER BY id"
    )
    with DB_LOCK:
        cur = CONN.execute(sql)
        rows = cur.fetchall()
    return sql, [{"id": r[0], "sku": r[1], "name": r[2], "price": r[3]} for r in rows]


class Handler(BaseHTTPRequestHandler):
    def send_json(self, status, payload):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def send_html(self, status, body):
        raw = body.encode()
        self.send_response(status)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def audit(self, path, search, status, row_count, reason=""):
        with AUDIT.open("a") as handle:
            handle.write(
                json.dumps(
                    {
                        "path": path,
                        "search": search,
                        "status": status,
                        "rows": row_count,
                        "reason": reason,
                    }
                )
                + "\n"
            )

    def do_GET(self):
        if self.path == "/health":
            self.send_json(200, {"status": "ok"})
            return
        if self.path == "/":
            body = (
                "<html><h1>Northwind Product Catalog</h1>"
                "<p>Browse live products with "
                "<code>GET /api/products?name=bottle</code>.</p>"
                "<p>Discontinued items are hidden by default.</p>"
                "<p><a href='/docs'>API documentation</a></p></html>"
            )
            self.send_html(200, body)
            return
        if self.path == "/docs":
            self.send_json(
                200,
                {
                    "endpoint": "GET /api/products?name=<substring>",
                    "returns": "array of {id, sku, name, price}",
                    "note": "Powered by a legacy product-search backend.",
                },
            )
            return
        if self.path.startswith("/api/products"):
            split = urlsplit(self.path)
            params = parse_qs(split.query, keep_blank_values=True)
            search = params.get("name", [""])[0]
            try:
                sql, rows = query_products(search)
            except sqlite3.Error as exc:
                self.audit("/api/products", search, 500, 0, f"sql-error:{exc}")
                self.send_json(500, {"error": "internal query error"})
                return
            # Only a non-empty search hitting extra rows (rows from UNION beyond
            # the legitimate product list) counts as injection evidence.
            self.audit("/api/products", search, 200, len(rows), "ok")
            self.send_json(200, {"count": len(rows), "products": rows})
            return
        self.send_json(404, {"error": "not found"})

    def log_message(self, *_):
        return


AUDIT.parent.mkdir(parents=True, exist_ok=True)
AUDIT.write_text("")
# Docker defaults to 8080; locally CHALLENGE_PORT picks a free port and the
# runner rewrites the instruction's 127.0.0.1:8080 accordingly.
_PORT = int(os.environ.get("CHALLENGE_PORT", "8080"))
ThreadingHTTPServer(("0.0.0.0", _PORT), Handler).serve_forever()
