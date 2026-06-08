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
