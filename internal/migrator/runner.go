package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/planner"
	"reporting-db-migrations/internal/reports"
	"reporting-db-migrations/internal/runreport"
)

type DBOpener interface {
	Open(context.Context, config.Config) (*sql.DB, error)
}

type defaultDBOpener struct{}

func (defaultDBOpener) Open(ctx context.Context, cfg config.Config) (*sql.DB, error) {
	return db.Open(ctx, cfg)
}

type Runner struct {
	cfg config.Config
	log logger.Logger
	db  DBOpener
}

func NewRunner(cfg config.Config, log logger.Logger) Runner {
	return NewRunnerWithDBOpener(cfg, log, nil)
}

func NewRunnerWithDBOpener(cfg config.Config, log logger.Logger, opener DBOpener) Runner {
	if opener == nil {
		opener = defaultDBOpener{}
	}
	return Runner{cfg: cfg, log: log, db: opener}
}

func (r Runner) Info(ctx context.Context) error {
	database, err := r.db.Open(ctx, r.cfg)
	if err != nil {
		return contracts.Wrap(contracts.ErrConnection, err)
	}
	defer database.Close()
	r.log.Info("connection_ok", r.cfg.MaskedTarget())
	return nil
}

func (r Runner) Plan(ctx context.Context) (contracts.MigrationPlan, error) {
	layout, hash, err := planner.ResolvePlanningLayoutForRunner(r.cfg)
	if err != nil {
		return contracts.MigrationPlan{}, err
	}
	conn, closeFn, err := r.openReservedConnection(ctx)
	if err != nil {
		return contracts.MigrationPlan{}, err
	}
	defer func() {
		if err := closeFn(); err != nil {
			r.log.Warn("db_close_failed", err.Error())
		}
	}()
	successfulByKey, err := metadata.LoadSuccessfulChecksumsIfPresent(ctx, conn)
	if err != nil {
		return contracts.MigrationPlan{}, contracts.Wrap(contracts.ErrCriticalState, err)
	}
	plan, err := planner.BuildResolved(ctx, r.cfg, successfulByKey, layout, hash, planner.SQLCatalogReader(conn))
	if err != nil {
		return contracts.MigrationPlan{}, err
	}
	catalogState, err := planner.SQLCatalogReader(conn).ReadCatalogState(ctx)
	if err != nil {
		return contracts.MigrationPlan{}, contracts.Wrap(contracts.ErrCriticalState, err)
	}
	if created, scaffoldErr := ensureTableTransitionFiles(r.cfg, layout, plan, catalogState.TableColumns); scaffoldErr != nil {
		return contracts.MigrationPlan{}, contracts.Wrap(contracts.ErrInvalidInput, scaffoldErr)
	} else if created {
		layout, hash, err = planner.ResolvePlanningLayoutForRunner(r.cfg)
		if err != nil {
			return contracts.MigrationPlan{}, err
		}
		plan, err = planner.BuildResolved(ctx, r.cfg, successfulByKey, layout, hash, planner.SQLCatalogReader(conn))
		if err != nil {
			return contracts.MigrationPlan{}, err
		}
	}
	if err := reports.WritePlan(r.cfg.ReportDir, plan); err != nil {
		return contracts.MigrationPlan{}, err
	}
	return plan, nil
}

func (r Runner) Migrate(ctx context.Context) error {
	state, err := r.startProtectedRunState(ctx, "migration_failed")
	if err != nil {
		return err
	}
	defer state.Close()

	planCtx, err := r.prepareMigrationExecution(ctx, state)
	if err != nil {
		return err
	}
	if err := r.executeMigrationPlan(ctx, state, planCtx); err != nil {
		return err
	}
	if err := r.validateMigrationScope(ctx, state, planCtx); err != nil {
		return err
	}
	return state.finishSuccess(ctx)
}

type executionPlanContext struct {
	layout  plannerLayoutContext
	plan    contracts.MigrationPlan
	itemIDs map[string]int64
}

