//go:build linux

package landlock

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

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

func TestSupportsNetworkThreshold(t *testing.T) {
	for abi := 1; abi < MinNetworkABI; abi++ {
		if SupportsNetwork(abi) {
			t.Fatalf("ABI %d must not be reported as network-capable", abi)
		}
	}
	if !SupportsNetwork(MinNetworkABI) {
		t.Fatalf("ABI %d must be network-capable", MinNetworkABI)
	}
	if ABI4NetConnectRights != AccessNetConnectTCP {
		t.Fatal("network policy must request CONNECT_TCP only")
	}
	if ABI4NetConnectRights&AccessNetBindTCP != 0 {
		t.Fatal("network policy must not request BIND_TCP")
	}
}

func TestAddNetPortRuleRejectsOutOfRangePort(t *testing.T) {
	err := AddNetPortRule(-1, 70000, ABI4NetConnectRights)
	if err == nil {
		t.Fatal("expected an error for an out-of-range port")
	}
	if !errors.Is(err, ErrInvalidRule) {
		t.Fatalf("expected ErrInvalidRule, got %v", err)
	}
}

func TestCreateRulesetRejectsUnsupportedNetRights(t *testing.T) {
	if _, err := ProbeABI(); err != nil {
		t.Skipf("landlock unsupported on this host: %v", err)
	}
	// Bit 63 is not a defined network access right; the kernel must reject it.
	fd, err := CreateRulesetWithNet(0, 1<<63)
	if err == nil {
		_ = unix.Close(fd)
		t.Fatal("expected error for unsupported network access right")
	}
}

// TestNetRuleDeniesConnect proves the kernel enforces a CONNECT_TCP policy: a
// ruleset handling CONNECT_TCP with no port rule must deny an otherwise valid
// loopback connection. It skips explicitly on kernels below ABI 4.
func TestNetRuleDeniesConnect(t *testing.T) {
	abi, err := ProbeABI()
	if err != nil {
		t.Skipf("landlock unsupported on this host: %v", err)
	}
	if !SupportsNetwork(abi) {
		t.Skipf("kernel ABI %d lacks network rules (need %d+)", abi, MinNetworkABI)
	}

	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		fd, err := CreateRulesetWithNet(0, ABI4NetConnectRights)
		if err != nil {
			done <- err
			return
		}
		defer unix.Close(fd)
		if err := RestrictSelf(fd); err != nil {
			done <- err
			return
		}
		conn, err := net.DialTimeout("tcp", "127.0.0.1:9", time.Second)
		if err == nil {
			_ = conn.Close()
			done <- errors.New("connect unexpectedly allowed")
			return
		}
		if !errors.Is(err, syscall.EACCES) {
			done <- fmt.Errorf("expected EACCES, got %v", err)
			return
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("net rule enforcement: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the restricted thread")
	}
}
