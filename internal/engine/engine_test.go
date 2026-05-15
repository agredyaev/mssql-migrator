package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"reporting-db-migrations/internal/apply"
	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

func TestPlan_PublishesEvents(t *testing.T) {
	b := bus.New()
	var events []types.Event
	var diffResult *types.DiffResult

	expectedPlan := &types.MigrationPlan{Command: "plan"}
	b.Subscribe(types.EventRunStarted, func(_ context.Context, p any) { events = append(events, types.EventRunStarted) })
	b.Subscribe(types.EventDiffComputed, func(_ context.Context, p any) {
		events = append(events, types.EventDiffComputed)
		diffResult = p.(*types.DiffResult)
	})
	b.Subscribe(types.EventRunFinished, func(_ context.Context, p any) { events = append(events, types.EventRunFinished) })

	eng := &Engine{
		cfg:  types.Config{SQLRoot: "/tmp/sql"},
		bus:  b,
		conn: &stubConn{},
		fs:   stubScanner{layout: fs.Layout{}},
		db:   stubInspector{state: &db.State{}},
		load: stubLoader{checksums: map[string]string{}},
		diff: stubComputer{plan: expectedPlan},
	}

	err := eng.Plan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(events), events)
	}
	if events[0] != types.EventRunStarted {
		t.Errorf("event[0] = %q, want run.started", events[0])
	}
	if events[1] != types.EventDiffComputed {
		t.Errorf("event[1] = %q, want diff.computed", events[1])
	}
	if events[2] != types.EventRunFinished {
		t.Errorf("event[2] = %q, want run.finished", events[2])
	}
	if diffResult == nil || diffResult.Plan != expectedPlan {
		t.Error("DiffResult payload should carry the plan from diff.Computer")
	}
}

type stubConn struct{}

func (s *stubConn) QueryContext(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	return nil, nil
}
func (s *stubConn) ExecContext(ctx context.Context, query string, args ...any) (driver.Result, error) {
	return nil, nil
}
func (s *stubConn) Ping(ctx context.Context) error { return nil }
func (s *stubConn) Close() error                   { return nil }

type stubScanner struct {
	layout fs.Layout
	err    error
}

func (s stubScanner) Scan(ctx context.Context, root string) (fs.Layout, error) {
	return s.layout, s.err
}

type stubInspector struct {
	state *db.State
	err   error
}

func (s stubInspector) Inspect(ctx context.Context, conn driver.Conn, scope fs.Layout) (*db.State, error) {
	return s.state, s.err
}

type stubLoader struct {
	checksums map[string]string
	err       error
}

func (s stubLoader) EnsureTables(ctx context.Context, conn driver.Conn) error { return nil }

func (s stubLoader) LoadChecksums(ctx context.Context, conn driver.Conn, keys []string) (map[string]string, error) {
	return s.checksums, s.err
}

func (s stubLoader) LoadAllAppliedMigrations(ctx context.Context, conn driver.Conn) (map[string]bool, error) {
	return nil, nil
}

type stubComputer struct {
	plan *types.MigrationPlan
	err  error
}

func (s stubComputer) Compute(ctx context.Context, layout fs.Layout, state *db.State, checksums map[string]string) (*types.MigrationPlan, error) {
	return s.plan, s.err
}