type plannerLayoutContext struct {
	resolved parser.Layout
	hash     string
}

func (r Runner) prepareMigrationExecution(ctx context.Context, state *protectedRunState) (executionPlanContext, error) {
	planCtx, err := r.prepareExecutionPlan(ctx, state, executionPlanOptions{
		layoutBase:     contracts.ErrInvalidInput,
		buildPlanError: classifyMigrationPlanBuildError,
		startRun: startRunOptions{
			command:  contracts.CommandMigrate,
			planFile: r.cfg.PlanFile,
		},
	})
	if err != nil {
		return executionPlanContext{}, err
	}
	plan := planCtx.plan
	if err := r.verifyMigrationPlan(plan); err != nil {
		return executionPlanContext{}, state.fail(ctx, contracts.ErrInvalidInput, err)
	}
	r.log.Info("rollback_scope", fmt.Sprintf("Rollback scope: %s. Previous successful scripts remain committed. Use database backups or restore points for full recovery guarantees.", plan.Rollback))
	return planCtx, nil
}

func (r Runner) executeMigrationPlan(ctx context.Context, state *protectedRunState, planCtx executionPlanContext) error {
	return state.executeTrackedPlan(ctx, r, planCtx.layout.resolved, planCtx.plan, planCtx.itemIDs)
}

func (r Runner) validateMigrationScope(ctx context.Context, state *protectedRunState, planCtx executionPlanContext) error {
	report := state.session.MigrationReport()
	if r.cfg.SkipValidate {
		report.ValidationSkipped = true
		return nil
	}
	validationReport, validationErr := r.validateManagedScope(ctx, state.session, planCtx.layout.resolved, state.runID)
	report.ValidationScope = validationReport.Scope
	report.Validation = validationReport.Validation
	if writeErr := runreport.WriteValidation(r.cfg.ReportDir, validationReport); writeErr != nil {
		return state.fail(ctx, contracts.ErrCriticalState, writeErr)
	}
	if validationErr != nil {
		base := contracts.ErrValidation
		if errors.Is(validationErr, contracts.ErrCriticalState) {
			base = contracts.ErrCriticalState
		}
		return state.fail(ctx, base, validationErr)
	}
	return nil
}

func (r Runner) loadProtectedPlanningInputs(ctx context.Context, state *protectedRunState, layoutBase error) (plannerLayoutContext, map[string]string, error) {
	if err := state.session.BootstrapMetadata(ctx); err != nil {
		return plannerLayoutContext{}, nil, state.fail(ctx, contracts.ErrCriticalState, err)
	}
	successfulByKey, err := state.session.LoadSuccessfulChecksums(ctx)
	if err != nil {
		return plannerLayoutContext{}, nil, state.fail(ctx, contracts.ErrCriticalState, err)
	}
	resolved, hash, err := state.session.ResolvePlanningLayout()
	if err != nil {
		return plannerLayoutContext{}, nil, state.fail(ctx, layoutBase, err)
	}
	state.setLayoutHash(hash)
	return plannerLayoutContext{resolved: resolved, hash: hash}, successfulByKey, nil
}

type executionPlanOptions struct {
	layoutBase     error
	buildPlanError func(error) error
	startRun       startRunOptions
}

type startRunOptions struct {
	command  string
	planFile string
	planHash string
}

