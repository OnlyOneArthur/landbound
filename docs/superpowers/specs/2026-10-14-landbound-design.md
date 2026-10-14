# Landbound — design

## Intent and scope
Standalone Go/Linux portfolio TUI, offline and unprivileged. An operator edits a small, explicit allowlist and runs a controlled fixture to see what succeeds and fails under Landlock. A 60-second demonstration contrasts a permitted read with a denied read, then changes the rule and reruns. This is a policy design/experiment tool, not an audit monitor or general-purpose sandbox launcher. v1 runs only the bundled fixture; arbitrary-command execution, persistent policies, write operations, network rights, privilege elevation, and denied-path event capture are out of scope. Network rights are reserved for a future ABI-aware iteration; filesystem rights alone give a reproducible demo with no external services.

## Alternatives and choice
- **Go with Bubble Tea and `golang.org/x/sys/unix` (chosen):** reuse existing Go/TUI skills, fast iteration, small native binary; implement narrow Linux syscall wrappers and review carefully.
- **Go with external `landlock-restrict` process:** fewer syscalls in our code, but external dependency and hard-to-explain provenance of enforcement.
- **Rust with `landlock`/`ratatui`:** excellent type-safe API, but slower for this developer's 2–4 week horizon; language already decided.

## Interface and data flow
- `landbound` launches full-screen TUI; `landbound demo` runs deterministic headless A/B proof for CI and non-interactive verification; `landbound doctor` reports kernel ABI and support status. Linux amd64 first; unsupported kernels fail closed with actionable explanation.
- The app creates a private temporary fixture root: `public/message.txt`, `private/secret.txt`; child process will attempt reads under the policy. One fixed scenario: public allowed, private initially denied; toggling private rule in TUI permits it on the next run.
- Parent TUI never applies Landlock to itself. It spawns its own executable in an unexported child mode, passes fixture paths/rule selection over dedicated arguments, and captures bounded stdout/stderr. Child sets `no_new_privs`, creates a ruleset, adds allowed path-beneath rules, calls `landlock_restrict_self`, then reads fixture files. Restriction happens before file access, only inside child. Child output is structured for deterministic parsing; no arbitrary paths or commands supplied by TUI to child.
- `internal/landlock` probes ABI via `landlock_create_ruleset` version query; handles ABI 1 filesystem read rights (`READ_FILE`, `READ_DIR`) and required `EXECUTE` only if needed for runtime. Open O_PATH descriptors for allowed directories/files before restriction; limit access to the intended fixture subtree. Explicitly handle unsupported ABI, syscall errors and missing rights. No security decisions based on output text.
- `internal/demo` owns fixture setup, spawning, typed results, cleanup. It only executes current binary and never accepts externally supplied commands. `internal/tui` owns state, keystrokes, and a compact matrix of public/private file access with latest test results. It does not need a large tree explorer to explain two paths; short visual folder tree + rule toggles suffice. Accessible text labels and a help key; exit restores terminal state.

## Security and truthful presentation
- An allowlist granted for a path is inherited down its tree; never claim the UI displays all kernel denials. Child explicitly tries known files and prints their observed results (`success`, `EACCES`/other error). Landlock stacks with other restrictions; a failed read does not prove Landlock was sole cause unless baseline read succeeds. Thus `demo` first verifies an unrestricted baseline for both files, then restricted run, and reports inability to establish proof distinctly.
- Do not accept arbitrary target commands/paths, follow untrusted links, or create an ambient privilege boundary. Temporary root mode 0700, files fixture-owned. Guard against child recursion and bound its duration and output. Disallow access to the private fixture by omission from allowlist; public read must work after restriction.
- Landlock is best-effort LSM hardening; this exercise proves specific read attempts on this kernel only, not whole-process confidentiality or absence of bypasses. `doctor` and README explain ABI/LSM availability and why no denial event stream exists.

## Verification
- Unit tests for ABI/error handling, state transitions and result parsing; headless `demo` integration test executes real child on supported host, skips with reason on unsupported host.
- `go test ./...`, `go vet ./...`, `go build ./...`; run `demo` and `doctor` on target kernel, inspect output for baseline success/public success/private denial; optionally manual TUI inspection.
- Keep Git history local unless explicitly asked to publish or push.
