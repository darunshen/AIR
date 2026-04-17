package main

import (
	"bufio"
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

	"github.com/darunshen/AIR/internal/buildinfo"
	"github.com/darunshen/AIR/internal/install"
	"github.com/darunshen/AIR/internal/model"
	"github.com/darunshen/AIR/internal/session"
	"github.com/darunshen/AIR/internal/vm"
)

func main() {
	manager, err := session.NewManager()
	if err != nil {
		exitErr(err)
	}

	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	switch args[0] {
	case "version":
		printJSON(buildinfo.Current())
	case "init":
		if len(args) < 2 {
			usage()
			os.Exit(1)
		}
		switch args[1] {
		case "firecracker":
			opts, err := parseInitFirecrackerFlags(args[2:])
			if err != nil {
				exitErr(err)
			}
			if err := runInitFirecracker(opts); err != nil {
				exitErr(err)
			}
		default:
			usage()
			os.Exit(1)
		}
	case "doctor":
		opts, err := parseDoctorFlags(args[1:])
		if err != nil {
			exitErr(err)
		}
		cfg := vm.ResolveConfig("runtime/sessions")
		if opts.Provider != "" {
			cfg.Provider = opts.Provider
		}
		report := vm.Diagnose(cfg)
		if opts.Human {
			printDoctorHuman(report)
		} else {
			printJSON(report)
		}
		if !report.Ready {
			os.Exit(1)
		}
	case "run":
		opts, command, err := parseRunFlags(args[1:])
		if err != nil {
			exitErr(err)
		}
		result, runErr := manager.Run(command, session.RunOptions{
			Provider:  opts.Provider,
			Timeout:   opts.Timeout,
			MemoryMiB: opts.MemoryMiB,
			VCPUCount: opts.VCPUCount,
		})
		if opts.Human {
			printRunHuman(result)
		} else {
			printJSON(result)
		}
		exitCode := exitCodeForRunResult(result, runErr)
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	case "session":
		if len(args) < 2 {
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
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  air version")
	fmt.Fprintln(os.Stderr, "  air init firecracker [--source official|custom] [--dir PATH] [--yes]")
	fmt.Fprintln(os.Stderr, "  air doctor [--provider local|firecracker] [--human]")
	fmt.Fprintln(os.Stderr, "  air run [--provider local|firecracker] [--timeout 30s] [--memory-mib 256] [--vcpu-count 1] [--human] -- <command>")
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
	if hint := firecrackerDoctorHint(err); hint != "" {
		fmt.Fprintln(os.Stderr, "hint:", hint)
	}
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

type doctorCLIOptions struct {
	Provider string
	Human    bool
}

type initFirecrackerCLIOptions struct {
	Source string
	Dir    string
	Yes    bool
}

func parseDoctorFlags(args []string) (doctorCLIOptions, error) {
	var opts doctorCLIOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--human":
			opts.Human = true
		case arg == "--provider":
			if i+1 >= len(args) || args[i+1] == "" {
				return doctorCLIOptions{}, errors.New("provider must not be empty")
			}
			opts.Provider = args[i+1]
			i++
		case strings.HasPrefix(arg, "--provider="):
			opts.Provider = strings.TrimPrefix(arg, "--provider=")
			if opts.Provider == "" {
				return doctorCLIOptions{}, errors.New("provider must not be empty")
			}
		default:
			return doctorCLIOptions{}, fmt.Errorf("unknown doctor flag: %s", arg)
		}
	}
	return opts, nil
}

func parseInitFirecrackerFlags(args []string) (initFirecrackerCLIOptions, error) {
	var opts initFirecrackerCLIOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--yes":
			opts.Yes = true
		case arg == "--source":
			if i+1 >= len(args) || args[i+1] == "" {
				return initFirecrackerCLIOptions{}, errors.New("source must not be empty")
			}
			opts.Source = args[i+1]
			i++
		case strings.HasPrefix(arg, "--source="):
			opts.Source = strings.TrimPrefix(arg, "--source=")
		case arg == "--dir":
			if i+1 >= len(args) || args[i+1] == "" {
				return initFirecrackerCLIOptions{}, errors.New("dir must not be empty")
			}
			opts.Dir = args[i+1]
			i++
		case strings.HasPrefix(arg, "--dir="):
			opts.Dir = strings.TrimPrefix(arg, "--dir=")
		default:
			return initFirecrackerCLIOptions{}, fmt.Errorf("unknown init flag: %s", arg)
		}
	}
	switch opts.Source {
	case "", "official", "custom":
	default:
		return initFirecrackerCLIOptions{}, fmt.Errorf("unsupported source: %s", opts.Source)
	}
	return opts, nil
}

