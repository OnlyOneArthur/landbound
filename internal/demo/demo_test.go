package demo

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OnlyOneArthur/landbound/internal/landlock"
)

// TestMain lets the test binary impersonate the restricted child, mirroring the
// wiring in cmd/landbound/main.go so Run can be integration-tested end to end.
func TestMain(m *testing.M) {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "--baseline-child":
			os.Exit(ChildMain(args[1:], false))
		case "--child":
			os.Exit(ChildMain(args[1:], true))
		}
	}
	os.Exit(m.Run())
}

// requireLandlock skips the test when the host cannot enforce Landlock.
func requireLandlock(t *testing.T) {
	t.Helper()
	if _, err := landlock.ProbeABI(); err != nil {
		t.Skipf("landlock unsupported on this host: %v", err)
	}
}

func TestRunDenyPrivate(t *testing.T) {
	requireLandlock(t)
	report, err := Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run(deny): %v", err)
	}
	if report.ABI < 1 {
		t.Fatalf("ABI = %d, want >= 1", report.ABI)
	}
	if len(report.Baseline) != 2 || len(report.Restricted) != 2 {
		t.Fatalf("expected 2 baseline and 2 restricted results, got %d/%d",
			len(report.Baseline), len(report.Restricted))
	}
	// Baseline proves both files are readable without Landlock.
	for _, r := range report.Baseline {
		if !r.OK {
			t.Fatalf("baseline read of %s failed: %s", r.Path, r.Error)
		}
	}
	// Restricted: public must still work, private must be denied.
	if !report.Restricted[0].OK {
		t.Fatalf("public read denied under policy: %s", report.Restricted[0].Error)
	}
	if report.Restricted[1].OK {
		t.Fatal("private read succeeded under deny-private policy")
	}
	if !strings.Contains(strings.ToLower(report.Restricted[1].Error), "permission denied") {
		t.Fatalf("expected permission denied, got %q", report.Restricted[1].Error)
	}
}

func TestRunAllowPrivate(t *testing.T) {
	requireLandlock(t)
	report, err := Run(context.Background(), true)
	if err != nil {
		t.Fatalf("Run(allow): %v", err)
	}
	if !report.PrivateOK {
		t.Fatal("report should record private_allowed=true")
	}
	if !report.Restricted[0].OK {
		t.Fatalf("public read denied under policy: %s", report.Restricted[0].Error)
	}
	if !report.Restricted[1].OK {
		t.Fatalf("private read denied despite allow-private policy: %s",
			report.Restricted[1].Error)
	}
}

// TestParentNotRestricted is the key security property from the design: Run must
// restrict only the child process, never the process that calls it.
func TestParentNotRestricted(t *testing.T) {
	requireLandlock(t)
	outside := filepath.Join(t.TempDir(), "outside-fixture.txt")
	if err := os.WriteFile(outside, []byte("parent must still read this\n"), 0o600); err != nil {
		t.Fatalf("write probe file: %v", err)
	}
	if _, err := Run(context.Background(), false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The probe file was never in the allowlist, so a restricted parent could not
	// read it. Success here proves enforcement stayed inside the child.
	if _, err := os.ReadFile(outside); err != nil {
		t.Fatalf("parent was restricted by Run: %v", err)
	}
}

func TestRunFixtureCleanup(t *testing.T) {
	requireLandlock(t)
	before := countLandboundTempDirs(t)
	if _, err := Run(context.Background(), false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	after := countLandboundTempDirs(t)
	if after > before {
		t.Fatalf("fixture dirs leaked: before=%d after=%d", before, after)
	}
}

func countLandboundTempDirs(t *testing.T) int {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(os.TempDir(), "landbound-*"))
	if err != nil {
		t.Fatalf("glob temp dirs: %v", err)
	}
	return len(entries)
}

func TestChildMainRejectsBadArguments(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		strict bool
	}{
		{"too few args", []string{"/a"}, true},
		{"relative public path", []string{"public/message.txt", "/tmp/private/secret.txt"}, false},
		{"same parent dir", []string{"/tmp/x/a.txt", "/tmp/x/b.txt"}, false},
		{"unknown policy flag", []string{"/tmp/a/f", "/tmp/b/g", "sometimes"}, true},
		{"missing policy flag", []string{"/tmp/a/f", "/tmp/b/g"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				if code := ChildMain(tc.args, tc.strict); code != 0 {
					t.Fatalf("ChildMain returned %d, want 0 (errors are reported in JSON)", code)
				}
			})
			var r childReport
			if err := json.Unmarshal([]byte(out), &r); err != nil {
				t.Fatalf("child output is not valid JSON (%v): %q", err, out)
			}
			if r.Error == "" {
				t.Fatalf("expected an error field, got %+v", r)
			}
		})
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 0, 1024)
		tmp := make([]byte, 512)
		for {
			n, err := r.Read(tmp)
			buf = append(buf, tmp[:n]...)
			if err != nil {
				break
			}
		}
		done <- string(buf)
	}()
	fn()
	os.Stdout = saved
	if err := w.Close(); err != nil {
		t.Fatalf("close write end: %v", err)
	}
	out := <-done
	if err := r.Close(); err != nil {
		t.Fatalf("close read end: %v", err)
	}
	return out
}

func TestLimitedBufferOverflow(t *testing.T) {
	var b limitedBuffer
	chunk := make([]byte, maxOutput)
	if _, err := b.Write(chunk); err != nil {
		t.Fatalf("exact-limit write should succeed: %v", err)
	}
	if _, err := b.Write([]byte("x")); err == nil {
		t.Fatal("write past the limit should fail")
	}
	if !b.overflow {
		t.Fatal("overflow flag should be set")
	}
}
