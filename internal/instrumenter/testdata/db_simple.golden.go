package workload

import (
	"context"
	"database/sql"
	"runtime/trace"
)

func f(ctx context.Context, db *sql.DB) error {
	_, err := db.QueryContext(ctx, "SELECT 1")
	return err
}
func init() {
	RegisterWorkload("demo",
		RunDemoProgram,
	)
}