func runInitFirecracker(opts initFirecrackerCLIOptions) error {
	source := opts.Source
	if source == "" {
		if !isInteractiveTerminal(os.Stdin) {
			return errors.New("source is required in non-interactive mode; use --source official or --source custom")
		}
		selected, err := promptInitFirecrackerSource()
		if err != nil {
			return err
		}
		source = selected
	}

	switch source {
	case "official":
		if !opts.Yes && isInteractiveTerminal(os.Stdin) {
			ok, err := promptConfirm(fmt.Sprintf("download AIR official Firecracker image bundle to %s", resolvedInitDir(opts.Dir)))
			if err != nil {
				return err
			}
			if !ok {
				return errors.New("init cancelled")
			}
		}
		return installOfficialFirecrackerBundle(resolvedInitDir(opts.Dir))
	case "custom":
		fmt.Fprintln(os.Stdout, install.BuildCustomInstallGuide(resolvedInitDir(opts.Dir)))
		fmt.Fprintln(os.Stdout, "参考仓库文档：docs/firecracker-deployment-guide.md")
		return nil
	default:
		return fmt.Errorf("unsupported source: %s", source)
	}
}

func resolvedInitDir(dir string) string {
	if dir != "" {
		return dir
	}
	return install.DefaultFirecrackerInstallDir()
}

func installOfficialFirecrackerBundle(outputDir string) error {
	version := install.CurrentVersion()
	fmt.Fprintf(os.Stdout, "downloading AIR official Firecracker bundle for %s to %s\n", version, outputDir)
	if version == "" {
		return errors.New("unable to determine AIR version for official bundle download")
	}
	ctx := context.Background()
	installedDir, err := install.DownloadOfficialFirecrackerBundle(ctx, version, outputDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "installed Firecracker assets to %s\n", installedDir)

	cfg := vm.ResolveConfig("runtime/sessions")
	cfg.Provider = "firecracker"
	report := vm.Diagnose(cfg)
	printDoctorHuman(report)
	if !report.Ready {
		return errors.New("firecracker assets downloaded, but runtime doctor still reports missing dependencies")
	}
	return nil
}

func promptInitFirecrackerSource() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintln(os.Stdout, "Firecracker 运行环境初始化方式：")
	fmt.Fprintln(os.Stdout, "  1) 下载 AIR 官方镜像包（推荐）")
	fmt.Fprintln(os.Stdout, "  2) 我自己部署 Firecracker / kernel / rootfs")
	fmt.Fprint(os.Stdout, "请选择 [1/2]: ")

	choice, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(choice) {
	case "1":
		return "official", nil
	case "2":
		return "custom", nil
	default:
		return "", fmt.Errorf("unsupported selection: %s", strings.TrimSpace(choice))
	}
}

func promptConfirm(message string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stdout, "%s? [y/N]: ", message)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}

func isInteractiveTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func printDoctorHuman(report *vm.DoctorReport) {
	if report == nil {
		return
	}

	fmt.Fprintf(os.Stdout, "provider=%s ready=%t\n", report.Provider, report.Ready)
	if report.Provider == "firecracker" {
		fmt.Fprintf(os.Stdout, "firecracker_binary=%s\n", report.ResolvedConfig.FirecrackerBinary)
		fmt.Fprintf(os.Stdout, "kernel_image=%s\n", report.ResolvedConfig.KernelImage)
		fmt.Fprintf(os.Stdout, "rootfs_image=%s\n", report.ResolvedConfig.RootfsImage)
		fmt.Fprintf(os.Stdout, "kvm_device=%s\n", report.ResolvedConfig.KVMDevice)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "CHECK\tSTATUS\tVALUE\tMESSAGE")
	for _, check := range report.Checks {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			check.Name,
			strings.ToUpper(check.Status),
			check.Value,
			check.Message,
		)
	}
	_ = w.Flush()

	for _, check := range report.Checks {
		if check.Hint == "" {
			continue
		}
		fmt.Fprintf(os.Stdout, "hint[%s]=%s\n", check.Name, check.Hint)
	}
}

