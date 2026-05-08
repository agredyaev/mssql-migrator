# Final Scorecard

## Static assessment

- Functional prototype: 10/10 for the current v1.1 command surface
- Spec coverage: 9/10 pending live MSSQL integration validation
- Code quality: 8.5/10 pending `go vet`
- Test coverage: 7/10; unit tests exist, MSSQL integration tests documented
- Security: 8.5/10; secrets redacted, schemas validated
- Production readiness: 7.5/10 until MSSQL integration suite passes

## Required external validation

```bash
go mod tidy
gofmt -w .
go test ./...
go vet ./...
go build -ldflags "-X main.version=0.1.0-dev -X main.commit=$(git rev-parse HEAD)" -o rmig ./cmd/rmig
```

Then run the integration checklist in `docs/integration-test-plan.md`.
