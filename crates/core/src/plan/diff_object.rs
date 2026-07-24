use crate::domain::{with_database_prefix, Action, Workspace};
use crate::export::{PlannedGit, PlannedObject};

#[derive(Clone, Copy)]
pub(crate) struct ObjectDecision {
    pub action: Action,
    pub with_git: bool,
    pub exists: bool,
}

pub(crate) fn reuse_unchanged_object(
    ws: &Workspace,
    index: usize,
    decision: ObjectDecision,
    planned: &PlannedObject,
) -> bool {
    if decision.action != Action::SkipUnchanged
        || decision.with_git
        || planned.planned_action != decision.action
        || planned.exists != decision.exists
        || planned.git.is_some()
        || !planned.transition_paths.is_empty()
    {
        return false;
    }
    let object = ws.entry(index);
    planned.checksum == object.checksum && planned.normalized_key == object.key.as_str()
}

pub(crate) fn planned_object(
    ws: &Workspace,
    index: usize,
    decision: ObjectDecision,
) -> PlannedObject {
    let object = ws.entry(index);
    let database = ws.database_name(object.db_id);
    let git = decision
        .with_git
        .then(|| {
            let script = ws.script(object.script_id);
            PlannedGit {
                hash: script.git_hash().to_owned(),
                author: script.git_author().to_owned(),
                date: script.git_date().to_owned(),
            }
        })
        .filter(|value| {
            !value.hash.is_empty() || !value.author.is_empty() || !value.date.is_empty()
        });
    let transition_paths = if decision.action == Action::ReprocessChanged {
        object
            .transitions
            .iter()
            .map(|transition| {
                with_database_prefix(database, ws.script(transition.script_id).path_str())
            })
            .collect()
    } else {
        Vec::new()
    };
    PlannedObject {
        normalized_key: object.key.shared(),
        object_path: ws.object_path_at(index),
        schema_name: object.key.schema_shared(),
        kind: object.key.kind_shared(),
        object_name: object.key.name_shared(),
        database_name: database.to_owned(),
        parent_name: object.parent_name(ws),
        transition_paths,
        git,
        checksum: object.checksum,
        planned_action: decision.action,
        exists: decision.exists,
    }
}
