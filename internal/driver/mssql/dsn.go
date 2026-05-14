package mssql

import (
	"fmt"
	"net/url"
	"strings"

	"reporting-db-migrations/internal/types"
)

func BuildDSN(cfg types.Config) string {
	query := url.Values{}
	query.Set("database", cfg.Database)
	query.Set("encrypt", fmt.Sprintf("%t", cfg.Encrypt))
	query.Set("TrustServerCertificate", fmt.Sprintf("%t", cfg.TrustServerCertificate))
	query.Set("app name", "rmig")
	if cfg.DBAuthMode() == types.DBAuthIntegrated {
		query.Set("authenticator", "winsspi")
	}

	var user *url.Userinfo
	if cfg.DBAuthMode() == types.DBAuthSQL && cfg.User != "" {
		user = url.UserPassword(cfg.User, cfg.Password)
	} else if strings.TrimSpace(cfg.User) != "" {
		user = url.User(cfg.User)
	}

	u := &url.URL{
		Scheme:   "sqlserver",
		User:     user,
		Host:     fmt.Sprintf("%s:%s", cfg.Server, cfg.Port),
		RawQuery: query.Encode(),
	}
	return u.String()
}
