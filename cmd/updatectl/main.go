// Command updatectl provides a CLI for managing update transactions.
// It is used by the systemd and compose updater agents to interact with the update contract.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/matta813/velora-dns/internal/update"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: updatectl {status|begin|transition|fail|complete|rollback|history}\n")
		os.Exit(2)
	}

	config := update.DefaultConfig()
	if v := os.Getenv("VELORA_UPDATE_STATE_FILE"); v != "" {
		config.StateFile = v
	}

	m := update.NewManager(config)
	if err := m.LoadState(); err != nil {
		fmt.Fprintf(os.Stderr, "load state: %v\n", err)
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "status":
		cmdStatus(m)
	case "begin":
		cmdBegin(m, os.Args[2:])
	case "transition":
		cmdTransition(m, os.Args[2:])
	case "fail":
		cmdFail(m, os.Args[2:])
	case "complete":
		cmdComplete(m, os.Args[2:])
	case "rollback":
		cmdRollback(m, os.Args[2:])
	case "history":
		cmdHistory(m)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(2)
	}
}

func cmdStatus(m *update.Manager) {
	current := m.Current()
	if current == nil {
		fmt.Println(`{"state":"idle"}`)
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.Encode(current)
}

func cmdBegin(m *update.Manager, args []string) {
	if len(args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: updatectl begin FROM_VERSION TO_VERSION DEPLOYMENT_MODE\n")
		os.Exit(2)
	}
	entry, err := m.Begin(args[0], args[1], args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "begin: %v\n", err)
		os.Exit(1)
	}
	_ = m.SaveState()
	enc := json.NewEncoder(os.Stdout)
	enc.Encode(entry)
}

func cmdTransition(m *update.Manager, args []string) {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: updatectl transition ID STATE\n")
		os.Exit(2)
	}
	if err := m.Transition(args[0], update.State(args[1])); err != nil {
		fmt.Fprintf(os.Stderr, "transition: %v\n", err)
		os.Exit(1)
	}
	_ = m.SaveState()
	fmt.Printf("transitioned to %s\n", args[1])
}

func cmdFail(m *update.Manager, args []string) {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: updatectl fail ID ERROR_MESSAGE\n")
		os.Exit(2)
	}
	if err := m.Fail(args[0], fmt.Errorf("%s", args[1])); err != nil {
		fmt.Fprintf(os.Stderr, "fail: %v\n", err)
		os.Exit(1)
	}
	_ = m.SaveState()
	fmt.Println("marked as failed")
}

func cmdComplete(m *update.Manager, args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "usage: updatectl complete ID\n")
		os.Exit(2)
	}
	if err := m.Complete(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "complete: %v\n", err)
		os.Exit(1)
	}
	_ = m.SaveState()
	fmt.Println("completed")
}

func cmdRollback(m *update.Manager, args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "usage: updatectl rollback ID\n")
		os.Exit(2)
	}
	if err := m.Rollback(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "rollback: %v\n", err)
		os.Exit(1)
	}
	_ = m.SaveState()
	fmt.Println("rolled back")
}

func cmdHistory(m *update.Manager) {
	history := m.History()
	enc := json.NewEncoder(os.Stdout)
	enc.Encode(history)
}

func init() {
	// Ensure timestamps are serialized in a consistent format.
	_ = time.UTC
}