func TestPlan_ScanError_PublishesRunFinishedFailure(t *testing.T) {
	b := bus.New()
	var runFinished types.RunFinished
	b.Subscribe(types.EventRunFinished, func(_ context.Context, p any) {
		rf := p.(*types.RunFinished)
		runFinished = *rf
	})

	eng := &Engine{
		cfg:  types.Config{SQLRoot: "/bad/path"},
		bus:  b,
		conn: &stubConn{},
		fs:   stubScanner{err: assertErr("scan failed")},
		db:   stubInspector{},
		load: stubLoader{},
		diff: stubComputer{},
	}

	err := eng.Plan(context.Background())
	if err == nil {
		t.Fatal("expected error from scan failure")
	}
	if runFinished.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", runFinished.ExitCode)
	}
	if runFinished.Result != "failure" {
		t.Errorf("result = %q, want %q", runFinished.Result, "failure")
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func TestPlan_BlockedPlan_StillPublished(t *testing.T) {
	b := bus.New()
	var diffPayload *types.DiffResult
	b.Subscribe(types.EventDiffComputed, func(_ context.Context, p any) {
		diffPayload = p.(*types.DiffResult)
	})

	eng := &Engine{
		cfg:  types.Config{SQLRoot: "/tmp"},
		bus:  b,
		conn: &stubConn{},
		fs:   stubScanner{layout: fs.Layout{}},
		db:   stubInspector{state: &db.State{}},
		load: stubLoader{checksums: map[string]string{}},
		diff: stubComputer{plan: &types.MigrationPlan{
			Blocked:  true,
			Blockers: []string{"no transitions"},
		}},
	}

	err := eng.Plan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v (blocked plan is not an error)", err)
	}
	if diffPayload == nil {
		t.Fatal("expected diff.computed event")
	}
	if !diffPayload.Plan.Blocked {
		t.Error("expected plan to be blocked")
	}
	if len(diffPayload.Plan.Blockers) != 1 || diffPayload.Plan.Blockers[0] != "no transitions" {
		t.Errorf("Blockers = %v, want [no transitions]", diffPayload.Plan.Blockers)
	}
}

type stubScaffolder struct {
	ensured bool
	err     error
}

func (s *stubScaffolder) Ensure(ctx context.Context, cfg types.Config, layout fs.Layout, plan *types.MigrationPlan, columns map[string][]db.TableColumn) (bool, error) {
	s.ensured = true
	return true, s.err
}

type stubApplier struct {
	executed bool
	result   *apply.ApplyResult
	err      error
}

func (s *stubApplier) Execute(ctx context.Context, conn driver.Conn, plan types.MigrationPlan, layout fs.Layout, eb bus.EventBus) (*apply.ApplyResult, error) {
	s.executed = true
	if s.result == nil {
		s.result = &apply.ApplyResult{Applied: 1}
	}
	return s.result, s.err
}

type stubLocker struct {
	acquired bool
	released bool
	err      error
}

func (s *stubLocker) Acquire(ctx context.Context, conn driver.Conn, timeout time.Duration) error {
	s.acquired = true
	return s.err
}
func (s *stubLocker) Release(ctx context.Context, conn driver.Conn) error {
	s.released = true
	return nil
}

func TestMigrate_BlockedPlan_CallsScaffoldNotApply(t *testing.T) {
	b := bus.New()
	scaff := &stubScaffolder{}
	appl := &stubApplier{}
	lock := &stubLocker{}

	eng := &Engine{
		cfg:      types.Config{SQLBase: "/tmp", SQLRoot: "/tmp"},
		bus:      b,
		conn:     &stubConn{},
		fs:       stubScanner{layout: fs.Layout{}},
		db:       stubInspector{state: &db.State{}},
		load:     stubLoader{checksums: map[string]string{}},
		diff:     stubComputer{plan: &types.MigrationPlan{Blocked: true, Blockers: []string{"no transitions"}}},
		scaffold: scaff,
		applier:  appl,
		locker:   lock,
	}

	err := eng.Migrate(context.Background())
	if err == nil {
		t.Fatal("expected error for blocked plan")
	}
	if !scaff.ensured {
		t.Error("expected scaffold.Ensure to be called")
	}
	if lock.acquired {
		t.Error("lock should NOT be acquired for blocked plan")
	}
	if appl.executed {
		t.Error("apply should NOT be called for blocked plan")
	}
}

