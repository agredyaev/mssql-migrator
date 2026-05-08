package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "github.com/microsoft/go-mssqldb"

	"reporting-db-migrations/internal/config"
)

func Open(ctx context.Context, cfg config.Config) (*sql.DB, error) {
	dsn := buildDSN(cfg)
	database, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, err
	}
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}

func buildDSN(cfg config.Config) string {
	query := url.Values{}
	query.Set("database", cfg.Database)
	query.Set("encrypt", fmt.Sprintf("%t", cfg.Encrypt))
	query.Set("TrustServerCertificate", fmt.Sprintf("%t", cfg.TrustServerCertificate))
	query.Set("app name", "reporting-migrator")

	url := &url.URL{
		Scheme:   "sqlserver",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     fmt.Sprintf("%s:%s", cfg.Server, cfg.Port),
		RawQuery: query.Encode(),
	}
	return url.String()
}
