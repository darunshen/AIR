package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"text/tabwriter"
	"time"

	"github.com/darunshen/AIR/internal/model"
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
	case "list":
		sessions, err := manager.List()
		if err != nil {
			exitErr(err)
		}
		printSessions(sessions)
	case "inspect":
		if len(args) < 3 {
			usage()
			os.Exit(1)
		}
		info, err := manager.Inspect(args[2])
		if err != nil {
			exitErr(err)
		}
		printJSON(info)
	case "console":
		if len(args) < 3 {
			usage()
			os.Exit(1)
		}
		follow := false
		for _, arg := range args[3:] {
			if arg == "--follow" || arg == "-f" {
				follow = true
			}
		}
		path, err := manager.ConsolePath(args[2])
		if err != nil {
			exitErr(err)
		}
		if err := streamConsole(path, follow); err != nil {
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
	fmt.Fprintln(os.Stderr, "  air session list")
	fmt.Fprintln(os.Stderr, "  air session inspect <id>")
	fmt.Fprintln(os.Stderr, "  air session console <id> [--follow]")
	fmt.Fprintln(os.Stderr, "  air session exec <id> \"<command>\"")
	fmt.Fprintln(os.Stderr, "  air session delete <id>")
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func printSessions(items []*model.Session) {
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPROVIDER\tSTATUS\tCREATED_AT\tLAST_USED_AT")
	for _, item := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			item.ID,
			item.Provider,
			item.Status,
			item.CreatedAt.Format(time.RFC3339),
			item.LastUsedAt.Format(time.RFC3339),
		)
	}
	_ = w.Flush()
}

func printJSON(v any) {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		exitErr(err)
	}
	fmt.Println(string(body))
}

func streamConsole(path string, follow bool) error {
	if !follow {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(body)
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var offset int64
	for {
		nextOffset, err := copyConsoleDelta(path, offset, os.Stdout)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(250 * time.Millisecond):
					continue
				}
			}
			return err
		}
		offset = nextOffset

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func copyConsoleDelta(path string, offset int64, dst io.Writer) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return offset, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return offset, err
	}
	if info.Size() < offset {
		offset = 0
	}

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}

	n, err := io.Copy(dst, file)
	if err != nil {
		return offset, err
	}
	return offset + n, nil
}
