package conversation

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func openPostgres(ctx context.Context, dsn string) (*sql.DB, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	db := stdlib.OpenDB(*config)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
