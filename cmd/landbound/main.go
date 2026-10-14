package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/OnlyOneArthur/landbound/internal/demo"
	"github.com/OnlyOneArthur/landbound/internal/landlock"
	"github.com/OnlyOneArthur/landbound/internal/tui"
)

func main() {
	args := os.Args[1:]

	// Private child modes: never user-facing, invoked only by this binary.
	if len(args) > 0 {
		switch args[0] {
		case "--baseline-child":
			os.Exit(demo.ChildMain(args[1:], false))
		case "--child":
			os.Exit(demo.ChildMain(args[1:], true))
		}
	}

	if len(args) == 0 {
		if err := tui.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "landbound:", err)
			os.Exit(1)
		}
		return
	}

	switch args[0] {
	case "demo":
		runDemo(args[1:])
	case "doctor":
		runDoctor()
	case "help", "-h", "--help":
		usage(os.Stdout)
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", args[0])
		usage(os.Stderr)
		os.Exit(2)
	}
}

func runDemo(args []string) {
	policy := demo.Policy{}
	for _, a := range args {
		switch a {
		case "--allow-private":
			policy.PrivateFileAllowed = true
		case "--deny-private":
			policy.PrivateFileAllowed = false
		case "--allow-net":
			policy.NetworkAllowed = true
		case "--deny-net":
			policy.NetworkAllowed = false
		default:
			fmt.Fprintln(os.Stderr, "unknown demo flag:", a)
			os.Exit(2)
		}
	}
	report, err := demo.Run(context.Background(), policy)
	if err != nil {
		fmt.Fprintln(os.Stderr, "demo failed:", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, "encode report:", err)
		os.Exit(1)
	}
}

func runDoctor() {
	abi, err := landlock.ProbeABI()
	if err != nil {
		fmt.Println("Landlock: unsupported")
		fmt.Println("detail:", err)
		os.Exit(1)
	}
	fmt.Printf("Landlock ABI: %d\n", abi)
	fmt.Println("filesystem read rules: available")
	if landlock.SupportsNetwork(abi) {
		fmt.Println("network connect rules: available (TCP connect)")
	} else {
		fmt.Printf("network connect rules: unavailable (needs ABI %d+, have %d)\n", landlock.MinNetworkABI, abi)
	}
	fmt.Println("note: Landlock reports access-denied to the operation, not a stream of every denied path.")
}

func usage(w *os.File) {
	fmt.Fprint(w, `landbound - Landlock policy workbench

Usage:
  landbound            launch the interactive TUI
  landbound demo [--allow-private] [--allow-net]
  landbound doctor     report kernel Landlock support
  landbound help

The demo builds a private temporary fixture, verifies an unrestricted baseline,
then runs the bundled fixture under a Landlock allowlist and reports results.
Only the bundled fixture runs; no arbitrary commands are accepted.

The private file is denied unless --allow-private is given. On kernels with
Landlock ABI 4+ the demo also attempts one loopback TCP connect, denied unless
--allow-net is given; on older kernels that scenario is reported as
unsupported rather than guessed.
`)
}
