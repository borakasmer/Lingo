package DB

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/microsoft/go-mssqldb"
)

func SqlOpen(
	ctx context.Context,
	connectionString string,
) (*sql.DB, error) {
	db, err := sql.Open("sqlserver", connectionString)
	if err != nil {
		return nil, fmt.Errorf("SQL bağlantısı oluşturulamadı: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("SQL bağlantısı kurulamadı: %w", err)
	}

	return db, nil
}
