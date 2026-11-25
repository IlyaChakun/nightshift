# TypeScript style

Tools enforce what they can (`tsc --strict`, the project's linter and formatter). This page holds the
reminders no linter checks. Source: [Google TypeScript Style Guide](https://google.github.io/styleguide/tsguide.html).

- Follow the project's package manager, test runner and layout first; do not add a dependency.
- No `any`; use `unknown` and narrow it. Validate data from the network or disk at the boundary.
- Prefer `const`, `readonly` and discriminated unions; switch over them exhaustively.
- Handle every promise: `await` it or return it; no floating promises, no empty `catch`.
- Throw `Error` subclasses, not strings. Do not swallow errors to make a test pass.
- Do not touch `process.env` outside one config module; never log secrets.
- Test behaviour, not implementation; a bug fix gets a regression test.
- Comments say why, not what. Exported names get a doc comment.
