# Landbound

A small Go/Bubble Tea TUI for **designing and observing Linux [Landlock](https://docs.kernel.org/userspace-api/landlock.html) rules** against a bundled fixture.

Landbound builds a throwaway fixture with one public and one private file, runs a bundled child process under a Landlock allowlist, and shows you exactly which reads succeeded and which were denied. On kernels with Landlock ABI 4+ it also attempts one loopback TCP connect under a network rule. Toggle a rule, re-run, and watch the result change.

This is a **policy design and experiment tool**, not a sandbox launcher and not an audit monitor.

```
LANDBOUND

Fixture tree                 Rule
├── public/
│   └── message.txt          ✓ read
└── private/
    └── secret.txt           DENIED (default)

Loopback TCP connect         DENIED (default)

[SPACE] private rule   [N] network rule   [ENTER] run   [?] help   [Q] quit

Landlock ABI 10
restricted results: public ✓ success | private ✗ permission denied
network: loopback connect denied — connect: permission denied
```

## Requirements

- Linux (amd64 first). Landlock is a Linux LSM.
- A kernel with Landlock enabled. `landbound doctor` reports what your kernel supports.
- Go 1.27+ to build from source.

Landlock has been in the mainline kernel since 5.13. Landbound needs **ABI 1** (filesystem read rights) for its file scenario and **ABI 4** for its network scenario. If your kernel or LSM configuration does not support Landlock, `landbound doctor` and every command **fail closed with an explanation** rather than reporting a false success. On a kernel below ABI 4 the network scenario is reported as **unsupported**, never guessed at.

## Build

```sh
go build ./cmd/landbound
```

## Usage

```sh
landbound                              # launch the interactive TUI
landbound demo                         # headless run, private file and TCP connect denied
landbound demo --allow-private         # headless run, private file allowed
landbound demo --allow-net             # headless run, loopback TCP connect allowed
landbound doctor                       # report kernel Landlock support
landbound help
```

`demo` prints a JSON report and is the deterministic, scriptable path — useful for CI and for verifying enforcement without driving the TUI.

```jsonc
{
  "abi": 10,
  "baseline":   [ /* unrestricted reads: both files succeed */ ],
  "restricted": [ /* under the allowlist: public succeeds, private denied */ ],
  "private_allowed": false,
  "network_supported": true,
  "network_allowed": false,
  "network_baseline":   { /* unrestricted connect succeeds */ },
  "network_restricted": { /* under the rule: connect denied */ }
}
```

`network_supported` is `false` below ABI 4, and the `network_*` fields are then omitted because the scenario cannot be enforced.

### Keys

| Key | Action |
| --- | --- |
| `SPACE` | toggle the private-file rule |
| `N` | toggle the loopback network rule |
| `ENTER` | run the fixture under the current rules |
| `?` | show or hide help |
| `Q` | quit |

## How it works

1. `internal/demo` creates a mode-`0700` temporary fixture with `public/message.txt` and `private/secret.txt`.
2. It first runs an **unrestricted baseline** to prove both files are readable and the loopback port is reachable at all.
3. It re-executes its own binary in a private child mode, which sets `PR_SET_NO_NEW_PRIVS`, creates a Landlock ruleset with the needed filesystem and (ABI 4+) network rights, adds `path-beneath` allowlist rules plus a `net-port` rule for the loopback port when permitted, calls `landlock_restrict_self`, and only then attempts the reads and the connect.
4. The child prints structured results; the parent parses them and renders the comparison.

**The parent never applies Landlock to itself.** Only the child is restricted, and the child only ever reads the two fixture paths and connects to the one loopback port — it accepts no arbitrary command, path, or host from the TUI.

Because the baseline is verified first, Landbound only attributes a denial to Landlock when the baseline succeeded *and* the restricted run still allows what the policy permits while denying what it omits with `EACCES`.

## What Landbound does *not* claim

These are deliberate limits, not gaps to be filed as bugs:

- **Landlock restricts a process and its descendants — it does not monitor other processes.** Landbound shows observed outcomes from controlled runs, nothing more.
- **There is no per-denial event stream.** Landlock returns access-denied to the *operation*; it does not report every path or port a process tried to touch. Landbound never implies otherwise.
- **A failed read or connect does not by itself prove Landlock was the cause.** That is why the unrestricted baseline runs first.
- **Landlock is best-effort LSM hardening, and it stacks with other restrictions.** A result here proves specific read and connect attempts on *this kernel* — not whole-process confidentiality, not whole-network egress control, and not the absence of bypasses.
- **The network scenario is a single loopback TCP connect.** It demonstrates Landlock's `CONNECT_TCP` right, not general egress filtering: it does not cover UDP, DNS, bind/listen, or raw sockets.
- **Landbound stays fixture-only.** Arbitrary-command execution, persistent policies, write operations, and a bind/listen (`BIND_TCP`) scenario are out of scope.

## Development

```sh
gofmt -l .        # should print nothing
go vet ./...
go test ./...
go build ./...
```

The demo and its tests execute a **real restricted child process**, so on a Landlock-capable host the integration tests exercise actual kernel enforcement. Network tests additionally skip with an explicit reason on kernels below ABI 4, and all tests skip explicitly on a host without Landlock support instead of passing vacuously.

## Layout

```
cmd/landbound/           CLI dispatch, private child modes
internal/landlock/      ABI probe, filesystem and network syscall wrappers
internal/demo/          fixture setup, child execution, result parsing
internal/tui/           Bubble Tea model and view
docs/superpowers/       design spec and implementation plan
```

## Troubleshooting

| Symptom | Meaning |
| --- | --- |
| `landlock is not supported by kernel or LSM disabled` | Kernel lacks Landlock, or the LSM is not enabled. Check `landbound doctor`. |
| `Landlock ABI 1 lacks filesystem read rules` | Kernel is too old for the read rights Landbound uses. |
| `Landlock ABI N lacks network rules` | Kernel is below ABI 4, so the TCP-connect scenario cannot be enforced. |
| `baseline did not read both fixture files` | The fixture itself could not be read, so restricted results cannot be attributed to Landlock. |
| `baseline loopback connect failed` | The parent's listener was unreachable, so a network denial cannot be attributed to Landlock. |
| `child timed out` | The child exceeded its 3-second bound; the fixture was cleaned up. |

## License

MIT — see [LICENSE](LICENSE).
