# Repository Guidelines

## Project Structure & Module Organization

This repository is currently minimal: the top level contains only `.codex` and no application modules, tests, or build files yet. Keep new code organized from the start:

- `src/` for application or library code
- `tests/` for automated tests
- `docs/` for design notes or contributor-facing documentation
- `assets/` for static files such as images or fixtures

Prefer small, focused modules. Example: `src/api/client.ts`, `tests/api/client.test.ts`.

## Build, Test, and Development Commands

No project-specific commands are defined yet. When adding tooling, expose a small, standard command set and document it here. Recommended baseline:

- `make test` or `npm test`: run the full test suite
- `make lint` or `npm run lint`: run static checks
- `make dev` or `npm run dev`: start the local development entry point

If you introduce a language-specific toolchain, keep commands predictable and runnable from the repository root.

## Documentation Language Rule

All user-facing documentation must be maintained in both Chinese and English. Treat this as a repository rule, not an optional preference.

- New docs must be added as a language pair in the same change
- When updating an existing doc, update its counterpart in the same commit
- Prefer paired files instead of mixed-language long-form docs
- For docs under `docs/` and similar feature docs, use `name.md` for Chinese and `name.en.md` for English
- For existing English-first root docs, add a Chinese companion such as `README.zh-CN.md` or `ROADMAP.zh-CN.md` when the file is not already bilingual

## Coding Style & Naming Conventions

Use consistent formatting and keep style automation in-repo. Until a formatter is added:

- Use 2 spaces for YAML/JSON/Markdown indentation
- Use 4 spaces for Python code
- Follow the formatter defaults for other languages once configured

Name files and directories clearly:

- `snake_case` for Python modules
- `kebab-case` for Markdown and asset filenames
- `PascalCase` for class or component names

## Testing Guidelines

Place tests under `tests/` and mirror the source layout. Use descriptive names such as `test_auth_login.py` or `client.test.ts`. Add tests with each behavior change or bug fix. If coverage tooling is introduced, target meaningful coverage on changed code rather than relying on untested paths.

## Commit & Pull Request Guidelines

This directory does not currently include Git history, so no local commit convention can be inferred. Use short, imperative commit subjects such as `Add API client scaffold` or `Fix test fixture path`.

Pull requests should include:

- a clear summary of the change
- linked issue or task reference when available
- test evidence (`npm test`, `pytest`, etc.)
- screenshots or sample output for UI or CLI changes

## Configuration Notes

Do not commit secrets, local credentials, or machine-specific config. Add environment examples through tracked templates such as `.env.example` once configuration is introduced.