func (r Runner) prepareExecutionPlan(ctx context.Context, state *protectedRunState, options executionPlanOptions) (executionPlanContext, error) {
	layout, successfulByKey, err := r.loadProtectedPlanningInputs(ctx, state, options.layoutBase)
	if err != nil {
		return executionPlanContext{}, err
	}
	plan, err := state.session.BuildPlan(ctx, successfulByKey, layout.resolved, layout.hash)
	if err != nil {
		return executionPlanContext{}, state.fail(ctx, options.buildPlanError(err), err)
	}
	catalogState, err := state.session.ReadPlanningCatalog(ctx)
	if err != nil {
		return executionPlanContext{}, state.fail(ctx, contracts.ErrCriticalState, err)
	}
	if created, scaffoldErr := ensureTableTransitionFiles(r.cfg, layout.resolved, plan, catalogState.TableColumns); scaffoldErr != nil {
		return executionPlanContext{}, state.fail(ctx, contracts.ErrInvalidInput, scaffoldErr)
	} else if created {
		layout, successfulByKey, err = r.loadProtectedPlanningInputs(ctx, state, options.layoutBase)
		if err != nil {
			return executionPlanContext{}, err
		}
		plan, err = state.session.BuildPlan(ctx, successfulByKey, layout.resolved, layout.hash)
		if err != nil {
			return executionPlanContext{}, state.fail(ctx, options.buildPlanError(err), err)
		}
	}
	if plan.Blocked {
		return executionPlanContext{}, state.fail(ctx, contracts.ErrChecksumMismatch, fmt.Errorf("%v", plan.BlockReasons))
	}
	if options.startRun.planFile != "" && options.startRun.planHash == "" {
		planHash, err := planArtifactHash(options.startRun.planFile)
		if err != nil {
			return executionPlanContext{}, state.fail(ctx, contracts.ErrInvalidInput, err)
		}
		options.startRun.planHash = planHash
	}
	if err := state.startRun(ctx, options.startRun.command, options.startRun.planFile, options.startRun.planHash, plan.Rollback); err != nil {
		return executionPlanContext{}, state.fail(ctx, contracts.ErrCriticalState, err)
	}
	itemIDs, err := state.recorder.scope.Migration(ctx, plan)
	if err != nil {
		return executionPlanContext{}, state.fail(ctx, contracts.ErrCriticalState, err)
	}
	return executionPlanContext{layout: layout, plan: plan, itemIDs: itemIDs}, nil
}

func classifyMigrationPlanBuildError(err error) error {
	if errors.Is(err, contracts.ErrCriticalState) {
		return contracts.ErrCriticalState
	}
	return contracts.ErrInvalidInput
}

func (r Runner) verifyMigrationPlan(plan contracts.MigrationPlan) error {
	if strings.TrimSpace(r.cfg.PlanFile) == "" {
		return nil
	}
	return planner.VerifyApprovedPlan(r.cfg, plan)
}

func (r Runner) openReservedConnection(ctx context.Context) (*sql.Conn, func() error, error) {
	database, err := r.db.Open(ctx, r.cfg)
	if err != nil {
		return nil, nil, contracts.Wrap(contracts.ErrConnection, err)
	}
	conn, err := database.Conn(ctx)
	if err != nil {
		_ = database.Close()
		return nil, nil, contracts.Wrap(contracts.ErrConnection, err)
	}
	closeFn := func() error {
		connErr := conn.Close()
		dbErr := database.Close()
		if connErr != nil {
			return connErr
		}
		return dbErr
	}
	return conn, closeFn, nil
}

func (r Runner) newMigrationReport() contracts.MigrationReport {
	return contracts.MigrationReport{
		Tool:              toolName,
		Version:           r.cfg.ToolVersion,
		ToolCommit:        r.cfg.ToolCommit,
		Environment:       r.cfg.Env,
		Database:          r.cfg.Database,
		GitCommit:         r.cfg.GitCommit,
		GitBranch:         r.cfg.GitBranch,
		SQLRoot:           r.cfg.SQLRoot,
		Base:              r.cfg.SQLBase,
		EffectiveBasePath: r.cfg.SelectedBasePath(),
		PipelineRunID:     r.cfg.PipelineRunID,
		PipelineURL:       logger.Redact(r.cfg.PipelineURL),
		Actor:             r.cfg.Actor,
		StartedAt:         time.Now().UTC(),
		Result:            "running",
		Applied:           []contracts.ScriptResult{},
		Skipped:           []contracts.ScriptResult{},
	}
}