func TestMigrate_Success_CallsLockAndApply(t *testing.T) {
	b := bus.New()
	scaff := &stubScaffolder{}
	appl := &stubApplier{}
	lock := &stubLocker{}

	eng := &Engine{
		cfg:      types.Config{SQLBase: "/tmp", SQLRoot: "/tmp"},
		bus:      b,
		conn:     &stubConn{},
		fs:       stubScanner{layout: fs.Layout{}},
		db:       stubInspector{state: &db.State{TableColumns: map[string][]db.TableColumn{}}},
		load:     stubLoader{checksums: map[string]string{}},
		diff:     stubComputer{plan: &types.MigrationPlan{}},
		scaffold: scaff,
		applier:  appl,
		locker:   lock,
	}

	err := eng.Migrate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !lock.acquired {
		t.Error("expected lock to be acquired")
	}
	if !lock.released {
		t.Error("expected lock to be released")
	}
	if !appl.executed {
		t.Error("expected apply to be called")
	}
	if scaff.ensured {
		t.Error("scaffold should NOT be called for non-blocked plan")
	}
}

func TestValidate_PublishesValidationEvents(t *testing.T) {
	b := bus.New()
	var events []types.Event
	var validationResult *types.ValidationResult
	b.Subscribe(types.EventRunStarted, func(_ context.Context, p any) { events = append(events, types.EventRunStarted) })
	b.Subscribe(types.EventValidationStart, func(_ context.Context, p any) { events = append(events, types.EventValidationStart) })
	b.Subscribe(types.EventValidationDone, func(_ context.Context, p any) {
		events = append(events, types.EventValidationDone)
		validationResult = p.(*types.ValidationResult)
	})
	b.Subscribe(types.EventRunFinished, func(_ context.Context, p any) { events = append(events, types.EventRunFinished) })

	eng := &Engine{
		cfg:  types.Config{SQLRoot: "/tmp"},
		bus:  b,
		conn: &stubConn{},
		fs:   stubScanner{layout: fs.Layout{}},
		db:   stubInspector{state: &db.State{}},
		load: stubLoader{checksums: map[string]string{}},
		diff: stubComputer{plan: &types.MigrationPlan{
			Summary: types.PlanSummary{ChangedCount: 5},
		}},
	}

	err := eng.Validate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) < 4 {
		t.Fatalf("expected at least 4 events, got %d", len(events))
	}
	if events[0] != types.EventRunStarted {
		t.Errorf("event[0] = %q, want run.started", events[0])
	}
	if events[1] != types.EventValidationStart {
		t.Errorf("event[1] = %q, want validation.start", events[1])
	}
	if events[len(events)-1] != types.EventRunFinished {
		t.Errorf("last event = %q, want run.finished", events[len(events)-1])
	}
	if validationResult == nil || validationResult.ModulesRefreshed != 5 {
		t.Errorf("ModulesRefreshed = %d, want 5", validationResult.ModulesRefreshed)
	}
}

func TestLockedCommands_Success(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Engine, context.Context) error
	}{
		{"baseline", (*Engine).Baseline},
		{"repair", (*Engine).RepairChecksum},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := bus.New()
			appl := &stubApplier{}
			lock := &stubLocker{}

			eng := &Engine{
				cfg:     types.Config{SQLRoot: "/tmp"},
				bus:     b,
				conn:    &stubConn{},
				fs:      stubScanner{layout: fs.Layout{}},
				db:      stubInspector{state: &db.State{}},
				load:    stubLoader{checksums: map[string]string{}},
				diff:    stubComputer{plan: &types.MigrationPlan{}},
				applier: appl,
				locker:  lock,
			}

			err := tt.run(eng, context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !appl.executed {
				t.Error("expected apply to be called")
			}
			if !lock.acquired {
				t.Error("expected lock to be acquired")
			}
		})
	}
}

