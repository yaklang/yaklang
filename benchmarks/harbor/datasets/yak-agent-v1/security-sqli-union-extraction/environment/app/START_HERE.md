# Northwind Product Catalog Lab

- Catalog: `http://127.0.0.1:8080/`
- API documentation: `http://127.0.0.1:8080/docs`
- Product search: `GET /api/products?name=<substring>`
  (returns `{id, sku, name, price}` rows)

A hidden `loyalty_code` column exists in the backing database but is not
returned by any documented endpoint. The goal is restricted to the local
catalog; do not attack external systems.
