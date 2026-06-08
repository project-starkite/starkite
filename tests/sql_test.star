# sql_test.star — SQLite Layer 0 (open/query/exec/transaction) integration tests.
# Run: kite test tests/sql_test.star --permissions=allow-all

def _setup():
    db = sql.open("sqlite", ":memory:")
    db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, active BOOLEAN)")
    return db

def test_exec_insert_result():
    db = _setup()
    res = db.exec("INSERT INTO users (name, active) VALUES (?, ?)", "alice", True)
    assert_equal(res.last_insert_id, 1)
    assert_equal(res.rows_affected, 1)
    db.close()

def test_query_returns_list_of_dicts():
    db = _setup()
    db.exec("INSERT INTO users (name) VALUES (?)", "alice")
    db.exec("INSERT INTO users (name) VALUES (?)", "bob")
    rows = db.query("SELECT id, name FROM users ORDER BY id")
    assert_equal(len(rows), 2)
    assert_equal(rows[0]["name"], "alice")
    assert_equal(rows[1]["name"], "bob")
    db.close()

def test_query_row_and_none():
    db = _setup()
    db.exec("INSERT INTO users (name) VALUES (?)", "carol")
    row = db.query_row("SELECT name FROM users WHERE id = ?", 1)
    assert_equal(row["name"], "carol")
    assert_equal(db.query_row("SELECT name FROM users WHERE id = ?", 99), None)
    db.close()

def test_transaction_commit_persists():
    db = _setup()
    tx = db.begin()
    tx.exec("INSERT INTO users (name) VALUES (?)", "dave")
    tx.commit()
    assert_equal(len(db.query("SELECT * FROM users")), 1)
    db.close()

def test_transaction_rollback_discards():
    db = _setup()
    tx = db.begin()
    tx.exec("INSERT INTO users (name) VALUES (?)", "eve")
    tx.rollback()
    tx.rollback()  # no-op the second time
    assert_equal(len(db.query("SELECT * FROM users")), 0)
    db.close()

def test_null_roundtrip():
    db = _setup()
    db.exec("INSERT INTO users (name) VALUES (?)", None)
    row = db.query_row("SELECT name FROM users WHERE id = ?", 1)
    assert_equal(row["name"], None)
    db.close()

def test_param_types():
    db = _setup()
    db.exec("CREATE TABLE vals (i INTEGER, f REAL, s TEXT, b BOOLEAN)")
    db.exec("INSERT INTO vals (i, f, s, b) VALUES (?, ?, ?, ?)", 42, 3.5, "hi", True)
    row = db.query_row("SELECT i, f, s, b FROM vals")
    assert_equal(row["i"], 42)
    assert_equal(row["f"], 3.5)
    assert_equal(row["s"], "hi")
    db.close()

def test_try_query_on_error():
    db = _setup()
    res = db.try_query("SELECT * FROM does_not_exist")
    assert_true(not res.ok)
    assert_true(res.error != None)
    db.close()

def test_stats_and_driver():
    db = _setup()
    assert_equal(db.driver, "sqlite")
    stats = db.stats()
    assert_true(stats.max_open >= 1)
    db.close()

# --- Layer 1: managed transaction, batch, read helpers, bulk ---

def test_tx_commit_persists():
    db = _setup()
    def work(tx):
        tx.exec("INSERT INTO users (name) VALUES (?)", "frank")
    db.tx(work)
    assert_equal(len(db.query("SELECT * FROM users")), 1)
    db.close()

def test_tx_rollback_on_error():
    db = _setup()
    def work(tx):
        tx.exec("INSERT INTO users (name) VALUES (?)", "grace")
        fail("boom")
    res = db.try_tx(work)
    assert_true(not res.ok)
    assert_equal(len(db.query("SELECT * FROM users")), 0)
    db.close()

def test_tx_returns_callback_value():
    db = _setup()
    db.exec("INSERT INTO users (name) VALUES (?)", "heidi")
    n = db.tx(lambda tx: tx.query_value("SELECT count(*) FROM users"))
    assert_equal(n, 1)
    db.close()

def test_batch_named_results():
    db = _setup()
    res = db.batch([
        sql.stmt("INSERT INTO users (name) VALUES (?)", "ann", name="a"),
        sql.stmt("INSERT INTO users (name) VALUES (?)", "ben", name="b"),
    ])
    assert_equal(res["a"].rows_affected, 1)
    assert_equal(res["b"].last_insert_id, 2)
    assert_equal(len(db.query("SELECT * FROM users")), 2)
    db.close()

def test_batch_unnamed_list():
    db = _setup()
    res = db.batch([
        sql.stmt("INSERT INTO users (name) VALUES (?)", "x"),
        sql.stmt("INSERT INTO users (name) VALUES (?)", "y"),
    ])
    assert_equal(len(res), 2)
    assert_equal(res[0].rows_affected, 1)
    db.close()

def test_batch_rolls_back_on_failure():
    db = _setup()
    res = db.try_batch([
        sql.stmt("INSERT INTO users (name) VALUES (?)", "ok"),
        sql.stmt("INSERT INTO no_such_table VALUES (?)", 1),
    ])
    assert_true(not res.ok)
    assert_equal(len(db.query("SELECT * FROM users")), 0)
    db.close()

def test_query_value_scalar():
    db = _setup()
    db.exec("INSERT INTO users (name) VALUES (?)", "a")
    db.exec("INSERT INTO users (name) VALUES (?)", "b")
    assert_equal(db.query_value("SELECT count(*) FROM users"), 2)
    assert_equal(db.query_value("SELECT name FROM users WHERE id = ?", 99), None)
    db.close()

def test_query_column_flat_list():
    db = _setup()
    db.exec("INSERT INTO users (name) VALUES (?)", "a")
    db.exec("INSERT INTO users (name) VALUES (?)", "b")
    assert_equal(db.query_column("SELECT name FROM users ORDER BY id"), ["a", "b"])
    db.close()

def test_query_each_streaming():
    db = _setup()
    db.exec("INSERT INTO users (name) VALUES (?)", "a")
    db.exec("INSERT INTO users (name) VALUES (?)", "b")
    seen = []
    db.query_each("SELECT name FROM users ORDER BY id", lambda r: seen.append(r["name"]))
    assert_equal(seen, ["a", "b"])
    db.close()

def test_exec_many_bulk():
    db = _setup()
    res = db.exec_many("INSERT INTO users (name) VALUES (?)", [["a"], ["b"], ["c"]])
    assert_equal(res.rows_affected, 3)
    assert_equal(len(db.query("SELECT * FROM users")), 3)
    db.close()
