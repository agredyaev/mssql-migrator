package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

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
		if closeErr := database.Close(); closeErr != nil {
			return nil, fmt.Errorf("ping database: %w; close failed: %v", err, closeErr)
		}
		return nil, err
	}
	return database, nil
}

func buildDSN(cfg config.Config) string {
	query := url.Values{}
	query.Set("database", cfg.Database)
	query.Set("encrypt", fmt.Sprintf("%t", cfg.Encrypt))
	query.Set("TrustServerCertificate", fmt.Sprintf("%t", cfg.TrustServerCertificate))
	query.Set("app name", "rmig")
	if cfg.DBAuthMode() == config.DBAuthIntegrated {
		query.Set("authenticator", "winsspi")
	}

	var user *url.Userinfo
	if cfg.DBAuthMode() == config.DBAuthSQL {
		user = url.UserPassword(cfg.User, cfg.Password)
	} else if strings.TrimSpace(cfg.User) != "" {
		user = url.User(cfg.User)
	}

	url := &url.URL{
		Scheme:   "sqlserver",
		User:     user,
		Host:     fmt.Sprintf("%s:%s", cfg.Server, cfg.Port),
		RawQuery: query.Encode(),
	}
	return url.String()
}
