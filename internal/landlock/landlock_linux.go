//go:build linux

package landlock

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

var (
	ErrNotSupported = errors.New("landlock is not supported by kernel or LSM disabled")
	ErrInvalidRule  = errors.New("invalid rule specification")
)

// Supported filesystem read rights in ABI v1.
const (
	AccessFSReadFile = unix.LANDLOCK_ACCESS_FS_READ_FILE
	AccessFSReadDir  = unix.LANDLOCK_ACCESS_FS_READ_DIR
	AccessFSExecute  = unix.LANDLOCK_ACCESS_FS_EXECUTE

	ABI1ReadRights = AccessFSReadFile | AccessFSReadDir
)

// Network access rights, available from ABI 4.
const (
	AccessNetBindTCP    = unix.LANDLOCK_ACCESS_NET_BIND_TCP
	AccessNetConnectTCP = unix.LANDLOCK_ACCESS_NET_CONNECT_TCP

	// ABI4NetConnectRights is the connect-only network policy Landbound demonstrates.
	ABI4NetConnectRights = AccessNetConnectTCP

	// MinNetworkABI is the first ABI that supports network access control.
	MinNetworkABI = 4

	// landlockRuleNetPort is LANDLOCK_RULE_NET_PORT, the net-port rule type.
	landlockRuleNetPort = 2
)

// SupportsNetwork reports whether an ABI can enforce Landlock network rules.
func SupportsNetwork(abi int) bool { return abi >= MinNetworkABI }

// ProbeABI checks supported Landlock ABI version without modifying security state.
func ProbeABI() (int, error) {
	r1, _, err := unix.Syscall(
		unix.SYS_LANDLOCK_CREATE_RULESET,
		0,
		0,
		uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION),
	)
	if err != 0 {
		if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EOPNOTSUPP) {
			return 0, fmt.Errorf("%w: %v", ErrNotSupported, err)
		}
		return 0, fmt.Errorf("probe landlock abi: %w", err)
	}
	return int(r1), nil
}

// CreateRuleset creates a Landlock ruleset file descriptor handling specified
// filesystem rights only.
func CreateRuleset(handledAccessFS uint64) (int, error) {
	return CreateRulesetWithNet(handledAccessFS, 0)
}

// CreateRulesetWithNet creates a ruleset handling both filesystem and network
// rights. handledAccessNet must be zero on kernels older than ABI 4.
func CreateRulesetWithNet(handledAccessFS, handledAccessNet uint64) (int, error) {
	// Prefix layout shared by ABI v1+: unknown trailing fields stay zero, so
	// older kernels accept the struct and reject any unsupported right.
	attr := struct {
		HandledAccessFS  uint64
		HandledAccessNet uint64
	}{handledAccessFS, handledAccessNet}
	r1, _, err := unix.Syscall(
		unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0,
	)
	if err != 0 {
		if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EOPNOTSUPP) {
			return -1, fmt.Errorf("%w: %v", ErrNotSupported, err)
		}
		return -1, fmt.Errorf("create landlock ruleset: %w", err)
	}
	return int(r1), nil
}

// AddPathRule adds an allowed path-beneath rule to an active ruleset fd.
func AddPathRule(rulesetFD int, path string, allowedAccess uint64) error {
	f, err := os.OpenFile(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open path %s: %w", path, err)
	}
	defer f.Close()

	attr := unix.LandlockPathBeneathAttr{
		Allowed_access: allowedAccess,
		Parent_fd:      int32(f.Fd()),
	}

	_, _, sysErr := unix.Syscall6(
		unix.SYS_LANDLOCK_ADD_RULE,
		uintptr(rulesetFD),
		uintptr(unix.LANDLOCK_RULE_PATH_BENEATH),
		uintptr(unsafe.Pointer(&attr)),
		0, 0, 0,
	)
	if sysErr != 0 {
		return fmt.Errorf("add landlock path rule for %s: %w", path, sysErr)
	}
	return nil
}

// AddNetPortRule allows one TCP port for the given network access rights
// (ABI 4+). Omitting a port rule denies every connection to it.
func AddNetPortRule(rulesetFD int, port uint64, allowedAccess uint64) error {
	if port > 65535 {
		return fmt.Errorf("%w: port %d out of range", ErrInvalidRule, port)
	}
	attr := struct {
		AllowedAccess uint64
		Port          uint64
	}{AllowedAccess: allowedAccess, Port: port}
	_, _, sysErr := unix.Syscall6(
		unix.SYS_LANDLOCK_ADD_RULE,
		uintptr(rulesetFD),
		uintptr(landlockRuleNetPort),
		uintptr(unsafe.Pointer(&attr)),
		0, 0, 0,
	)
	if sysErr != 0 {
		return fmt.Errorf("add landlock net rule for port %d: %w", port, sysErr)
	}
	return nil
}

// RestrictSelf enables PR_SET_NO_NEW_PRIVS and enforces ruleset on the calling thread.
// The caller must stay on this OS thread for restricted operations.
func RestrictSelf(rulesetFD int) error {
	runtime.LockOSThread()
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("prctl PR_SET_NO_NEW_PRIVS: %w", err)
	}

	_, _, sysErr := unix.Syscall(
		unix.SYS_LANDLOCK_RESTRICT_SELF,
		uintptr(rulesetFD),
		0,
		0,
	)
	if sysErr != 0 {
		return fmt.Errorf("landlock_restrict_self: %w", sysErr)
	}
	return nil
}
