use super::command_mutates;
use super::types::Command;

#[test]
fn only_mutating_commands_lock_and_apply() {
    // Mutating commands run inside the advisory lock; read-only commands must not.
    for c in [Command::Migrate, Command::Baseline, Command::RepairChecksum] {
        assert!(command_mutates(c), "{c:?} must run under the advisory lock");
    }
    for c in [Command::Plan, Command::Validate, Command::Version] {
        assert!(!command_mutates(c), "{c:?} is read-only");
    }
}
