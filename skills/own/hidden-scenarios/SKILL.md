---
name: hidden-scenarios
description: Write acceptance scenarios that only Lead and Verifier see, so Builder cannot tailor the code to them. Use after the criteria are fixed, for features with behaviour a user can observe.
---

# Hidden scenarios

Visible criteria tell Builder what to build. Hidden scenarios let Verifier check that it was built for real, not just to the letter.

## Rules

- **Different from the criteria.** Same behaviour, other inputs, order, size, timing and error paths. A scenario that repeats a criterion adds nothing.
- **Observable and runnable.** Each is Given/When/Then with concrete values, and Verifier can run it through the tests or the product.
- **Cover what code tends to get wrong:** empty and maximal input, repeated and concurrent calls, a failure in the middle, permissions, a second user.
- **Three to eight scenarios.** More is noise.
- **Keep them out of the repository.** Set `hidden_scenarios_path` to where the book stores them. Never put them in a pull request, a comment or a chat message; the runner hands them only to Verifier (`jobsapi` drops them for every other role).
- Never tell Builder what a scenario contains, even when it returns for fixes: say which criterion fails and what was observed.