func firecrackerDoctorHint(err error) string {
	switch {
	case errors.Is(err, vm.ErrFirecrackerBinaryNotFound),
		errors.Is(err, vm.ErrFirecrackerKernelRequired),
		errors.Is(err, vm.ErrFirecrackerKernelNotFound),
		errors.Is(err, vm.ErrFirecrackerRootfsRequired),
		errors.Is(err, vm.ErrFirecrackerRootfsNotFound),
		errors.Is(err, vm.ErrKVMDeviceNotAvailable):
		return "run `air init firecracker` or `air doctor --provider firecracker --human` to inspect runtime dependencies"
	default:
		return ""
	}
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

type runCLIOptions struct {
	Provider  string
	Timeout   time.Duration
	MemoryMiB int
	VCPUCount int
	Human     bool
}

func parseRunFlags(args []string) (runCLIOptions, string, error) {
	opts := runCLIOptions{
		Timeout: 30 * time.Second,
	}

	var commandArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			commandArgs = append(commandArgs, args[i+1:]...)
			i = len(args)
		case arg == "--human":
			opts.Human = true
		case arg == "--provider":
			if i+1 >= len(args) || args[i+1] == "" {
				return runCLIOptions{}, "", errors.New("provider must not be empty")
			}
			opts.Provider = args[i+1]
			i++
		case strings.HasPrefix(arg, "--provider="):
			opts.Provider = strings.TrimPrefix(arg, "--provider=")
			if opts.Provider == "" {
				return runCLIOptions{}, "", errors.New("provider must not be empty")
			}
		case arg == "--timeout":
			if i+1 >= len(args) || args[i+1] == "" {
				return runCLIOptions{}, "", errors.New("timeout must not be empty")
			}
			value, err := time.ParseDuration(args[i+1])
			if err != nil {
				return runCLIOptions{}, "", fmt.Errorf("invalid timeout: %w", err)
			}
			opts.Timeout = value
			i++
		case strings.HasPrefix(arg, "--timeout="):
			value, err := time.ParseDuration(strings.TrimPrefix(arg, "--timeout="))
			if err != nil {
				return runCLIOptions{}, "", fmt.Errorf("invalid timeout: %w", err)
			}
			opts.Timeout = value
		case arg == "--memory-mib":
			if i+1 >= len(args) || args[i+1] == "" {
				return runCLIOptions{}, "", errors.New("memory-mib must not be empty")
			}
			value, err := strconv.Atoi(args[i+1])
			if err != nil || value <= 0 {
				return runCLIOptions{}, "", errors.New("memory-mib must be a positive integer")
			}
			opts.MemoryMiB = value
			i++
		case strings.HasPrefix(arg, "--memory-mib="):
			value, err := strconv.Atoi(strings.TrimPrefix(arg, "--memory-mib="))
			if err != nil || value <= 0 {
				return runCLIOptions{}, "", errors.New("memory-mib must be a positive integer")
			}
			opts.MemoryMiB = value
		case arg == "--vcpu-count":
			if i+1 >= len(args) || args[i+1] == "" {
				return runCLIOptions{}, "", errors.New("vcpu-count must not be empty")
			}
			value, err := strconv.Atoi(args[i+1])
			if err != nil || value <= 0 {
				return runCLIOptions{}, "", errors.New("vcpu-count must be a positive integer")
			}
			opts.VCPUCount = value
			i++
		case strings.HasPrefix(arg, "--vcpu-count="):
			value, err := strconv.Atoi(strings.TrimPrefix(arg, "--vcpu-count="))
			if err != nil || value <= 0 {
				return runCLIOptions{}, "", errors.New("vcpu-count must be a positive integer")
			}
			opts.VCPUCount = value
		default:
			commandArgs = append(commandArgs, args[i:]...)
			i = len(args)
		}
	}

	command := strings.TrimSpace(strings.Join(commandArgs, " "))
	if command == "" {
		return runCLIOptions{}, "", errors.New("usage: air run [--provider local|firecracker] [--timeout 30s] [--memory-mib 256] [--vcpu-count 1] [--human] -- <command>")
	}
	if opts.Timeout <= 0 {
		return runCLIOptions{}, "", errors.New("timeout must be greater than 0")
	}
	return opts, command, nil
}

func printRunHuman(result *session.RunResult) {
	if result == nil {
		return
	}
	if result.Stdout != "" {
		fmt.Print(result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, result.Stderr)
	}
	fmt.Fprintf(os.Stderr, "provider=%s request_id=%s duration_ms=%d timeout=%t\n",
		result.Provider,
		result.RequestID,
		result.DurationMS,
		result.Timeout,
	)
	if result.ErrorType != "" && result.ErrorType != session.RunErrorTypeTimeout {
		fmt.Fprintf(os.Stderr, "error_type=%s error_message=%s\n", result.ErrorType, result.ErrorMessage)
	}
}

func exitCodeForRunResult(result *session.RunResult, runErr error) int {
	if result != nil {
		if result.ExitCode >= 0 {
			return result.ExitCode
		}
		if result.ErrorType != "" {
			return 1
		}
	}
	if runErr != nil {
		return 1
	}
	return 0
}
