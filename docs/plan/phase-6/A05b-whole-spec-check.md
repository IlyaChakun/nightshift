# Pilot a check over a whole decomposed spec

State: proposed

Source: [audit #336](https://github.com/IlyaChakun/nightshift/pull/336). Child tasks of one spec pass their own checks, but no step checks the feature they make together. Run a two-task pilot with one acceptance criterion that spans both tasks, read the tasks from the [A05a](A05a-spec-task-map.md) map, and record whether a joint check found anything the per-task checks missed. No scheduler, objective table or new role; a larger model needs the owner's decision and its own ADR.
