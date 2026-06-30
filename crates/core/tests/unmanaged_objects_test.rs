//! Safety guards: database objects absent from the repository are never planned
//! and never dropped or altered.
//!
//! These are offline (no SQL Server) tests over `compute_diff`. They lock in the
//! "safe by default" contract: the diff is repository-bounded, so an object that
//! exists in the live catalog but is not represented in the repository tree is
//! treated as unmanaged and left untouched. See `docs/repository-contract.md`.

use migrator_core::db::state::{catalog_object, CatalogState, ChecksumMap};
use migrator_core::domain::{
    share, Action, ObjectEntry, ObjectKey, Script, ScriptKey, ScriptKind, Workspace,
};
use migrator_core::gate::{evaluate_gate, GateInput, PlanSnapshot};
use migrator_core::plan::compute_diff;
use migrator_core::timings::PhaseTimings;

/// Build one repository object `(key, entry)` pair (not yet adopted into `ws`).
fn ws_object(
    ws: &mut Workspace,
    schema: &str,
    kind: &str,
    name: &str,
    checksum: [u8; 32],
) -> (ObjectKey, ObjectEntry) {
    let rel = format!("{schema}/{kind}/{name}.sql");
    let script_id = ws.insert_script(Script {
        key: ScriptKey::from_path(&rel),
        kind: ScriptKind::Object,
        abs_path: share(&rel),
        checksum: Some(checksum),
        scaffold: false,
    });
    let db_id = ws.intern_database(share("db"));
    ObjectEntry::with_staging_key(
        ObjectKey::new(schema, kind, name),
        script_id,
        checksum,
        false,
        db_id,
    )
}

/// `true` only for actions that drop or delete a database object. The exhaustive
/// match means adding a future destructive `Action` variant fails to compile,
/// forcing an explicit safety review.
fn drops_or_deletes(action: Action) -> bool {
    match action {
        Action::CreateObject
        | Action::AdoptExisting
        | Action::SkipUnchanged
        | Action::UpdateExistingModule
        | Action::ReprocessChanged
        | Action::ReprocessChangedBlocked
        | Action::Fail => false,
    }
}

#[test]
fn action_set_has_no_destructive_variant() {
    // Every representable action (repr 0..=6) must be non-destructive.
    for repr in 0u8..=6 {
        let action = Action::from_repr(repr).expect("known action repr");
        assert!(
            !drops_or_deletes(action),
            "no Action variant may drop/delete an object: {action:?}"
        );
    }
}

#[test]
fn orphan_db_object_absent_from_repo_is_not_planned() {
    let mut ws = Workspace::default();
    let managed = ws_object(&mut ws, "r", "views", "managed", [1; 32]);
    let managed_key = managed.0.clone();
    ws.adopt_dense_entries(vec![managed]);

    // Catalog has the managed object AND an orphan that exists only in the DB.
    let mut catalog = CatalogState::default();
    catalog.objects.insert(
        managed_key.clone(),
        catalog_object("r", "views", "managed", None),
    );
    let orphan_key = ObjectKey::new("r", "views", "orphan");
    catalog.objects.insert(
        orphan_key.clone(),
        catalog_object("r", "views", "orphan", None),
    );

    let checksums = ChecksumMap::new();
    let (mut plan, _) = compute_diff(&mut ws, &catalog, &checksums).unwrap();
    plan.ensure_objects_materialized(&ws);

    // The plan contains exactly the one repository object; the orphan is absent.
    assert_eq!(plan.summary.object_count, 1);
    assert_eq!(plan.objects.len(), 1);
    assert!(!plan.blocked);
    assert!(plan
        .objects
        .iter()
        .all(|o| o.normalized_key.as_ref() != orphan_key.as_str()));
    assert!(plan
        .objects
        .iter()
        .all(|o| !drops_or_deletes(o.planned_action)));
}

#[test]
fn partial_repository_adoption_leaves_unrelated_db_objects_untouched() {
    let mut ws = Workspace::default();
    let a = ws_object(&mut ws, "r", "tables", "a", [1; 32]);
    let c = ws_object(&mut ws, "r", "tables", "c", [1; 32]);
    let key_a = a.0.clone();
    let key_c = c.0.clone();
    ws.adopt_dense_entries(vec![a, c]);

    // Catalog is a superset: managed a, c plus unrelated orphans b, d.
    let mut catalog = CatalogState::default();
    for (s, k, n) in [
        ("r", "tables", "a"),
        ("r", "tables", "b"),
        ("r", "tables", "c"),
        ("r", "tables", "d"),
    ] {
        catalog
            .objects
            .insert(ObjectKey::new(s, k, n), catalog_object(s, k, n, None));
    }

    let checksums = ChecksumMap::new();
    let (mut plan, _) = compute_diff(&mut ws, &catalog, &checksums).unwrap();
    plan.ensure_objects_materialized(&ws);

    // Only the two repository objects are planned; orphans b and d never appear.
    assert_eq!(plan.summary.object_count, 2);
    assert_eq!(plan.objects.len(), 2);
    let planned: Vec<&str> = plan
        .objects
        .iter()
        .map(|o| o.normalized_key.as_ref())
        .collect();
    assert!(planned.contains(&key_a.as_str()));
    assert!(planned.contains(&key_c.as_str()));
    assert!(!planned
        .iter()
        .any(|k| k.ends_with("/b") || k.ends_with("/d")));
    assert!(plan
        .objects
        .iter()
        .all(|o| !drops_or_deletes(o.planned_action)));
    assert_eq!(plan.summary.blocked_count, 0);
    assert_eq!(plan.summary.changed_count, 0);
}

/// Build a `GateInput` with empty/identical snapshots so only `blocked` decides.
fn gate_input(blocked: bool) -> GateInput {
    GateInput {
        blocked,
        timings: PhaseTimings::default(),
        max_plan_wall_ms: 0,
        baseline: PlanSnapshot::default(),
        current: PlanSnapshot::default(),
        delta_keys: std::collections::HashSet::new(),
    }
}

#[test]
fn prod_gate_rejects_blocked_plan() {
    // A blocked plan (e.g. a destructive-precursor table change without a committed
    // transition) is fail-closed by the production gate before any apply.
    assert!(
        !evaluate_gate(gate_input(true)).passed,
        "blocked plan must be NO-GO"
    );
    // A clean, unchanged plan passes (sanity: the gate is not unconditionally failing).
    assert!(
        evaluate_gate(gate_input(false)).passed,
        "clean plan must be GO"
    );
}

#[test]
fn repository_removal_does_not_drop_object() {
    // The repository no longer declares any object (it was removed), but the DB
    // still has it and it was previously managed (has a prior checksum).
    let mut ws = Workspace::default();

    let removed_key = ObjectKey::new("r", "procedures", "gone");
    let mut catalog = CatalogState::default();
    catalog.objects.insert(
        removed_key.clone(),
        catalog_object("r", "procedures", "gone", None),
    );
    let mut checksums = ChecksumMap::new();
    checksums.insert_key(&removed_key, [9; 32]);

    let (mut plan, _) = compute_diff(&mut ws, &catalog, &checksums).unwrap();
    plan.ensure_objects_materialized(&ws);

    // Absence from the repository produces no plan entry — and therefore no drop.
    assert_eq!(plan.summary.object_count, 0);
    assert!(plan.objects.is_empty());
    assert!(!plan.blocked);
}
