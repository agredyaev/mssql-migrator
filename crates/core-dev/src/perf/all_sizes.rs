use std::mem::size_of;

use migrator_core::apply::ApplyResult;
use migrator_core::audit::HistoryRecord;
use migrator_core::config::{Config, ConfigCold};
use migrator_core::db::state::{CatalogObject, CatalogState, TableColumn};
use migrator_core::db::ChecksumMap;
use migrator_core::db::PlanDbResult;
use migrator_core::db::PlanDbTrace;
use migrator_core::domain::{
    ObjectEntry, ObjectKey, ObjectRow, ParentRef, SchemaEntry, Script, ScriptGit,
    ScriptKey, ScriptRow, SharedStr, StrOff, TransitionEntry, Workspace, WorkspaceCold,
};
use migrator_core::driver::{IoProfile, MssqlConn, RowData, TimingConn};
use migrator_core::engine::RunOutput;
use migrator_core::export::{
    MigrationPlan, PlanGitOff, PlanRow, PlanSummary, PlannedGit, PlannedObject, PlannedSchema,
    RunFinished,
};
use migrator_core::gate::{
    ChangedPathsResult, CompareOptions, CompareResult, E2EApplyReport, E2EBlockedReport,
    E2EGateReport, E2EScenarioReport, GateInput, GateResult, PlanSnapshot, SnapshotObject,
};
use migrator_core::git::GitMeta;
use migrator_core::plan::InspectScope;
use migrator_core::scaffold::ParsedColumn;
use migrator_core::session::{ProxyClient, Response};
use migrator_core::timings::PhaseTimings;

pub fn all_struct_size_lines() -> Vec<String> {
    let rows: &[(&str, &str, usize)] = &[
        ("domain", "WorkspaceCold", size_of::<WorkspaceCold>()),
        ("export", "MigrationPlan", size_of::<MigrationPlan>()),
        ("config", "ConfigCold", size_of::<ConfigCold>()),
        ("timings", "PhaseTimings", size_of::<PhaseTimings>()),
        ("config", "Config", size_of::<Config>()),
        ("export", "PlannedObject", size_of::<PlannedObject>()),
        ("gate", "PlanSnapshot", size_of::<PlanSnapshot>()),
        ("domain", "Workspace", size_of::<Workspace>()),
        ("gate", "SnapshotObject", size_of::<SnapshotObject>()),
        ("domain", "ObjectEntry", size_of::<ObjectEntry>()),
        ("export", "PlanSummary", size_of::<PlanSummary>()),
        ("domain", "ScriptRow", size_of::<ScriptRow>()),
        ("export", "PlanRow", size_of::<PlanRow>()),
        ("export", "PlanGitOff", size_of::<PlanGitOff>()),
        ("export", "PlannedGit", size_of::<PlannedGit>()),
        ("export", "PlannedSchema", size_of::<PlannedSchema>()),
        ("domain", "ObjectRow", size_of::<ObjectRow>()),
        ("domain", "ParentRef", size_of::<ParentRef>()),
        ("domain", "SchemaEntry", size_of::<SchemaEntry>()),
        ("domain", "ScriptGit", size_of::<ScriptGit>()),
        ("domain", "Script", size_of::<Script>()),
        ("domain", "TransitionEntry", size_of::<TransitionEntry>()),
        ("domain", "SharedStr", size_of::<SharedStr>()),
        ("domain", "ObjectKey", size_of::<ObjectKey>()),
        ("domain", "ScriptKey", size_of::<ScriptKey>()),
        ("domain", "StrOff", size_of::<StrOff>()),
        ("db", "CatalogState", size_of::<CatalogState>()),
        ("db", "CatalogObject", size_of::<CatalogObject>()),
        ("db", "TableColumn", size_of::<TableColumn>()),
        ("db", "ChecksumMap", size_of::<ChecksumMap>()),
        ("db", "PlanDbResult", size_of::<PlanDbResult>()),
        ("db", "PlanDbTrace", size_of::<PlanDbTrace>()),
        ("driver", "TimingConn", size_of::<TimingConn>()),
        ("driver", "IoProfile", size_of::<IoProfile>()),
        ("driver", "MssqlConn", size_of::<MssqlConn>()),
        ("driver", "RowData", size_of::<RowData>()),
        ("engine", "RunOutput", size_of::<RunOutput>()),
        ("export", "RunFinished", size_of::<RunFinished>()),
        ("gate", "GateResult", size_of::<GateResult>()),
        ("gate", "GateInput", size_of::<GateInput>()),
        ("gate", "CompareOptions", size_of::<CompareOptions>()),
        ("gate", "CompareResult", size_of::<CompareResult>()),
        ("gate", "ChangedPathsResult", size_of::<ChangedPathsResult>()),
        ("gate", "E2EScenarioReport", size_of::<E2EScenarioReport>()),
        ("gate", "E2EApplyReport", size_of::<E2EApplyReport>()),
        ("gate", "E2EGateReport", size_of::<E2EGateReport>()),
        ("gate", "E2EBlockedReport", size_of::<E2EBlockedReport>()),
        ("git", "GitMeta", size_of::<GitMeta>()),
        ("plan", "InspectScope", size_of::<InspectScope>()),
        ("scaffold", "ParsedColumn", size_of::<ParsedColumn>()),
        ("session", "ProxyClient", size_of::<ProxyClient>()),
        ("session", "Response", size_of::<Response>()),
        ("audit", "HistoryRecord", size_of::<HistoryRecord>()),
        ("apply", "ApplyResult", size_of::<ApplyResult>()),
    ];
    rows.iter()
        .map(|(pkg, name, bytes)| format!("{pkg}::{name}: {bytes} B"))
        .collect()
}
