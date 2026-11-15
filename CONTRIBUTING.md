# Contributing

Thank you for your interest in Freeloop. Everyone taking part follows the [Code of Conduct](CODE_OF_CONDUCT.md).

**External code contributions open after v1 scoping.** Until then the design is still moving quickly and pull requests from outside are not reviewed or merged. What helps now:

- **Issues.** Bugs, crashes, and agents, providers or MCP servers that do not work as expected. Include the system and its version (macOS, Windows or Linux), how you built or installed the app, and the steps. Leave API keys and other secrets out of logs and transcripts you attach.
- **Security reports.** Do not open a public issue; follow [docs/SECURITY.md](docs/SECURITY.md).
- **Presets and skills.** An ACP agent preset or an Agent Skill that works for you can be shared as a file in an issue; adding either needs no change to the app.

## When contributions open

The rules are set now (OQ-017 in [docs/OPEN_QUESTIONS.md](docs/OPEN_QUESTIONS.md)), so nothing changes for a contributor when they open:

- **Developer Certificate of Origin.** Every commit carries a sign-off by its author, `Signed-off-by: Your Name <you@example.com>`, which `git commit -s` adds. It states that you may contribute the change under the project's license, Apache-2.0, as the [Developer Certificate of Origin 1.1](https://developercertificate.org/) says. There is no contributor license agreement. CI refuses a pull request with a commit whose sign-off is missing or names someone other than its author.
- **Code of conduct.** Everyone in the project's issues, pull requests and discussions follows the [Code of Conduct](CODE_OF_CONDUCT.md), the Contributor Covenant 2.1. A violation is reported privately, through GitHub's private reporting on this repository, as it says.

## For the record: how the code is worked on

[docs/DEVELOPING.md](docs/DEVELOPING.md) explains how the code fits together and how to add a provider, a harness preset, an environment or an event type.

- Requirements have stable IDs (`PRV-001`, `SEC-003`…) in [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md); commit messages and pull request titles name the IDs they deliver.
- When the implementation proves a document wrong, the document changes in the same commit. Decisions the documents do not settle go to [docs/OPEN_QUESTIONS.md](docs/OPEN_QUESTIONS.md) with options and a recommendation.
- `make test` and `make lint` pass before every commit. The lint includes an architecture test: no process or file access outside `internal/environment` (and the store, config and platform packages), Wails only in `cmd/freeloop`, system-specific code only in `internal/platform/{darwin,linux,windows}` and the environment layer's per-system files.
- New dependencies are added only after agreement; npm dependencies stay few and pinned.
