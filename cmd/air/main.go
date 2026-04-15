package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
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
		provider, err := parseProviderFlag(args[2:])
		if err != nil {
			exitErr(err)
		}
		s, err := manager.CreateWithProvider(provider)
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
		fmt.Fprintf(os.Stderr, "request_id=%s duration_ms=%d\n", result.RequestID, result.Duration.Milliseconds())
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
		tailLines := 0
		for _, arg := range args[3:] {
			if arg == "--follow" || arg == "-f" {
				follow = true
				continue
			}
			if strings.HasPrefix(arg, "--tail=") {
				value := strings.TrimPrefix(arg, "--tail=")
				parsed, err := strconv.Atoi(value)
				if err != nil || parsed < 0 {
					exitErr(errors.New("tail must be a non-negative integer"))
				}
				tailLines = parsed
			}
		}
		path, err := manager.ConsolePath(args[2])
		if err != nil {
			exitErr(err)
		}
		if err := streamFile(path, follow, tailLines); err != nil {
			exitErr(err)
		}
	case "events":
		if len(args) < 3 {
			usage()
			os.Exit(1)
		}
		follow := false
		tailLines := 50
		for _, arg := range args[3:] {
			if arg == "--follow" || arg == "-f" {
				follow = true
				continue
			}
			if strings.HasPrefix(arg, "--tail=") {
				value := strings.TrimPrefix(arg, "--tail=")
				parsed, err := strconv.Atoi(value)
				if err != nil || parsed < 0 {
					exitErr(errors.New("tail must be a non-negative integer"))
				}
				tailLines = parsed
			}
		}
		path, err := manager.EventsPath(args[2])
		if err != nil {
			exitErr(err)
		}
		if err := streamFile(path, follow, tailLines); err != nil {
			exitErr(err)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  air session create [--provider local|firecracker]")
	fmt.Fprintln(os.Stderr, "  air session list")
	fmt.Fprintln(os.Stderr, "  air session inspect <id>")
	fmt.Fprintln(os.Stderr, "  air session console <id> [--follow] [--tail=N]")
	fmt.Fprintln(os.Stderr, "  air session events <id> [--follow] [--tail=N]")
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

func streamFile(path string, follow bool, tailLines int) error {
	if !follow {
		if tailLines > 0 {
			_, err := copyTailLines(path, tailLines, os.Stdout)
			return err
		}
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
	if tailLines > 0 {
		nextOffset, err := copyTailLines(path, tailLines, os.Stdout)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		offset = nextOffset
	}
	for {
		nextOffset, err := copyFileDelta(path, offset, os.Stdout)
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

func copyFileDelta(path string, offset int64, dst io.Writer) (int64, error) {
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

func copyTailLines(path string, lines int, dst io.Writer) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	body, err := io.ReadAll(file)
	if err != nil {
		return 0, err
	}
	if lines <= 0 {
		return int64(len(body)), nil
	}
	all := strings.Split(string(body), "\n")
	if len(all) > 0 && all[len(all)-1] == "" {
		all = all[:len(all)-1]
	}
	start := 0
	if len(all) > lines {
		start = len(all) - lines
	}
	content := strings.Join(all[start:], "\n")
	if content != "" {
		content += "\n"
	}
	if _, err := io.WriteString(dst, content); err != nil {
		return 0, err
	}
	return int64(len(body)), nil
}

func parseProviderFlag(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if len(args) != 2 || args[0] != "--provider" {
		return "", errors.New("usage: air session create [--provider local|firecracker]")
	}
	if args[1] == "" {
		return "", errors.New("provider must not be empty")
	}
	return args[1], nil
}
