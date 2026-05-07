package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/darunshen/AIR/internal/buildinfo"
	"github.com/darunshen/AIR/internal/egressproxy"
	"github.com/darunshen/AIR/internal/install"
	"github.com/darunshen/AIR/internal/model"
	"github.com/darunshen/AIR/internal/session"
	"github.com/darunshen/AIR/internal/vm"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	switch args[0] {
	case "__firecracker-egress-proxy":
		if len(args) != 2 {
			exitErr(errors.New("usage: air __firecracker-egress-proxy <unix-socket>"))
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := egressproxy.ServeUnixHTTPProxy(ctx, args[1]); err != nil {
			exitErr(err)
		}
		return
	}

	manager, err := session.NewManager()
	if err != nil {
		exitErr(err)
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
	case "chat":
		opts, err := parseChatFlags(args[1:])
		if err != nil {
			exitErr(err)
		}
		if err := runChatWizard(context.Background(), manager, opts); err != nil {
			exitErr(err)
		}
	case "run":
		opts, command, err := parseRunFlags(args[1:])
		if err != nil {
			exitErr(err)
		}
		result, runErr := manager.Run(command, session.RunOptions{
			Provider:      opts.Provider,
			Timeout:       opts.Timeout,
			MemoryMiB:     opts.MemoryMiB,
			VCPUCount:     opts.VCPUCount,
			WorkspacePath: opts.WorkspacePath,
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
			opts, err := parseSessionCreateFlags(args[2:])
			if err != nil {
				exitErr(err)
			}
			s, err := manager.CreateWithOptions(session.CreateOptions{
				Provider:      opts.Provider,
				WorkspacePath: opts.WorkspacePath,
			})
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
		case "export-workspace":
			opts, err := parseSessionExportWorkspaceFlags(args[2:])
			if err != nil {
				exitErr(err)
			}
			result, err := manager.ExportWorkspace(opts.SessionID, opts.OutputPath, opts.Force)
			if err != nil {
				exitErr(err)
			}
			printJSON(result)
		default:
			usage()
			os.Exit(1)
		}
	case "agent":
		if len(args) < 3 {
			usage()
			os.Exit(1)
		}
		switch args[1] {
		case "openclaude":
			switch args[2] {
			case "start":
				opts, err := parseOpenClaudeStartFlags(args[3:])
				if err != nil {
					exitErr(err)
				}
				status, err := manager.StartOpenClaude(opts)
				if err != nil {
					exitErr(err)
				}
				printJSON(status)
			case "status":
				if len(args) != 4 {
					exitErr(errors.New("usage: air agent openclaude status <session-id>"))
				}
				status, err := manager.OpenClaudeStatus(args[3])
				if err != nil {
					exitErr(err)
				}
				printJSON(status)
			case "stop":
				if len(args) != 4 {
					exitErr(errors.New("usage: air agent openclaude stop <session-id>"))
				}
				status, err := manager.StopOpenClaude(args[3])
				if err != nil {
					exitErr(err)
				}
				printJSON(status)
			case "forward":
				opts, err := parseOpenClaudeForwardFlags(args[3:])
				if err != nil {
					exitErr(err)
				}
				ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
				defer stop()
				fmt.Fprintf(os.Stderr, "forwarding %s -> openclaude session %s\n", opts.ListenAddress, opts.SessionID)
				if err := manager.ForwardOpenClaude(ctx, opts.SessionID, session.OpenClaudeForwardOptions{
					ListenAddress: opts.ListenAddress,
				}); err != nil {
					exitErr(err)
				}
			case "chat":
				opts, err := parseOpenClaudeChatFlags(args[3:])
				if err != nil {
					exitErr(err)
				}
				if err := runOpenClaudeChat(context.Background(), manager, opts); err != nil {
					exitErr(err)
				}
			case "run":
				opts, err := parseOpenClaudeRunFlags(args[3:])
				if err != nil {
					exitErr(err)
				}
				if err := runOpenClaudeOneCommand(context.Background(), manager, opts); err != nil {
					exitErr(err)
				}
			case "replay":
				if len(args) != 4 {
					exitErr(errors.New("usage: air agent openclaude replay <session-id>"))
				}
				if err := replayOpenClaudeChat(manager, args[3], os.Stdout); err != nil {
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
	fmt.Fprintln(os.Stderr, "  air chat [--provider local|firecracker] [--workspace PATH] [--listen 127.0.0.1:50052] [--reconfigure]")
	fmt.Fprintln(os.Stderr, "  air run [--provider local|firecracker] [--timeout 30s] [--memory-mib 256] [--vcpu-count 1] [--workspace PATH] [--human] -- <command>")
	fmt.Fprintln(os.Stderr, "  air session create [--provider local|firecracker] [--workspace PATH]")
	fmt.Fprintln(os.Stderr, "  air session list")
	fmt.Fprintln(os.Stderr, "  air session inspect <id>")
	fmt.Fprintln(os.Stderr, "  air session console <id> [--follow] [--tail=N]")
	fmt.Fprintln(os.Stderr, "  air session events <id> [--follow] [--tail=N]")
	fmt.Fprintln(os.Stderr, "  air session exec <id> \"<command>\"")
	fmt.Fprintln(os.Stderr, "  air session export-workspace <id> <output-dir> [--force]")
	fmt.Fprintln(os.Stderr, "  air session delete <id>")
	fmt.Fprintln(os.Stderr, "  air agent openclaude start [--session ID] [--provider local|firecracker] [--repo PATH] [--guest-repo PATH] [--workspace PATH] [--host HOST] [--port 50051] [--command \"bun run scripts/start-grpc.ts\"]")
	fmt.Fprintln(os.Stderr, "  air agent openclaude status <session-id>")
	fmt.Fprintln(os.Stderr, "  air agent openclaude stop <session-id>")
	fmt.Fprintln(os.Stderr, "  air agent openclaude forward <session-id> [--listen 127.0.0.1:50052]")
	fmt.Fprintln(os.Stderr, "  air agent openclaude chat <session-id> [--listen 127.0.0.1:50052] [--cli-repo PATH]")
	fmt.Fprintln(os.Stderr, "  air agent openclaude run [--provider local|firecracker] [--workspace PATH] [--repo PATH] [--guest-repo PATH] [--listen 127.0.0.1:50052]")
	fmt.Fprintln(os.Stderr, "  air agent openclaude replay <session-id>")
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

type sessionExportWorkspaceCLIOptions struct {
	SessionID  string
	OutputPath string
	Force      bool
}

type initFirecrackerCLIOptions struct {
	Source string
	Dir    string
	Yes    bool
}

type chatCLIOptions struct {
	Provider      string
	Reconfigure   bool
	WorkspacePath string
	ListenAddress string
}

type chatProfile struct {
	OpenAIBaseURL string `json:"openai_base_url"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIAPIKey  string `json:"openai_api_key"`
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

func parseChatFlags(args []string) (chatCLIOptions, error) {
	opts := chatCLIOptions{
		Provider:      "firecracker",
		ListenAddress: "127.0.0.1:50052",
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--reconfigure":
			opts.Reconfigure = true
		case arg == "--provider":
			if i+1 >= len(args) || args[i+1] == "" {
				return chatCLIOptions{}, errors.New("provider must not be empty")
			}
			opts.Provider = args[i+1]
			i++
		case strings.HasPrefix(arg, "--provider="):
			opts.Provider = strings.TrimPrefix(arg, "--provider=")
			if opts.Provider == "" {
				return chatCLIOptions{}, errors.New("provider must not be empty")
			}
		case arg == "--workspace":
			if i+1 >= len(args) || args[i+1] == "" {
				return chatCLIOptions{}, errors.New("workspace must not be empty")
			}
			opts.WorkspacePath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--workspace="):
			opts.WorkspacePath = strings.TrimPrefix(arg, "--workspace=")
			if opts.WorkspacePath == "" {
				return chatCLIOptions{}, errors.New("workspace must not be empty")
			}
		case arg == "--listen":
			if i+1 >= len(args) || args[i+1] == "" {
				return chatCLIOptions{}, errors.New("listen must not be empty")
			}
			opts.ListenAddress = args[i+1]
			i++
		case strings.HasPrefix(arg, "--listen="):
			opts.ListenAddress = strings.TrimPrefix(arg, "--listen=")
			if opts.ListenAddress == "" {
				return chatCLIOptions{}, errors.New("listen must not be empty")
			}
		default:
			return chatCLIOptions{}, fmt.Errorf("unknown chat flag: %s", arg)
		}
	}
	switch opts.Provider {
	case "local", "firecracker":
	default:
		return chatCLIOptions{}, fmt.Errorf("unsupported provider: %s", opts.Provider)
	}
	if opts.WorkspacePath == "" {
		if cwd, err := os.Getwd(); err == nil {
			opts.WorkspacePath = cwd
		}
	}
	return opts, nil
}

func parseSessionExportWorkspaceFlags(args []string) (sessionExportWorkspaceCLIOptions, error) {
	var opts sessionExportWorkspaceCLIOptions
	positionals := make([]string, 0, 2)
	for _, arg := range args {
		switch arg {
		case "--force":
			opts.Force = true
		default:
			if strings.HasPrefix(arg, "--") {
				return sessionExportWorkspaceCLIOptions{}, fmt.Errorf("unknown export-workspace flag: %s", arg)
			}
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) != 2 {
		return sessionExportWorkspaceCLIOptions{}, errors.New("usage: air session export-workspace <id> <output-dir> [--force]")
	}
	opts.SessionID = positionals[0]
	opts.OutputPath = positionals[1]
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

func runChatWizard(ctx context.Context, manager *session.Manager, opts chatCLIOptions) error {
	if !isInteractiveTerminal(os.Stdin) {
		return errors.New("air chat requires an interactive terminal")
	}

	provider := opts.Provider
	if provider == "" {
		provider = "firecracker"
	}

	if provider == "firecracker" {
		cfg := vm.ResolveConfig("runtime/sessions")
		cfg.Provider = "firecracker"
		report := vm.Diagnose(cfg)
		if !report.Ready {
			fmt.Fprintln(os.Stdout, "Firecracker 运行时未准备完成。")
			ok, err := promptConfirm(fmt.Sprintf("下载 AIR 官方 Firecracker 运行包到 %s", resolvedInitDir("")))
			if err != nil {
				return err
			}
			if !ok {
				return errors.New("chat cancelled")
			}
			if err := installOfficialFirecrackerBundle(resolvedInitDir("")); err != nil {
				return err
			}
		}
		if err := ensureOpenClaudeGuestRootfsInteractive(); err != nil {
			return err
		}
	}

	repoPath, err := ensureOpenClaudeCLIRepo()
	if err != nil {
		return err
	}
	if err := ensureProviderEnvInteractive(opts.Reconfigure); err != nil {
		return err
	}

	runOpts := openClaudeRunCLIOptions{
		Provider:      provider,
		WorkspacePath: opts.WorkspacePath,
		RepoPath:      repoPath,
		GuestRepoPath: "/opt/openclaude",
		ListenAddress: opts.ListenAddress,
	}
	return runOpenClaudeOneCommand(ctx, manager, runOpts)
}

func ensureOpenClaudeCLIRepo() (string, error) {
	if existing := os.Getenv("AIR_OPENCLAUDE_REPO"); existing != "" {
		if resolved, err := resolveOpenClaudeRepo(existing); err == nil {
			return resolved, nil
		}
	}

	defaultRepo := defaultOpenClaudeRepoPath()
	if resolved, err := resolveOpenClaudeRepo(defaultRepo); err == nil {
		_ = os.Setenv("AIR_OPENCLAUDE_REPO", resolved)
		return resolved, nil
	}

	fmt.Fprintf(os.Stdout, "OpenClaude host 运行目录未找到，默认安装位置：%s\n", defaultRepo)
	ok, err := promptConfirm("自动准备 OpenClaude host 运行时")
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("chat cancelled")
	}

	fmt.Fprintln(os.Stdout, "优先尝试下载 AIR 官方 OpenClaude bundle...")
	if err := installOfficialOpenClaudeBundle(defaultRepo); err != nil {
		fmt.Fprintf(os.Stdout, "官方 bundle 下载失败，回退到源码安装路径：%v\n", err)
		if err := installOpenClaudeRepoFromSource(defaultRepo); err != nil {
			return "", err
		}
	}
	resolved, err := resolveOpenClaudeRepo(defaultRepo)
	if err != nil {
		return "", err
	}
	_ = os.Setenv("AIR_OPENCLAUDE_REPO", resolved)
	return resolved, nil
}

func ensureOpenClaudeGuestRootfsInteractive() error {
	if value := os.Getenv("AIR_FIRECRACKER_ROOTFS"); value != "" {
		if _, err := os.Stat(value); err == nil {
			return nil
		}
	}

	if resolved := vm.ResolveFirecrackerAsset("openclaude-alpine-rootfs.ext4"); resolved != "" {
		_ = os.Setenv("AIR_FIRECRACKER_ROOTFS", resolved)
		return nil
	}

	installDir := resolvedInitDir("")
	fmt.Fprintf(os.Stdout, "OpenClaude guest rootfs 未找到，默认安装位置：%s\n", installDir)
	ok, err := promptConfirm(fmt.Sprintf("下载 AIR 官方 OpenClaude Firecracker guest 镜像到 %s", installDir))
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("chat cancelled")
	}

	rootfsPath, err := installOfficialOpenClaudeGuestBundle(installDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "installed OpenClaude guest rootfs to %s\n", rootfsPath)
	_ = os.Setenv("AIR_FIRECRACKER_ROOTFS", rootfsPath)
	return nil
}

func defaultOpenClaudeRepoPath() string {
	return install.DefaultOpenClaudeInstallDir()
}

func validateOpenClaudeRepo(repoPath string) error {
	if repoPath == "" {
		return errors.New("openclaude repo path is empty")
	}
	if _, err := os.Stat(filepath.Join(repoPath, "scripts", "start-grpc.ts")); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(repoPath, "package.json")); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(repoPath, "node_modules")); err != nil {
		return err
	}
	if _, err := exec.LookPath("bun"); err != nil {
		return fmt.Errorf("bun not found in PATH: %w", err)
	}
	return nil
}

func resolveOpenClaudeRepo(repoPath string) (string, error) {
	candidates := []string{
		repoPath,
		filepath.Join(repoPath, "openclaude"),
	}
	var lastErr error
	for _, candidate := range candidates {
		if err := validateOpenClaudeRepo(candidate); err == nil {
			if bundleBun := filepath.Join(repoPath, "bin", "bun"); candidate != repoPath {
				if info, err := os.Stat(bundleBun); err == nil && info.Mode().IsRegular() {
					_ = os.Setenv("PATH", filepath.Join(repoPath, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
				}
			}
			return candidate, nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("openclaude runtime not found")
	}
	return "", lastErr
}

func installOfficialOpenClaudeBundle(repoPath string) error {
	version := install.CurrentVersion()
	if version == "" {
		version = "latest"
	}
	installedDir, err := install.DownloadOfficialOpenClaudeBundle(context.Background(), version, repoPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "installed official OpenClaude runtime to %s\n", installedDir)
	return validateOpenClaudeRepo(installedDir)
}

func installOfficialOpenClaudeGuestBundle(outputDir string) (string, error) {
	version := install.CurrentVersion()
	if version == "" {
		version = "latest"
	}
	return install.DownloadOfficialOpenClaudeGuestBundle(context.Background(), version, outputDir)
}

func installOpenClaudeRepoFromSource(repoPath string) error {
	parent := filepath.Dir(repoPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH: %w", err)
	}
	if _, err := exec.LookPath("curl"); err != nil {
		return fmt.Errorf("curl not found in PATH: %w", err)
	}
	if _, err := exec.LookPath("unzip"); err != nil {
		return fmt.Errorf("unzip not found in PATH: %w", err)
	}
	if _, err := exec.LookPath("bun"); err != nil {
		if err := installHostBun(); err != nil {
			return err
		}
	}
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stdout, "cloning OpenClaude into %s\n", repoPath)
		cmd := exec.Command("git", "clone", "--depth=1", "https://github.com/Gitlawb/openclaude.git", repoPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	fmt.Fprintln(os.Stdout, "installing OpenClaude dependencies with bun install")
	cmd := exec.Command("bun", "install")
	cmd.Dir = repoPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func installHostBun() error {
	fmt.Fprintln(os.Stdout, "bun 未安装，准备下载官方 bun 二进制。")
	ok, err := promptConfirm("下载并安装 bun 到 ~/.local/bin")
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("chat cancelled")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	var asset string
	switch runtime.GOARCH {
	case "amd64":
		asset = "bun-linux-x64.zip"
	case "arm64":
		asset = "bun-linux-aarch64.zip"
	default:
		return fmt.Errorf("unsupported architecture for bun: %s", runtime.GOARCH)
	}
	tmpDir, err := os.MkdirTemp("", "air-bun-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	zipPath := filepath.Join(tmpDir, asset)
	url := "https://github.com/oven-sh/bun/releases/latest/download/" + asset
	curlCmd := exec.Command("curl", "-fsSL", url, "-o", zipPath)
	curlCmd.Stdout = os.Stdout
	curlCmd.Stderr = os.Stderr
	if err := curlCmd.Run(); err != nil {
		return err
	}
	unzipCmd := exec.Command("unzip", "-q", zipPath, "-d", tmpDir)
	unzipCmd.Stdout = os.Stdout
	unzipCmd.Stderr = os.Stderr
	if err := unzipCmd.Run(); err != nil {
		return err
	}
	bunPath := ""
	if err := filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && info.Name() == "bun" {
			bunPath = path
			return io.EOF
		}
		return nil
	}); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if bunPath == "" {
		return errors.New("downloaded bun archive does not contain bun binary")
	}
	target := filepath.Join(binDir, "bun")
	input, err := os.ReadFile(bunPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, input, 0o755); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "installed bun to %s\n", target)
	if !strings.Contains(os.Getenv("PATH"), binDir) {
		_ = os.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	return nil
}

func ensureProviderEnvInteractive(reconfigure bool) error {
	if os.Getenv("OPENAI_API_KEY") != "" && os.Getenv("OPENAI_BASE_URL") != "" && os.Getenv("OPENAI_MODEL") != "" {
		return nil
	}
	if !reconfigure {
		profile, err := loadChatProfile()
		if err == nil {
			if os.Getenv("OPENAI_BASE_URL") == "" && profile.OpenAIBaseURL != "" {
				_ = os.Setenv("OPENAI_BASE_URL", profile.OpenAIBaseURL)
			}
			if os.Getenv("OPENAI_MODEL") == "" && profile.OpenAIModel != "" {
				_ = os.Setenv("OPENAI_MODEL", profile.OpenAIModel)
			}
			if os.Getenv("OPENAI_API_KEY") == "" && profile.OpenAIAPIKey != "" {
				_ = os.Setenv("OPENAI_API_KEY", profile.OpenAIAPIKey)
			}
			_ = os.Setenv("CLAUDE_CODE_USE_OPENAI", "1")
		}
	}
	if os.Getenv("OPENAI_API_KEY") != "" && os.Getenv("OPENAI_BASE_URL") != "" && os.Getenv("OPENAI_MODEL") != "" {
		return nil
	}

	fmt.Fprintln(os.Stdout, "当前未检测到完整的模型配置，将进入交互设置。")
	reader := bufio.NewReader(os.Stdin)
	defaultBaseURL := "https://api.deepseek.com/v1"
	defaultModel := "deepseek-chat"
	fmt.Fprintf(os.Stdout, "OpenAI-compatible Base URL [%s]: ", defaultBaseURL)
	baseURL, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	fmt.Fprintf(os.Stdout, "Model [%s]: ", defaultModel)
	modelName, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		modelName = defaultModel
	}
	fmt.Fprint(os.Stdout, "API Key: ")
	apiKey, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return errors.New("API key must not be empty")
	}
	_ = os.Setenv("CLAUDE_CODE_USE_OPENAI", "1")
	_ = os.Setenv("OPENAI_BASE_URL", baseURL)
	_ = os.Setenv("OPENAI_MODEL", modelName)
	_ = os.Setenv("OPENAI_API_KEY", apiKey)
	return saveChatProfile(&chatProfile{
		OpenAIBaseURL: baseURL,
		OpenAIModel:   modelName,
		OpenAIAPIKey:  apiKey,
	})
}

func loadChatProfile() (*chatProfile, error) {
	path, err := chatProfilePath()
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var profile chatProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

func saveChatProfile(profile *chatProfile) error {
	if profile == nil {
		return errors.New("chat profile is nil")
	}
	path, err := chatProfilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o600)
}

func chatProfilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "air", "chat.json"), nil
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

type sessionCreateCLIOptions struct {
	Provider      string
	WorkspacePath string
}

func parseSessionCreateFlags(args []string) (sessionCreateCLIOptions, error) {
	var opts sessionCreateCLIOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--provider":
			if i+1 >= len(args) || args[i+1] == "" {
				return sessionCreateCLIOptions{}, errors.New("provider must not be empty")
			}
			opts.Provider = args[i+1]
			i++
		case strings.HasPrefix(arg, "--provider="):
			opts.Provider = strings.TrimPrefix(arg, "--provider=")
			if opts.Provider == "" {
				return sessionCreateCLIOptions{}, errors.New("provider must not be empty")
			}
		case arg == "--workspace":
			if i+1 >= len(args) || args[i+1] == "" {
				return sessionCreateCLIOptions{}, errors.New("workspace must not be empty")
			}
			opts.WorkspacePath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--workspace="):
			opts.WorkspacePath = strings.TrimPrefix(arg, "--workspace=")
			if opts.WorkspacePath == "" {
				return sessionCreateCLIOptions{}, errors.New("workspace must not be empty")
			}
		default:
			return sessionCreateCLIOptions{}, fmt.Errorf("unknown session create flag: %s", arg)
		}
	}
	return opts, nil
}

type runCLIOptions struct {
	Provider      string
	Timeout       time.Duration
	MemoryMiB     int
	VCPUCount     int
	WorkspacePath string
	Human         bool
}

type openClaudeForwardCLIOptions struct {
	SessionID     string
	ListenAddress string
}

type openClaudeChatCLIOptions struct {
	SessionID     string
	ListenAddress string
	CLIRepoPath   string
}

type openClaudeRunCLIOptions struct {
	Provider      string
	WorkspacePath string
	RepoPath      string
	GuestRepoPath string
	ListenAddress string
}

type openClaudeTranscriptEvent struct {
	Timestamp  string `json:"ts"`
	Event      string `json:"event"`
	SessionID  string `json:"session_id"`
	Provider   string `json:"provider"`
	Workdir    string `json:"workdir"`
	Text       string `json:"text,omitempty"`
	Target     string `json:"target,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	ArgsJSON   string `json:"arguments_json,omitempty"`
	ToolUseID  string `json:"tool_use_id,omitempty"`
	Output     string `json:"output,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`
	Question   string `json:"question,omitempty"`
	PromptID   string `json:"prompt_id,omitempty"`
	Reply      string `json:"reply,omitempty"`
	FullText   string `json:"full_text,omitempty"`
	PromptTok  int    `json:"prompt_tokens,omitempty"`
	CompleteTok int   `json:"completion_tokens,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
}

func parseOpenClaudeStartFlags(args []string) (session.OpenClaudeStartOptions, error) {
	var opts session.OpenClaudeStartOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--session":
			if i+1 >= len(args) || args[i+1] == "" {
				return session.OpenClaudeStartOptions{}, errors.New("session must not be empty")
			}
			opts.SessionID = args[i+1]
			i++
		case strings.HasPrefix(arg, "--session="):
			opts.SessionID = strings.TrimPrefix(arg, "--session=")
			if opts.SessionID == "" {
				return session.OpenClaudeStartOptions{}, errors.New("session must not be empty")
			}
		case arg == "--provider":
			if i+1 >= len(args) || args[i+1] == "" {
				return session.OpenClaudeStartOptions{}, errors.New("provider must not be empty")
			}
			opts.Provider = args[i+1]
			i++
		case strings.HasPrefix(arg, "--provider="):
			opts.Provider = strings.TrimPrefix(arg, "--provider=")
			if opts.Provider == "" {
				return session.OpenClaudeStartOptions{}, errors.New("provider must not be empty")
			}
		case arg == "--repo":
			if i+1 >= len(args) || args[i+1] == "" {
				return session.OpenClaudeStartOptions{}, errors.New("repo must not be empty")
			}
			opts.RepoPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--repo="):
			opts.RepoPath = strings.TrimPrefix(arg, "--repo=")
			if opts.RepoPath == "" {
				return session.OpenClaudeStartOptions{}, errors.New("repo must not be empty")
			}
		case arg == "--guest-repo":
			if i+1 >= len(args) || args[i+1] == "" {
				return session.OpenClaudeStartOptions{}, errors.New("guest-repo must not be empty")
			}
			opts.GuestRepoPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--guest-repo="):
			opts.GuestRepoPath = strings.TrimPrefix(arg, "--guest-repo=")
			if opts.GuestRepoPath == "" {
				return session.OpenClaudeStartOptions{}, errors.New("guest-repo must not be empty")
			}
		case arg == "--workspace":
			if i+1 >= len(args) || args[i+1] == "" {
				return session.OpenClaudeStartOptions{}, errors.New("workspace must not be empty")
			}
			opts.WorkspacePath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--workspace="):
			opts.WorkspacePath = strings.TrimPrefix(arg, "--workspace=")
			if opts.WorkspacePath == "" {
				return session.OpenClaudeStartOptions{}, errors.New("workspace must not be empty")
			}
		case arg == "--command":
			if i+1 >= len(args) || args[i+1] == "" {
				return session.OpenClaudeStartOptions{}, errors.New("command must not be empty")
			}
			opts.Command = args[i+1]
			i++
		case strings.HasPrefix(arg, "--command="):
			opts.Command = strings.TrimPrefix(arg, "--command=")
			if opts.Command == "" {
				return session.OpenClaudeStartOptions{}, errors.New("command must not be empty")
			}
		case arg == "--host":
			if i+1 >= len(args) || args[i+1] == "" {
				return session.OpenClaudeStartOptions{}, errors.New("host must not be empty")
			}
			opts.Host = args[i+1]
			i++
		case strings.HasPrefix(arg, "--host="):
			opts.Host = strings.TrimPrefix(arg, "--host=")
			if opts.Host == "" {
				return session.OpenClaudeStartOptions{}, errors.New("host must not be empty")
			}
		case arg == "--port":
			if i+1 >= len(args) || args[i+1] == "" {
				return session.OpenClaudeStartOptions{}, errors.New("port must not be empty")
			}
			port, err := strconv.Atoi(args[i+1])
			if err != nil || port <= 0 {
				return session.OpenClaudeStartOptions{}, errors.New("port must be a positive integer")
			}
			opts.Port = port
			i++
		case strings.HasPrefix(arg, "--port="):
			port, err := strconv.Atoi(strings.TrimPrefix(arg, "--port="))
			if err != nil || port <= 0 {
				return session.OpenClaudeStartOptions{}, errors.New("port must be a positive integer")
			}
			opts.Port = port
		default:
			return session.OpenClaudeStartOptions{}, fmt.Errorf("unknown openclaude flag: %s", arg)
		}
	}
	return opts, nil
}

func parseOpenClaudeForwardFlags(args []string) (openClaudeForwardCLIOptions, error) {
	opts := openClaudeForwardCLIOptions{
		ListenAddress: "127.0.0.1:50052",
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--listen":
			if i+1 >= len(args) || args[i+1] == "" {
				return openClaudeForwardCLIOptions{}, errors.New("listen must not be empty")
			}
			opts.ListenAddress = args[i+1]
			i++
		case strings.HasPrefix(arg, "--listen="):
			opts.ListenAddress = strings.TrimPrefix(arg, "--listen=")
			if opts.ListenAddress == "" {
				return openClaudeForwardCLIOptions{}, errors.New("listen must not be empty")
			}
		case strings.HasPrefix(arg, "--"):
			return openClaudeForwardCLIOptions{}, fmt.Errorf("unknown openclaude forward flag: %s", arg)
		default:
			if opts.SessionID != "" {
				return openClaudeForwardCLIOptions{}, errors.New("usage: air agent openclaude forward <session-id> [--listen 127.0.0.1:50052]")
			}
			opts.SessionID = arg
		}
	}
	if opts.SessionID == "" {
		return openClaudeForwardCLIOptions{}, errors.New("usage: air agent openclaude forward <session-id> [--listen 127.0.0.1:50052]")
	}
	return opts, nil
}

func parseOpenClaudeChatFlags(args []string) (openClaudeChatCLIOptions, error) {
	opts := openClaudeChatCLIOptions{
		ListenAddress: "127.0.0.1:50052",
		CLIRepoPath:   os.Getenv("AIR_OPENCLAUDE_REPO"),
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--listen":
			if i+1 >= len(args) || args[i+1] == "" {
				return openClaudeChatCLIOptions{}, errors.New("listen must not be empty")
			}
			opts.ListenAddress = args[i+1]
			i++
		case strings.HasPrefix(arg, "--listen="):
			opts.ListenAddress = strings.TrimPrefix(arg, "--listen=")
			if opts.ListenAddress == "" {
				return openClaudeChatCLIOptions{}, errors.New("listen must not be empty")
			}
		case arg == "--cli-repo":
			if i+1 >= len(args) || args[i+1] == "" {
				return openClaudeChatCLIOptions{}, errors.New("cli-repo must not be empty")
			}
			opts.CLIRepoPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--cli-repo="):
			opts.CLIRepoPath = strings.TrimPrefix(arg, "--cli-repo=")
			if opts.CLIRepoPath == "" {
				return openClaudeChatCLIOptions{}, errors.New("cli-repo must not be empty")
			}
		case strings.HasPrefix(arg, "--"):
			return openClaudeChatCLIOptions{}, fmt.Errorf("unknown openclaude chat flag: %s", arg)
		default:
			if opts.SessionID != "" {
				return openClaudeChatCLIOptions{}, errors.New("usage: air agent openclaude chat <session-id> [--listen 127.0.0.1:50052] [--cli-repo PATH]")
			}
			opts.SessionID = arg
		}
	}
	if opts.SessionID == "" {
		return openClaudeChatCLIOptions{}, errors.New("usage: air agent openclaude chat <session-id> [--listen 127.0.0.1:50052] [--cli-repo PATH]")
	}
	if opts.CLIRepoPath == "" {
		return openClaudeChatCLIOptions{}, errors.New("openclaude cli repo path is required; use --cli-repo or AIR_OPENCLAUDE_REPO")
	}
	return opts, nil
}

func parseOpenClaudeRunFlags(args []string) (openClaudeRunCLIOptions, error) {
	opts := openClaudeRunCLIOptions{
		Provider:      "firecracker",
		ListenAddress: "127.0.0.1:50052",
		RepoPath:      os.Getenv("AIR_OPENCLAUDE_REPO"),
		GuestRepoPath: os.Getenv("AIR_OPENCLAUDE_GUEST_REPO"),
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--provider":
			if i+1 >= len(args) || args[i+1] == "" {
				return openClaudeRunCLIOptions{}, errors.New("provider must not be empty")
			}
			opts.Provider = args[i+1]
			i++
		case strings.HasPrefix(arg, "--provider="):
			opts.Provider = strings.TrimPrefix(arg, "--provider=")
			if opts.Provider == "" {
				return openClaudeRunCLIOptions{}, errors.New("provider must not be empty")
			}
		case arg == "--workspace":
			if i+1 >= len(args) || args[i+1] == "" {
				return openClaudeRunCLIOptions{}, errors.New("workspace must not be empty")
			}
			opts.WorkspacePath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--workspace="):
			opts.WorkspacePath = strings.TrimPrefix(arg, "--workspace=")
			if opts.WorkspacePath == "" {
				return openClaudeRunCLIOptions{}, errors.New("workspace must not be empty")
			}
		case arg == "--repo":
			if i+1 >= len(args) || args[i+1] == "" {
				return openClaudeRunCLIOptions{}, errors.New("repo must not be empty")
			}
			opts.RepoPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--repo="):
			opts.RepoPath = strings.TrimPrefix(arg, "--repo=")
			if opts.RepoPath == "" {
				return openClaudeRunCLIOptions{}, errors.New("repo must not be empty")
			}
		case arg == "--guest-repo":
			if i+1 >= len(args) || args[i+1] == "" {
				return openClaudeRunCLIOptions{}, errors.New("guest-repo must not be empty")
			}
			opts.GuestRepoPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--guest-repo="):
			opts.GuestRepoPath = strings.TrimPrefix(arg, "--guest-repo=")
			if opts.GuestRepoPath == "" {
				return openClaudeRunCLIOptions{}, errors.New("guest-repo must not be empty")
			}
		case arg == "--listen":
			if i+1 >= len(args) || args[i+1] == "" {
				return openClaudeRunCLIOptions{}, errors.New("listen must not be empty")
			}
			opts.ListenAddress = args[i+1]
			i++
		case strings.HasPrefix(arg, "--listen="):
			opts.ListenAddress = strings.TrimPrefix(arg, "--listen=")
			if opts.ListenAddress == "" {
				return openClaudeRunCLIOptions{}, errors.New("listen must not be empty")
			}
		default:
			return openClaudeRunCLIOptions{}, fmt.Errorf("unknown openclaude run flag: %s", arg)
		}
	}
	if opts.RepoPath == "" {
		return openClaudeRunCLIOptions{}, errors.New("openclaude repo path is required; use --repo or AIR_OPENCLAUDE_REPO")
	}
	return opts, nil
}

func runOpenClaudeChat(ctx context.Context, manager *session.Manager, opts openClaudeChatCLIOptions) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	inspect, err := manager.Inspect(opts.SessionID)
	if err != nil {
		return err
	}
	transcriptPath := filepath.Join(inspect.Runtime.RootPath, "openclaude-chat-transcript.jsonl")

	forwardErrCh := make(chan error, 1)
	go func() {
		forwardErrCh <- manager.ForwardOpenClaude(ctx, opts.SessionID, session.OpenClaudeForwardOptions{
			ListenAddress: opts.ListenAddress,
		})
	}()

	host, port, err := splitListenAddress(opts.ListenAddress)
	if err != nil {
		stop()
		<-forwardErrCh
		return err
	}

	if err := waitForLocalTCPReady(ctx, opts.ListenAddress, 10*time.Second); err != nil {
		stop()
		<-forwardErrCh
		return fmt.Errorf("openclaude forward did not become ready on %s: %w", opts.ListenAddress, err)
	}

	scriptPath, err := writeOpenClaudeChatScript(opts.CLIRepoPath)
	if err != nil {
		stop()
		<-forwardErrCh
		return err
	}
	defer os.Remove(scriptPath)

	cli := exec.CommandContext(ctx, "bun", "run", scriptPath)
	cli.Dir = opts.CLIRepoPath
	cli.Env = append(os.Environ(),
		"GRPC_HOST="+host,
		"GRPC_PORT="+port,
		"AIR_OPENCLAUDE_CHAT_SESSION="+opts.SessionID,
		"AIR_OPENCLAUDE_CHAT_PROVIDER="+inspect.Session.Provider,
		"AIR_OPENCLAUDE_CHAT_WORKDIR=/workspace",
		"AIR_OPENCLAUDE_CHAT_TRANSCRIPT="+transcriptPath,
	)
	cli.Stdin = os.Stdin
	cli.Stdout = os.Stdout
	cli.Stderr = os.Stderr

	fmt.Fprintf(os.Stderr, "air: openclaude chat on session %s via %s\n", opts.SessionID, opts.ListenAddress)
	if err := cli.Run(); err != nil {
		stop()
		<-forwardErrCh
		return err
	}

	stop()
	if err := <-forwardErrCh; err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func waitForLocalTCPReady(ctx context.Context, address string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		conn, err := dialer.DialContext(ctx, "tcp", address)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = os.ErrDeadlineExceeded
	}
	return lastErr
}

func runOpenClaudeOneCommand(ctx context.Context, manager *session.Manager, opts openClaudeRunCLIOptions) error {
	started, err := manager.StartOpenClaude(session.OpenClaudeStartOptions{
		Provider:      opts.Provider,
		RepoPath:      opts.RepoPath,
		GuestRepoPath: opts.GuestRepoPath,
		WorkspacePath: opts.WorkspacePath,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "air: created session %s provider=%s\n", started.SessionID, started.Provider)
	return runOpenClaudeChat(ctx, manager, openClaudeChatCLIOptions{
		SessionID:     started.SessionID,
		ListenAddress: opts.ListenAddress,
		CLIRepoPath:   opts.RepoPath,
	})
}

func writeOpenClaudeChatScript(repoPath string) (string, error) {
	dir := os.TempDir()
	f, err := os.CreateTemp(dir, "air-openclaude-chat-*.mjs")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(openClaudeChatScript); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func splitListenAddress(address string) (string, string, error) {
	host, port, ok := strings.Cut(address, ":")
	if !ok || host == "" || port == "" {
		return "", "", fmt.Errorf("invalid listen address: %s", address)
	}
	return host, port, nil
}

func replayOpenClaudeChat(manager *session.Manager, sessionID string, out io.Writer) error {
	inspect, err := manager.Inspect(sessionID)
	if err != nil {
		return err
	}
	path := filepath.Join(inspect.Runtime.RootPath, "openclaude-chat-transcript.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	const maxJSONLLine = 1024 * 1024
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxJSONLLine)

	for scanner.Scan() {
		line := scanner.Bytes()
		var event openClaudeTranscriptEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return fmt.Errorf("parse transcript line: %w", err)
		}
		renderOpenClaudeTranscriptEvent(out, &event)
	}
	return scanner.Err()
}

func renderOpenClaudeTranscriptEvent(out io.Writer, event *openClaudeTranscriptEvent) {
	switch event.Event {
	case "session_start":
		fmt.Fprintf(out, "[%s] session_start provider=%s session=%s workdir=%s target=%s\n", event.Timestamp, event.Provider, event.SessionID, event.Workdir, event.Target)
	case "user_message":
		fmt.Fprintf(out, "[%s] user> %s\n", event.Timestamp, event.Text)
	case "assistant_text_chunk":
		fmt.Fprintf(out, "[%s] assistant.chunk %s\n", event.Timestamp, event.Text)
	case "tool_start":
		fmt.Fprintf(out, "[%s] tool.start %s %s\n", event.Timestamp, event.ToolName, event.ArgsJSON)
	case "tool_result":
		fmt.Fprintf(out, "[%s] tool.result %s error=%t %s\n", event.Timestamp, event.ToolName, event.IsError, event.Output)
	case "approval_response":
		fmt.Fprintf(out, "[%s] approval %s => %s\n", event.Timestamp, event.Question, event.Reply)
	case "assistant_done":
		fmt.Fprintf(out, "[%s] assistant.done prompt_tokens=%d completion_tokens=%d %s\n", event.Timestamp, event.PromptTok, event.CompleteTok, event.FullText)
	case "server_error":
		fmt.Fprintf(out, "[%s] server.error code=%s %s\n", event.Timestamp, event.Code, event.Message)
	case "stream_error":
		fmt.Fprintf(out, "[%s] stream.error %s\n", event.Timestamp, event.Message)
	case "user_exit":
		fmt.Fprintf(out, "[%s] user.exit %s\n", event.Timestamp, event.Text)
	default:
		fmt.Fprintf(out, "[%s] %s\n", event.Timestamp, event.Event)
	}
}

const openClaudeChatScript = `import * as grpc from '@grpc/grpc-js'
import * as protoLoader from '@grpc/proto-loader'
import fs from 'fs'
import path from 'path'
import * as readline from 'readline'

const sessionId = process.env.AIR_OPENCLAUDE_CHAT_SESSION || 'air-session'
const provider = process.env.AIR_OPENCLAUDE_CHAT_PROVIDER || 'unknown'
const workdir = process.env.AIR_OPENCLAUDE_CHAT_WORKDIR || '/workspace'
const transcriptPath = process.env.AIR_OPENCLAUDE_CHAT_TRANSCRIPT || ''
const host = process.env.GRPC_HOST || '127.0.0.1'
const port = process.env.GRPC_PORT || '50052'
const protoPath = path.resolve(process.cwd(), 'src/proto/openclaude.proto')

const packageDefinition = protoLoader.loadSync(protoPath, {
  keepCase: true,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true,
})

const protoDescriptor = grpc.loadPackageDefinition(packageDefinition)
const openclaudeProto = protoDescriptor.openclaude.v1

const rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout,
})

function askQuestion(query) {
  return new Promise(resolve => {
    rl.question(query, resolve)
  })
}

function appendTranscript(eventType, payload = {}) {
  if (!transcriptPath) {
    return
  }
  try {
    const body = {
      ts: new Date().toISOString(),
      event: eventType,
      session_id: sessionId,
      provider,
      workdir,
      ...payload,
    }
    fs.appendFileSync(transcriptPath, JSON.stringify(body) + '\n')
  } catch (_) {
  }
}

async function main() {
  const client = new openclaudeProto.AgentService(
    host + ':' + port,
    grpc.credentials.createInsecure(),
  )

  let call = null
  let textStreamed = false
  let awaitingReply = false

  const promptLabel = '\x1b[36mair:openclaude@' + sessionId + '\x1b[0m'
  const approveLabel = '\x1b[33mair:approve@' + sessionId + '\x1b[0m'

  const promptUser = async () => {
    const message = await askQuestion('\n' + promptLabel + '> ')
    const trimmed = message.trim().toLowerCase()
    if (trimmed === '/exit' || trimmed === '/quit') {
      appendTranscript('user_exit', { text: '/exit' })
      console.log('bye')
      rl.close()
      process.exit(0)
    }
    appendTranscript('user_message', { text: message })
    if (!call || call.destroyed || call.writableEnded) {
      startStream()
    }
    awaitingReply = true
    call.write({
      request: {
        session_id: sessionId,
        message,
        working_directory: workdir,
      },
    })
  }

  const startStream = () => {
    call = client.Chat()
    textStreamed = false

    call.on('data', async (serverMessage) => {
      if (serverMessage.text_chunk) {
        process.stdout.write(serverMessage.text_chunk.text)
        textStreamed = true
        appendTranscript('assistant_text_chunk', { text: serverMessage.text_chunk.text })
        return
      }
      if (serverMessage.tool_start) {
        console.log('\n\x1b[36m[Tool Call]\x1b[0m \x1b[1m' + serverMessage.tool_start.tool_name + '\x1b[0m')
        console.log('\x1b[90m' + serverMessage.tool_start.arguments_json + '\x1b[0m\n')
        appendTranscript('tool_start', {
          tool_name: serverMessage.tool_start.tool_name,
          arguments_json: serverMessage.tool_start.arguments_json,
          tool_use_id: serverMessage.tool_start.tool_use_id,
        })
        return
      }
      if (serverMessage.tool_result) {
        console.log('\n\x1b[32m[Tool Result]\x1b[0m \x1b[1m' + serverMessage.tool_result.tool_name + '\x1b[0m')
        const out = serverMessage.tool_result.output || ''
        if (out.length > 500) {
          console.log('\x1b[90m' + out.substring(0, 500) + '...\n(Output truncated, total length: ' + out.length + ')\x1b[0m')
        } else {
          console.log('\x1b[90m' + out + '\x1b[0m')
        }
        appendTranscript('tool_result', {
          tool_name: serverMessage.tool_result.tool_name,
          output: out,
          is_error: !!serverMessage.tool_result.is_error,
          tool_use_id: serverMessage.tool_result.tool_use_id,
        })
        return
      }
      if (serverMessage.action_required) {
        const action = serverMessage.action_required
        const reply = await askQuestion('\n' + approveLabel + '> ' + action.question + ' (y/n) ')
        appendTranscript('approval_response', {
          question: action.question,
          prompt_id: action.prompt_id,
          reply: reply.trim(),
        })
        call.write({
          input: {
            prompt_id: action.prompt_id,
            reply: reply.trim(),
          },
        })
        return
      }
      if (serverMessage.done) {
        if (!textStreamed && serverMessage.done.full_text) {
          process.stdout.write(serverMessage.done.full_text)
        }
        textStreamed = false
        awaitingReply = false
        appendTranscript('assistant_done', {
          full_text: serverMessage.done.full_text || '',
          prompt_tokens: serverMessage.done.prompt_tokens || 0,
          completion_tokens: serverMessage.done.completion_tokens || 0,
        })
        console.log('\n\x1b[32m[Done]\x1b[0m')
        if (call) {
          call.end()
          call = null
        }
        promptUser()
        return
      }
      if (serverMessage.error) {
        awaitingReply = false
        console.error('\n\x1b[31m[Server Error]\x1b[0m ' + serverMessage.error.message)
        appendTranscript('server_error', {
          message: serverMessage.error.message,
          code: serverMessage.error.code || '',
        })
        if (call) {
          call.end()
          call = null
        }
        promptUser()
      }
    })

    call.on('end', () => {
      call = null
    })

    call.on('error', (err) => {
      call = null
      if (!awaitingReply) {
        return
      }
      awaitingReply = false
      console.error('\n\x1b[31m[Stream Error]\x1b[0m', err.message)
      appendTranscript('stream_error', { message: err.message })
      promptUser()
    })
  }

  console.log('\x1b[32mAIR OpenClaude Chat\x1b[0m')
  console.log('\x1b[90mprovider=' + provider + ' session=' + sessionId + ' workdir=' + workdir + '\x1b[0m')
  console.log('\x1b[90mConnected to ' + host + ':' + port + '. Type /exit to quit.\x1b[0m')
  if (transcriptPath) {
    console.log('\x1b[90mtranscript=' + transcriptPath + '\x1b[0m')
    appendTranscript('session_start', { target: host + ':' + port })
  }
  await promptUser()
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
`

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
		case arg == "--workspace":
			if i+1 >= len(args) || args[i+1] == "" {
				return runCLIOptions{}, "", errors.New("workspace must not be empty")
			}
			opts.WorkspacePath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--workspace="):
			opts.WorkspacePath = strings.TrimPrefix(arg, "--workspace=")
			if opts.WorkspacePath == "" {
				return runCLIOptions{}, "", errors.New("workspace must not be empty")
			}
		default:
			commandArgs = append(commandArgs, args[i:]...)
			i = len(args)
		}
	}

	command := strings.TrimSpace(strings.Join(commandArgs, " "))
	if command == "" {
		return runCLIOptions{}, "", errors.New("usage: air run [--provider local|firecracker] [--timeout 30s] [--memory-mib 256] [--vcpu-count 1] [--workspace PATH] [--human] -- <command>")
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
