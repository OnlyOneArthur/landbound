package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/OnlyOneArthur/landbound/internal/landlock"
)

const (
	publicContent  = "This message is public.\n"
	privateContent = "This is a private fixture.\n"
	maxOutput      = 16 * 1024
	childTimeout   = 3 * time.Second
	connectTimeout = 2 * time.Second
)

// MinNetworkABI is the first Landlock ABI that can enforce network rules. It is
// re-exported so callers can explain an unsupported network scenario without
// importing the landlock package directly.
const MinNetworkABI = landlock.MinNetworkABI

// Policy selects what the restricted child is allowed to do.
type Policy struct {
	// PrivateFileAllowed permits reading private/secret.txt when true.
	PrivateFileAllowed bool
	// NetworkAllowed permits the loopback TCP connect when true.
	NetworkAllowed bool
}

// ReadResult records the outcome of one fixture file read.
type ReadResult struct {
	Path  string `json:"path"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Text  string `json:"text,omitempty"`
}

// NetResult records the outcome of one loopback TCP connect attempt.
type NetResult struct {
	Target string `json:"target"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

// Report is the full evidence from one demo run.
type Report struct {
	ABI        int          `json:"abi"`
	Baseline   []ReadResult `json:"baseline"`
	Restricted []ReadResult `json:"restricted"`
	PrivateOK  bool         `json:"private_allowed"`

	// NetworkSupported is false on kernels below ABI 4, where the network
	// scenario cannot be enforced and is therefore never claimed.
	NetworkSupported  bool       `json:"network_supported"`
	NetworkAllowed    bool       `json:"network_allowed"`
	NetworkBaseline   *NetResult `json:"network_baseline,omitempty"`
	NetworkRestricted *NetResult `json:"network_restricted,omitempty"`
}

type childReport struct {
	Public  ReadResult `json:"public"`
	Private ReadResult `json:"private"`
	Network *NetResult `json:"network,omitempty"`
	Error   string     `json:"error,omitempty"`
}

// Run creates a local fixture, verifies unrestricted baseline behaviour, then
// runs a restricted child under the requested policy and returns the evidence.
//
// The network scenario only runs when the kernel can enforce Landlock network
// rules (ABI 4+); on older kernels the report marks it unsupported instead of
// producing a result it could not attribute.
func Run(ctx context.Context, policy Policy) (Report, error) {
	var report Report
	abi, err := landlock.ProbeABI()
	if err != nil {
		return report, err
	}
	report.ABI = abi
	report.PrivateOK = policy.PrivateFileAllowed
	report.NetworkSupported = landlock.SupportsNetwork(abi)
	report.NetworkAllowed = policy.NetworkAllowed

	root, err := os.MkdirTemp("", "landbound-")
	if err != nil {
		return report, fmt.Errorf("create fixture: %w", err)
	}
	defer os.RemoveAll(root)
	if err := os.Chmod(root, 0700); err != nil {
		return report, fmt.Errorf("secure fixture: %w", err)
	}
	publicDir, privateDir := filepath.Join(root, "public"), filepath.Join(root, "private")
	if err := os.Mkdir(publicDir, 0700); err != nil {
		return report, err
	}
	if err := os.Mkdir(privateDir, 0700); err != nil {
		return report, err
	}
	publicPath, privatePath := filepath.Join(publicDir, "message.txt"), filepath.Join(privateDir, "secret.txt")
	if err := os.WriteFile(publicPath, []byte(publicContent), 0600); err != nil {
		return report, err
	}
	if err := os.WriteFile(privatePath, []byte(privateContent), 0600); err != nil {
		return report, err
	}

	// The network scenario needs a reachable loopback endpoint owned by the
	// parent. Both children connect back to it; the baseline proves the port is
	// reachable so a restricted denial can be attributed to Landlock.
	var port int
	if report.NetworkSupported {
		ln, err := listenLoopback()
		if err != nil {
			return report, fmt.Errorf("network scenario: %w", err)
		}
		defer ln.Close()
		port = ln.Addr().(*net.TCPAddr).Port
	}

	baselineArgs := []string{"--baseline-child", publicPath, privatePath}
	if port > 0 {
		baselineArgs = append(baselineArgs, strconv.Itoa(port))
	}
	base, err := runChild(ctx, baselineArgs...)
	if err != nil {
		return report, fmt.Errorf("baseline child: %w", err)
	}
	report.Baseline = []ReadResult{base.Public, base.Private}
	if !base.Public.OK || !base.Private.OK {
		return report, errors.New("baseline did not read both fixture files; cannot attribute restricted results to Landlock")
	}
	if port > 0 {
		report.NetworkBaseline = base.Network
		if base.Network == nil || !base.Network.OK {
			return report, errors.New("baseline loopback connect failed; cannot attribute network results to Landlock")
		}
	}

	filePolicy := "deny-private"
	if policy.PrivateFileAllowed {
		filePolicy = "allow-private"
	}
	childArgs := []string{"--child", publicPath, privatePath, filePolicy}
	if port > 0 {
		netPolicy := "net-deny"
		if policy.NetworkAllowed {
			netPolicy = "net-allow"
		}
		childArgs = append(childArgs, strconv.Itoa(port), netPolicy)
	}

	restricted, err := runChild(ctx, childArgs...)
	if err != nil {
		return report, fmt.Errorf("restricted child: %w", err)
	}
	report.Restricted = []ReadResult{restricted.Public, restricted.Private}
	report.NetworkRestricted = restricted.Network
	return report, nil
}

// listenLoopback starts a parent-owned listener on an ephemeral loopback port
// and drains connections so a permitted child connect completes promptly.
func listenLoopback() (net.Listener, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	return ln, nil
}

func runChild(parent context.Context, args ...string) (childReport, error) {
	var out childReport
	exe, err := os.Executable()
	if err != nil {
		return out, fmt.Errorf("locate executable: %w", err)
	}
	ctx, cancel := context.WithTimeout(parent, childTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	var stdout limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return out, fmt.Errorf("child timed out: %w", ctx.Err())
		}
		return out, fmt.Errorf("child failed: %w", err)
	}
	if stdout.overflow {
		return out, errors.New("child output exceeded limit")
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return out, fmt.Errorf("parse child result: %w", err)
	}
	if out.Error != "" {
		return out, errors.New(out.Error)
	}
	return out, nil
}

type limitedBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > maxOutput {
		b.overflow = true
		return 0, errors.New("output limit exceeded")
	}
	return b.Buffer.Write(p)
}

// ChildMain executes only fixed fixture operations after applying the requested
// policy. It is reached solely through private flags set by Run; it accepts no
// arbitrary command, and the only resources it touches are the two fixture
// files plus one loopback port.
func ChildMain(args []string, restrict bool) int {
	if len(args) < 2 {
		return writeChildError("invalid child arguments")
	}
	publicPath, privatePath := filepath.Clean(args[0]), filepath.Clean(args[1])
	if !filepath.IsAbs(publicPath) || !filepath.IsAbs(privatePath) || filepath.Dir(publicPath) == filepath.Dir(privatePath) {
		return writeChildError("invalid fixture paths")
	}

	rest := args[2:]
	var (
		filePolicy string
		port       int
		netPolicy  string
	)
	if restrict {
		if len(rest) == 0 {
			return writeChildError("missing policy flag")
		}
		filePolicy = rest[0]
		if filePolicy != "allow-private" && filePolicy != "deny-private" {
			return writeChildError("unknown policy flag")
		}
		switch len(rest) {
		case 1:
		case 3:
			p, ok := parsePort(rest[1])
			if !ok {
				return writeChildError("invalid port")
			}
			port = p
			netPolicy = rest[2]
			if netPolicy != "net-allow" && netPolicy != "net-deny" {
				return writeChildError("unknown network policy flag")
			}
		default:
			return writeChildError("invalid policy arguments")
		}
	} else {
		switch len(rest) {
		case 0:
		case 1:
			p, ok := parsePort(rest[0])
			if !ok {
				return writeChildError("invalid port")
			}
			port = p
		default:
			return writeChildError("invalid baseline arguments")
		}
	}

	if restrict {
		abi, err := landlock.ProbeABI()
		if err != nil {
			return writeChildError(err.Error())
		}
		if abi < 1 {
			return writeChildError(fmt.Sprintf("Landlock ABI %d lacks filesystem read rules", abi))
		}
		var handledNet uint64
		if port > 0 {
			if !landlock.SupportsNetwork(abi) {
				return writeChildError(fmt.Sprintf("Landlock ABI %d lacks network rules", abi))
			}
			handledNet = landlock.ABI4NetConnectRights
		}
		ruleset, err := landlock.CreateRulesetWithNet(landlock.ABI1ReadRights, handledNet)
		if err != nil {
			return writeChildError(err.Error())
		}
		defer unixClose(ruleset)
		if err := landlock.AddPathRule(ruleset, filepath.Dir(publicPath), landlock.ABI1ReadRights); err != nil {
			return writeChildError(err.Error())
		}
		if filePolicy == "allow-private" {
			if err := landlock.AddPathRule(ruleset, filepath.Dir(privatePath), landlock.ABI1ReadRights); err != nil {
				return writeChildError(err.Error())
			}
		}
		// Omitting a net-port rule is what denies the connection: a ruleset that
		// handles CONNECT_TCP permits only explicitly listed ports.
		if port > 0 && netPolicy == "net-allow" {
			if err := landlock.AddNetPortRule(ruleset, uint64(port), landlock.ABI4NetConnectRights); err != nil {
				return writeChildError(err.Error())
			}
		}
		if err := landlock.RestrictSelf(ruleset); err != nil {
			return writeChildError(err.Error())
		}
	}

	r := childReport{
		Public:  attemptRead(publicPath, "public/message.txt"),
		Private: attemptRead(privatePath, "private/secret.txt"),
	}
	if port > 0 {
		net := attemptConnect(port)
		r.Network = &net
	}
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(r); err != nil {
		return 1
	}
	return 0
}

func attemptRead(path, label string) ReadResult {
	b, err := os.ReadFile(path)
	if err != nil {
		return ReadResult{Path: label, Error: err.Error()}
	}
	return ReadResult{Path: label, OK: true, Text: strings.TrimSpace(string(b))}
}

func attemptConnect(port int) NetResult {
	target := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", target, connectTimeout)
	if err != nil {
		return NetResult{Target: target, Error: err.Error()}
	}
	_ = conn.Close()
	return NetResult{Target: target, OK: true}
}

func parsePort(s string) (int, bool) {
	p, err := strconv.Atoi(s)
	if err != nil || p < 1 || p > 65535 {
		return 0, false
	}
	return p, true
}

func writeChildError(message string) int {
	_ = json.NewEncoder(os.Stdout).Encode(childReport{Error: message})
	return 0
}
