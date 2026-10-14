# Landbound implementation plan

Scope: standalone Linux/Go TUI, fixture-only. Build narrow path-read demo first; no arbitrary command runner, no network policy, no claim of comprehensive sandboxing.

## Files
- `go.mod`: standalone module; Go version compatible with installed toolchain.
- `cmd/landbound/main.go`: command dispatch (`tui`, `demo`, `doctor`) and private child mode.
- `internal/landlock/landlock_linux.go`: ABI detection and filesystem read ruleset syscalls.
- `internal/landlock/landlock_linux_test.go`: ABI/error and enforcement tests.
- `internal/demo/demo.go`, `internal/demo/demo_test.go`: private fixture, baseline/restricted child execution, bounded output, cleanup, parsed evidence.
- `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/model_test.go`: Bubble Tea rule toggles, test action, status, help and compact tree/matrix.
- `README.md`: prerequisites, commands, demonstration, limitations and troubleshooting.

## Tasks

1. **Initialize module and Landlock ABI probe.** Create module; implement version-query wrapper using `golang.org/x/sys/unix`, minimum ABI check, typed unsupported/error result. Add tests for mapping supported ABI and syscall errors. Run `go test ./internal/landlock`.
2. **Apply read-only rules in child.** Create ruleset with supported read rights; open intended paths via `O_PATH`; add path-beneath rules; call `prctl(PR_SET_NO_NEW_PRIVS)` then `landlock_restrict_self`; execute only fixed public/private reads and return structured results. Test fail-closed behavior and real enforcement when kernel supports Landlock; skip only explicitly unsupported host.
3. **Build deterministic demo CLI.** Create mode-0700 temp fixture; run unrestricted baseline then child under selected policy; child only invoked by current executable in private mode, with timeout/output cap; clean up; report baseline and restricted results. `demo` toggles whether private file is allowed via flag for repeatable before/after. Unit-test cleanup, parsing and failures; integration-test actual enforcement.
4. **Add doctor and TUI.** `doctor` reports kernel ABI/support. Bubble Tea TUI shows two fixture paths, public/private read rule, latest baseline/restricted result; space toggles private rule, enter runs, `?` help, `q` exits. Keep child restriction out of parent process. Unit-test toggles and result state; build/test.
5. **Document and final gate.** Write usage, support, accurate security claims, ABI constraints and troubleshooting. Run `gofmt`, `go test ./...`, `go vet ./...`, `go build ./...`, `doctor`, default demo and private-allowed demo; inspect actual outcome, fix issues, commit.

## Review focus
- Parent never restricts itself; private child mode cannot recurse or accept arbitrary command/path input.
- Baseline verifies both fixture reads first; report Landlock denial only when baseline works and restricted public works/private fails with `EACCES`.
- Only rights supported by reported ABI are requested; no unsupported rights panic.
- Temporary fixture cleanup and child timeout/output bounds apply on every exit path.
- Platform-specific syscall files and tests compile only on Linux; unsupported kernel exits with clear message, not false success.

## Self-review
All agreed fixture-only behavior covered. Network rights and arbitrary command mode excluded. Headless demo permits kernel integration proof without TUI automation. Tests exercise real enforcement on this host and clearly skip unsupported hosts. Task order follows syscall API → child execution → CLI → TUI → final verification.
