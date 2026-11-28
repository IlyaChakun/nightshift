---
name: hidden-scenarios
description: Write acceptance scenarios that only Lead and Verifier see, so Builder cannot tailor the code to them. Use after the criteria are fixed, for features with behaviour a user can observe.
---

# Hidden scenarios

Visible criteria tell Builder what to build. Hidden scenarios let Verifier check that it was built for real, not just to the letter.

## Rules

- **Different from the criteria.** Same behaviour, other inputs, order, size, timing and error paths. A scenario that repeats a criterion adds nothing.
- **Observable in the preview.** Each is Given/When/Then with concrete values, and a person in a browser on the task's preview can run it and see the result. Verifier's acceptance job has only that browser: no shell, no Git, no file reads of the change. It opens no other URL, so production and the state after a release or a rollback are out too. A criterion about a diff, file bytes (an encoding, a BOM, line endings), a log, a test or production belongs in the visible criteria, where review and the checks cover it, not here.
- **Cover what code tends to get wrong:** empty and maximal input, repeated and concurrent calls, a failure in the middle, permissions, a second user.
- **Three to eight scenarios.** More is noise.
- **Keep them out of the repository.** Put them in `hidden_scenarios` of your reply (ids `HS-1`, `HS-2`, with the spec), not in the spec. `nightshift` stores them in the book for Lead and Verifier; never put them in a pull request, a comment or a chat message (`jobsapi` drops them for every other role).
- Never tell Builder what a scenario contains, even when it returns for fixes: say which criterion fails and what was observed.
