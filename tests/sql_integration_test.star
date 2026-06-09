# sql_integration_test.star — real Postgres/MySQL integration.
#
# Gated: set STARKITE_PG_DSN and/or STARKITE_MYSQL_DSN to run; each test skips
# when its DSN is absent. Requires the all-in-one kite (pg/mysql drivers).
#   kite test tests/sql_integration_test.star --permissions=allow-all

def test_postgres():
    dsn = os.env("STARKITE_PG_DSN")
    if not dsn:
        skip("STARKITE_PG_DSN not set")
    db = sql.open("postgres", dsn)
    db.exec("DROP TABLE IF EXISTS sk_t")
    db.exec("CREATE TABLE sk_t (id INT PRIMARY KEY, name TEXT, amount DECIMAL(10,2))")

    res = db.exec("INSERT INTO sk_t (id, name, amount) VALUES ($1, $2, $3)", 1, "alice", "19.99")
    assert_equal(res.rows_affected, 1)

    row = db.query_row("SELECT name, amount FROM sk_t WHERE id = $1", 1)
    assert_equal(row["name"], "alice")
    assert_equal(row["amount"], "19.99")  # DECIMAL → string, precision preserved

    # managed transaction
    db.tx(lambda tx: tx.exec("INSERT INTO sk_t (id, name) VALUES ($1, $2)", 2, "bob"))
    assert_equal(db.query_value("SELECT count(*) FROM sk_t"), 2)

    # batch (atomic)
    db.batch([
        sql.stmt("INSERT INTO sk_t (id, name) VALUES ($1, $2)", 3, "carol"),
        sql.stmt("INSERT INTO sk_t (id, name) VALUES ($1, $2)", 4, "dave"),
    ])
    assert_equal(db.query_value("SELECT count(*) FROM sk_t"), 4)

    # placeholder mismatch → driver-aware hint
    r = db.try_query("SELECT * FROM sk_t WHERE id = ?")
    assert_true(not r.ok)

    # insert generates $N placeholders for postgres
    db.exec("DROP TABLE IF EXISTS sk_ins")
    db.exec("CREATE TABLE sk_ins (id SERIAL PRIMARY KEY, name TEXT, n INT)")
    assert_equal(db.insert("sk_ins", {"name": "x", "n": 7}).rows_affected, 1)
    db.insert("sk_ins", [{"name": "a", "n": 1}, {"name": "b", "n": 2}])
    assert_equal(db.query_value("SELECT count(*) FROM sk_ins"), 3)
    db.exec("DROP TABLE sk_ins")

    # migrate: tracking table + apply/skip
    db.exec("DROP TABLE IF EXISTS schema_migrations")
    migs = [sql.stmt("CREATE TABLE sk_mig (id INT)", name="001_mig")]
    assert_equal(len(db.migrate(migs).applied), 1)
    assert_equal(len(db.migrate(migs).skipped), 1)
    db.exec("DROP TABLE sk_mig")
    db.exec("DROP TABLE schema_migrations")

    db.exec("DROP TABLE sk_t")
    db.close()

def test_mysql():
    dsn = os.env("STARKITE_MYSQL_DSN")
    if not dsn:
        skip("STARKITE_MYSQL_DSN not set")
    db = sql.open("mysql", dsn)
    db.exec("DROP TABLE IF EXISTS sk_t")
    db.exec("CREATE TABLE sk_t (id INT PRIMARY KEY, name TEXT, amount DECIMAL(10,2))")

    res = db.exec("INSERT INTO sk_t (id, name, amount) VALUES (?, ?, ?)", 1, "alice", "19.99")
    assert_equal(res.rows_affected, 1)

    row = db.query_row("SELECT name, amount FROM sk_t WHERE id = ?", 1)
    assert_equal(row["name"], "alice")
    assert_equal(row["amount"], "19.99")  # DECIMAL → string

    db.exec_many("INSERT INTO sk_t (id, name) VALUES (?, ?)", [[2, "bob"], [3, "carol"]])
    assert_equal(db.query_value("SELECT count(*) FROM sk_t"), 3)

    names = db.query_column("SELECT name FROM sk_t ORDER BY id")
    assert_equal(names, ["alice", "bob", "carol"])

    # insert generates ? placeholders for mysql
    db.exec("DROP TABLE IF EXISTS sk_ins")
    db.exec("CREATE TABLE sk_ins (id INT AUTO_INCREMENT PRIMARY KEY, name TEXT, n INT)")
    assert_equal(db.insert("sk_ins", {"name": "x", "n": 7}).rows_affected, 1)
    db.insert("sk_ins", [{"name": "a", "n": 1}, {"name": "b", "n": 2}])
    assert_equal(db.query_value("SELECT count(*) FROM sk_ins"), 3)
    db.exec("DROP TABLE sk_ins")

    # migrate: VARCHAR(255) tracking table works on mysql
    db.exec("DROP TABLE IF EXISTS schema_migrations")
    migs = [sql.stmt("CREATE TABLE sk_mig (id INT)", name="001_mig")]
    assert_equal(len(db.migrate(migs).applied), 1)
    assert_equal(len(db.migrate(migs).skipped), 1)
    db.exec("DROP TABLE sk_mig")
    db.exec("DROP TABLE schema_migrations")

    db.exec("DROP TABLE sk_t")
    db.close()
