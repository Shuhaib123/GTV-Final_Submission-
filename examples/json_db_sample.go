package main

// Minimal program to exercise encoding/json and database/sql calls.
// Paste this into the live Instrument page with name: iodemo
// Then enable IO Regions (or IO JSON/DB env/config) to see json.* and db.* regions.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type user struct {
	ID   int
	Name string
}

func main() {
	ctx := context.Background()

	// encoding/json: Marshal + Unmarshal
	u := user{ID: 42, Name: "Ada"}
	b, _ := json.Marshal(u)
	var u2 user
	_ = json.Unmarshal(b, &u2)

	// database/sql: QueryContext / ExecContext (unreachable, but type-checked)
	var db *sql.DB // nil; we guard calls so they are never executed
	query := "SELECT 1"
	if false {
		_, _ = db.QueryContext(ctx, query)
		_, _ = db.ExecContext(ctx, "UPDATE t SET v=1 WHERE id=?", 1)
	}

	// gtv:loop=sample
	for i := 0; i < 3; i++ {
		// simple safe loop to demonstrate loop region labeling
		fmt.Sprint(i)
	}
}
