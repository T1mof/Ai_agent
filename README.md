# Ai_agent

Ai_agent is an MVP Go code quality agent.

## Current features
- load spec.yaml
- scan Go project structure
- parse AST and collect functions
- run go vet
- run staticcheck
- run one AST analyzer
- generate fixes_report.json

## Run
```bash
go mod tidy
go run ./cmd/ai_agent --spec ./examples/spec.yaml --project /path/to/project


---

## Что получится после запуска

Команда:

```bash
go mod tidy
go run ./cmd/ai_agent --spec ./examples/spec.yaml --project ./some_go_project


go run ./cmd/ai_agent reset-demo --project ./examples/demo_project
go run ./cmd/ai_agent dry-run --spec ./examples/spec.yaml --project ./examples/demo_project --report ./examples/demo_project/dry_run_report.json