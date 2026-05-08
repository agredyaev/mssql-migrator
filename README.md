# Reporting Migrator

Go CLI for MSSQL reporting-layer migrations, validation, and reports.

## Build

```bash
go mod tidy
gofmt -w .
go test ./...
go vet ./...
go build -o reporting-migrator ./cmd/reporting-migrator
```

## Commands

```bash
reporting-migrator version
reporting-migrator info --env prod
reporting-migrator plan --env prod
reporting-migrator migrate --env prod --plan-file reports/migration-plan.json
reporting-migrator validate --env prod
reporting-migrator baseline --env prod --up-to V010 --confirm
reporting-migrator repair-checksum --env prod --script R002__views.sql --confirm
```