func TestBaseline_LockFailure(t *testing.T) {
	lockErr := errors.New("lock denied")
	eng := &Engine{
		cfg:     types.Config{SQLRoot: "/tmp"},
		bus:     bus.New(),
		conn:    &stubConn{},
		fs:      stubScanner{layout: fs.Layout{}},
		db:      stubInspector{state: &db.State{}},
		load:    stubLoader{},
		diff:    stubComputer{plan: &types.MigrationPlan{}},
		applier: &stubApplier{},
		locker:  &stubLocker{err: lockErr},
	}

	err := eng.Baseline(context.Background())
	if err == nil {
		t.Fatal("expected lock error")
	}
	if !errors.Is(err, lockErr) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBaseline_ScanFailure(t *testing.T) {
	eng := &Engine{
		cfg:  types.Config{SQLRoot: "/bad"},
		bus:  bus.New(),
		conn: &stubConn{},
		fs:   stubScanner{err: errors.New("scan failed")},
		db:   stubInspector{},
		load: stubLoader{},
		diff: stubComputer{},
	}

	err := eng.Baseline(context.Background())
	if err == nil {
		t.Fatal("expected scan error")
	}
}

func TestRepairChecksum_LockFailure(t *testing.T) {
	lockErr := errors.New("lock denied")
	eng := &Engine{
		cfg:     types.Config{SQLRoot: "/tmp"},
		bus:     bus.New(),
		conn:    &stubConn{},
		fs:      stubScanner{layout: fs.Layout{}},
		db:      stubInspector{state: &db.State{}},
		load:    stubLoader{checksums: map[string]string{}},
		diff:    stubComputer{plan: &types.MigrationPlan{}},
		applier: &stubApplier{},
		locker:  &stubLocker{err: lockErr},
	}

	err := eng.RepairChecksum(context.Background())
	if err == nil {
		t.Fatal("expected lock error")
	}
	if !errors.Is(err, lockErr) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRepairChecksum_ApplyFailure(t *testing.T) {
	applyErr := errors.New("apply failed")
	eng := &Engine{
		cfg:     types.Config{SQLRoot: "/tmp"},
		bus:     bus.New(),
		conn:    &stubConn{},
		fs:      stubScanner{layout: fs.Layout{}},
		db:      stubInspector{state: &db.State{}},
		load:    stubLoader{checksums: map[string]string{}},
		diff:    stubComputer{plan: &types.MigrationPlan{}},
		applier: &stubApplier{err: applyErr},
		locker:  &stubLocker{},
	}

	err := eng.RepairChecksum(context.Background())
	if err == nil {
		t.Fatal("expected apply error")
	}
	if !errors.Is(err, applyErr) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMigrate_ApplyFailure(t *testing.T) {
	applyErr := errors.New("SQL error")
	eng := &Engine{
		cfg:      types.Config{SQLBase: "/tmp", SQLRoot: "/tmp"},
		bus:      bus.New(),
		conn:     &stubConn{},
		fs:       stubScanner{layout: fs.Layout{}},
		db:       stubInspector{state: &db.State{TableColumns: map[string][]db.TableColumn{}}},
		load:     stubLoader{checksums: map[string]string{}},
		diff:     stubComputer{plan: &types.MigrationPlan{}},
		scaffold: &stubScaffolder{},
		applier:  &stubApplier{err: applyErr},
		locker:   &stubLocker{},
	}

	err := eng.Migrate(context.Background())
	if err == nil {
		t.Fatal("expected apply error")
	}
	if !errors.Is(err, applyErr) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMigrate_LockFailure(t *testing.T) {
	lockErr := errors.New("lock timeout")
	eng := &Engine{
		cfg:      types.Config{SQLBase: "/tmp", SQLRoot: "/tmp"},
		bus:      bus.New(),
		conn:     &stubConn{},
		fs:       stubScanner{layout: fs.Layout{}},
		db:       stubInspector{state: &db.State{TableColumns: map[string][]db.TableColumn{}}},
		load:     stubLoader{checksums: map[string]string{}},
		diff:     stubComputer{plan: &types.MigrationPlan{}},
		scaffold: &stubScaffolder{},
		applier:  &stubApplier{},
		locker:   &stubLocker{err: lockErr},
	}

	err := eng.Migrate(context.Background())
	if err == nil {
		t.Fatal("expected lock error")
	}
	if !errors.Is(err, lockErr) {
		t.Errorf("unexpected error: %v", err)
	}
}
