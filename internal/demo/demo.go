package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OnlyOneArthur/landbound/internal/landlock"
)

const (
	publicContent  = "This message is public.\n"
	privateContent = "This is a private fixture.\n"
	maxOutput      = 16 * 1024
)

type ReadResult struct {
	Path  string `json:"path"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Text  string `json:"text,omitempty"`
}

type Report struct {
	ABI        int          `json:"abi"`
	Baseline   []ReadResult `json:"baseline"`
	Restricted []ReadResult `json:"restricted"`
	PrivateOK  bool         `json:"private_allowed"`
}

type childReport struct {
	Public  ReadResult `json:"public"`
	Private ReadResult `json:"private"`
	Error   string     `json:"error,omitempty"`
}

// Run creates a local fixture, verifies unrestricted reads, then runs a restricted child.
func Run(ctx context.Context, privateAllowed bool) (Report, error) {
	var report Report
	abi, err := landlock.ProbeABI()
	if err != nil {
		return report, err
	}
	report.ABI, report.PrivateOK = abi, privateAllowed

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

	base, err := readFiles(ctx, publicPath, privatePath)
	if err != nil {
		return report, fmt.Errorf("baseline child: %w", err)
	}
	report.Baseline = []ReadResult{base.Public, base.Private}
	if !base.Public.OK || !base.Private.OK {
		return report, errors.New("baseline did not read both fixture files; cannot attribute restricted results to Landlock")
	}

	var childArgs []string
	if privateAllowed {
		childArgs = []string{"--child", publicPath, privatePath, "allow-private"}
	} else {
		childArgs = []string{"--child", publicPath, privatePath, "deny-private"}
	}

	restricted, err := runChild(ctx, childArgs...)
	if err != nil {
		return report, fmt.Errorf("restricted child: %w", err)
	}
	report.Restricted = []ReadResult{restricted.Public, restricted.Private}
	return report, nil
}

func readFiles(ctx context.Context, publicPath, privatePath string) (childReport, error) {
	return runChild(ctx, "--baseline-child", publicPath, privatePath)
}

func runChild(parent context.Context, args ...string) (childReport, error) {
	var out childReport
	exe, err := os.Executable()
	if err != nil {
		return out, fmt.Errorf("locate executable: %w", err)
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
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

// ChildMain executes only fixed fixture reads after applying the requested rules.
func ChildMain(args []string, restrict bool) int {
	if len(args) < 2 {
		return writeChildError("invalid child arguments")
	}
	publicPath, privatePath := filepath.Clean(args[0]), filepath.Clean(args[1])
	if !filepath.IsAbs(publicPath) || !filepath.IsAbs(privatePath) || filepath.Dir(publicPath) == filepath.Dir(privatePath) {
		return writeChildError("invalid fixture paths")
	}
	if restrict {
		if len(args) != 3 {
			return writeChildError("invalid policy flag")
		}
		policy := args[2]
		if policy != "allow-private" && policy != "deny-private" {
			return writeChildError("unknown policy flag")
		}

		abi, err := landlock.ProbeABI()
		if err != nil {
			return writeChildError(err.Error())
		}
		if abi < 1 {
			return writeChildError(fmt.Sprintf("Landlock ABI %d lacks filesystem read rules", abi))
		}
		ruleset, err := landlock.CreateRuleset(landlock.ABI1ReadRights)
		if err != nil {
			return writeChildError(err.Error())
		}
		defer unixClose(ruleset)
		if err := landlock.AddPathRule(ruleset, filepath.Dir(publicPath), landlock.ABI1ReadRights); err != nil {
			return writeChildError(err.Error())
		}
		if policy == "allow-private" {
			if err := landlock.AddPathRule(ruleset, filepath.Dir(privatePath), landlock.ABI1ReadRights); err != nil {
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

func writeChildError(message string) int {
	_ = json.NewEncoder(os.Stdout).Encode(childReport{Error: message})
	return 0
}
