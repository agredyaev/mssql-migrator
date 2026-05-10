package commands

type Spec struct {
	Name                 string
	Usage                string
	FailureEvent         string
	RequiresSQLSelection bool
	RequiresGitCommit    bool
	RequiresPlanFile     bool
	RequiresConfirm      bool
}

const (
	Info           = "info"
	Plan           = "plan"
	Migrate        = "migrate"
	Validate       = "validate"
	Baseline       = "baseline"
	RepairChecksum = "repair-checksum"
)

var specs = []Spec{
	{Name: Info, Usage: "info --env prod", FailureEvent: "info_failed", RequiresSQLSelection: false, RequiresGitCommit: false, RequiresPlanFile: false, RequiresConfirm: false},
	{Name: Plan, Usage: "plan --env prod --sql-root ./sql --sql-base dwh", FailureEvent: "plan_failed", RequiresSQLSelection: true, RequiresGitCommit: true, RequiresPlanFile: false, RequiresConfirm: false},
	{Name: Migrate, Usage: "migrate --env prod --sql-root ./sql --sql-base dwh --plan-file reports/migration-plan.json", FailureEvent: "migration_failed", RequiresSQLSelection: true, RequiresGitCommit: true, RequiresPlanFile: true, RequiresConfirm: false},
	{Name: Validate, Usage: "validate --env prod --sql-root ./sql --sql-base dwh", FailureEvent: "validation_failed", RequiresSQLSelection: true, RequiresGitCommit: false, RequiresPlanFile: false, RequiresConfirm: false},
	{Name: Baseline, Usage: "baseline --env prod --sql-root ./sql --sql-base dwh --confirm", FailureEvent: "baseline_failed", RequiresSQLSelection: true, RequiresGitCommit: true, RequiresPlanFile: false, RequiresConfirm: true},
	{Name: RepairChecksum, Usage: "repair-checksum --env prod --sql-root ./sql --sql-base dwh --script reporting/views/monthly.sql --confirm", FailureEvent: "repair_checksum_failed", RequiresSQLSelection: true, RequiresGitCommit: true, RequiresPlanFile: false, RequiresConfirm: true},
}

var specsByName = func() map[string]Spec {
	result := make(map[string]Spec, len(specs))
	for _, spec := range specs {
		result[spec.Name] = spec
	}
	return result
}()

func Specs() []Spec {
	result := make([]Spec, len(specs))
	copy(result, specs)
	return result
}

func Names() []string {
	result := make([]string, 0, len(specs))
	for _, spec := range specs {
		result = append(result, spec.Name)
	}
	return result
}

func Lookup(name string) (Spec, bool) {
	spec, ok := specsByName[name]
	return spec, ok
}
