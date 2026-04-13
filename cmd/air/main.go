package main

import (
	"fmt"
	"os"

	"github.com/darunshen/AIR/internal/session"
)

func main() {
	manager, err := session.NewManager()
	if err != nil {
		exitErr(err)
	}

	args := os.Args[1:]
	if len(args) < 2 || args[0] != "session" {
		usage()
		os.Exit(1)
	}

	switch args[1] {
	case "create":
		s, err := manager.Create()
		if err != nil {
			exitErr(err)
		}
		fmt.Println(s.ID)
	case "exec":
		if len(args) < 4 {
			usage()
			os.Exit(1)
		}
		result, err := manager.Exec(args[2], args[3])
		if err != nil {
			exitErr(err)
		}
		if result.Stdout != "" {
			fmt.Print(result.Stdout)
		}
		if result.Stderr != "" {
			fmt.Fprint(os.Stderr, result.Stderr)
		}
		if result.ExitCode != 0 {
			os.Exit(result.ExitCode)
		}
	case "delete":
		if len(args) < 3 {
			usage()
			os.Exit(1)
		}
		if err := manager.Delete(args[2]); err != nil {
			exitErr(err)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  air session create")
	fmt.Fprintln(os.Stderr, "  air session exec <id> \"<command>\"")
	fmt.Fprintln(os.Stderr, "  air session delete <id>")
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
