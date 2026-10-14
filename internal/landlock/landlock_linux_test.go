//go:build linux

package landlock

import (
	"errors"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestProbeABI(t *testing.T) {
	abi, err := ProbeABI()
	if err != nil {
		if errors.Is(err, ErrNotSupported) {
			t.Skipf("landlock unsupported on this host: %v", err)
		}
		t.Fatalf("ProbeABI: %v", err)
	}
	if abi < 1 {
		t.Fatalf("expected ABI >= 1, got %d", abi)
	}
}

func TestCreateRulesetRejectsUnsupportedRights(t *testing.T) {
	if _, err := ProbeABI(); err != nil {
		t.Skipf("landlock unsupported on this host: %v", err)
	}
	// Bit 63 is not a defined filesystem access right; the kernel must reject it
	// rather than silently accepting a rule it cannot enforce.
	fd, err := CreateRuleset(1 << 63)
	if err == nil {
		_ = unix.Close(fd)
		t.Fatal("expected error for unsupported access right")
	}
}

func TestAddPathRuleMissingPath(t *testing.T) {
	if _, err := ProbeABI(); err != nil {
		t.Skipf("landlock unsupported on this host: %v", err)
	}
	fd, err := CreateRuleset(ABI1ReadRights)
	if err != nil {
		t.Fatalf("CreateRuleset: %v", err)
	}
	defer unix.Close(fd)

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if err := AddPathRule(fd, missing, ABI1ReadRights); err == nil {
		t.Fatal("expected error adding a rule for a missing path")
	}
}

func TestABI1ReadRightsCombination(t *testing.T) {
	if ABI1ReadRights != AccessFSReadFile|AccessFSReadDir {
		t.Fatalf("ABI1ReadRights = %b, want READ_FILE|READ_DIR", ABI1ReadRights)
	}
	if ABI1ReadRights&AccessFSExecute != 0 {
		t.Fatal("ABI1ReadRights must not request EXECUTE for a read-only policy")
	}
}
