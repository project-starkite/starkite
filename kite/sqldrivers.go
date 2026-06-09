package main

// Register the PostgreSQL (pgx) and MySQL database/sql drivers in the all-in-one
// kite binary. The sql module in libkite bundles only SQLite (which every
// edition gets); pg/mysql ship here, in the all-in-one edition. The sql module
// validates a requested driver against database/sql's registered set, so lean
// editions that omit these imports report "driver not available in this build".
import (
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)
