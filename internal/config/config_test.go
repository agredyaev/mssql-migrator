package config

import "testing"

func TestValidateRejectsInvalidManagedSchema(t *testing.T) {
	cfg := Config{Env: "prod", Server: "server", Database: "db", User: "user", Password: "password", ManagedSchemas: []string{"reporting", "bad-name"}}
	if cfg.Validate() == nil {
		t.Fatal("expected invalid schema error")
	}
}
