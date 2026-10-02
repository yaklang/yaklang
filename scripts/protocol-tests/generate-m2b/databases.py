"""Independent native database fixture clients. Run while capturing loopback ports 13306/15432.
Requires mysql-connector-python==9.2.0 and psycopg[binary]==3.2.6.
Only synthetic, public fixture credentials and data are used.
"""
import datetime, json, pathlib, sys
import mysql.connector
import psycopg
out = pathlib.Path(sys.argv[1]); out.mkdir(parents=True, exist_ok=True)
oracle = {}
c = mysql.connector.connect(host='127.0.0.1', port=13306, user='m2', password='fixture-only', database='m2', ssl_disabled=True, use_pure=True)
p = c.cursor(prepared=True)
rows = []
for args in [(42, 'fixture', None, datetime.date(2026,9,22)), (-7, 'second', None, datetime.date(2000,1,2))]:
    p.execute('SELECT CAST(? AS SIGNED) AS n, ? AS txt, ? AS nullable, CAST(? AS DATE) AS day', args)
    rows.append(p.fetchall())
p.close()
c.close()
oracle['mysql.connector'] = rows
with psycopg.connect('host=127.0.0.1 port=15432 user=m2 dbname=postgres sslmode=disable', autocommit=True) as c:
    c.execute("SET client_encoding TO 'UTF8'")
    c.execute('CREATE TEMP TABLE m2 (n integer, txt text)')
    with c.cursor().copy('COPY m2 FROM STDIN') as cp:
        cp.write_row((1, 'first')); cp.write_row((2, 'second'))
    oracle['pg_prepared'] = [c.execute('SELECT n, txt, %s::integer AS parameter FROM m2 ORDER BY n', (v,), prepare=True).fetchall() for v in (42,7)]
    with c.cursor().copy('COPY m2 TO STDOUT') as cp:
        oracle['pg_copy'] = b''.join(bytes(v) for v in cp).decode()
    try:
        c.execute('SELECT 1 / %s::integer', (0,), prepare=True)
    except psycopg.errors.DivisionByZero:
        pass
    oracle['pg_after_error'] = c.execute('SELECT %s::integer', (99,), prepare=True).fetchall()
    with c.transaction():
        with c.cursor(name='m2_portal') as cur:
            cur.execute('SELECT n, txt FROM m2 ORDER BY n')
            oracle['pg_portal'] = [cur.fetchone(), cur.fetchone()]
    c.execute("DO $$ BEGIN RAISE NOTICE 'm2 notice'; END $$")
    try:
        with c.cursor().copy('COPY m2 FROM STDIN') as cp:
            cp.write_row((3,'abort'))
            raise ValueError('fixture abort')
    except ValueError:
        pass
    oracle['pg_after_copy_abort'] = c.execute('SELECT count(*) FROM m2').fetchone()
(out/'database-oracle.json').write_text(json.dumps(oracle, default=str, indent=2)+'\n')
