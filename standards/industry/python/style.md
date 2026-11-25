# Python style

Tools enforce what they can (`ruff`, `mypy --strict`; the project's `verify` runs them). This page
holds the reminders no linter checks. Sources: [PEP 8](https://peps.python.org/pep-0008/),
[Google Python Style Guide](https://google.github.io/styleguide/pyguide.html).

- Follow the project's own tooling and layout first (uv, pytest, `pyproject.toml`); do not add a tool.
- Type every public function. Use `X | None`, not `Optional[X]`; no bare `Any` without a comment.
- Raise specific exceptions; never `except:` or `except Exception: pass`. Wrap with `raise ... from err`.
- Use `pathlib`, `subprocess.run([...], check=True)` with a list (never `shell=True` with input), context managers for files.
- No mutable default arguments; no global mutable state; no import-time side effects.
- Parsers and data code get table-driven tests (`pytest.mark.parametrize`); fix a bug with a regression test.
- Comments say why, not what. Docstrings on public names, one summary line first.
