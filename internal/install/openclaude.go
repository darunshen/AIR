package install

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	defaultSystemOpenClaudeInstallDir = "/usr/lib/air/openclaude"
	defaultLocalOpenClaudeInstallDir  = "/usr/local/lib/air/openclaude"
	defaultOpenClaudeGuestRootfsName  = "openclaude-ubuntu-rootfs.ext4"
)

func OfficialOpenClaudeBundleURL(version, goos, goarch string) (string, error) {
	if override := os.Getenv("AIR_OFFICIAL_OPENCLAUDE_BUNDLE_URL"); override != "" {
		return override, nil
	}
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	if goos != "linux" {
		return "", fmt.Errorf("official openclaude bundle is only available on linux hosts")
	}
	if goarch != "amd64" {
		return "", fmt.Errorf("unsupported architecture for official openclaude bundle: %s", goarch)
	}
	baseURL := os.Getenv("AIR_OFFICIAL_RELEASE_BASE_URL")
	if baseURL == "" {
		baseURL = defaultOfficialReleaseBaseURL
	}
	assetName := fmt.Sprintf("air_openclaude_%s_%s.tar.gz", goos, goarch)
	if strings.HasPrefix(version, "v") {
		return fmt.Sprintf("%s/%s/%s", strings.TrimRight(baseURL, "/"), version, assetName), nil
	}
	baseURL = strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/download")
	return fmt.Sprintf("%s/latest/download/%s", baseURL, assetName), nil
}

func DefaultOpenClaudeInstallDir() string {
	for _, candidate := range []string{
		defaultSystemOpenClaudeInstallDir,
		defaultLocalOpenClaudeInstallDir,
		userOpenClaudeInstallDir(),
	} {
		if candidate == "" {
			continue
		}
		if canWriteDir(candidate) {
			return candidate
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Join(cwd, "runtime", "openclaude")
	}
	return filepath.Join("runtime", "openclaude")
}

func DownloadOfficialOpenClaudeBundle(ctx context.Context, version, outputDir string) (string, error) {
	if outputDir == "" {
		outputDir = DefaultOpenClaudeInstallDir()
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}

	var lastErr error
	for _, url := range officialOpenClaudeBundleCandidates(version, runtime.GOOS, runtime.GOARCH) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("download official openclaude bundle failed: %s", resp.Status)
			_ = resp.Body.Close()
			continue
		}
		err = extractTarGz(resp.Body, outputDir)
		_ = resp.Body.Close()
		if err != nil {
			return "", err
		}
		return outputDir, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("download official openclaude bundle failed")
	}
	return "", lastErr
}

func OfficialOpenClaudeGuestBundleURL(version, goos, goarch string) (string, error) {
	if override := os.Getenv("AIR_OFFICIAL_OPENCLAUDE_GUEST_BUNDLE_URL"); override != "" {
		return override, nil
	}
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	if goos != "linux" {
		return "", fmt.Errorf("official openclaude guest bundle is only available on linux hosts")
	}
	if goarch != "amd64" && goarch != "arm64" {
		return "", fmt.Errorf("unsupported architecture for official openclaude guest bundle: %s", goarch)
	}
	baseURL := os.Getenv("AIR_OFFICIAL_RELEASE_BASE_URL")
	if baseURL == "" {
		baseURL = defaultOfficialReleaseBaseURL
	}
	assetName := fmt.Sprintf("air_openclaude_firecracker_%s_%s.tar.gz", goos, goarch)
	if strings.HasPrefix(version, "v") {
		return fmt.Sprintf("%s/%s/%s", strings.TrimRight(baseURL, "/"), version, assetName), nil
	}
	baseURL = strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/download")
	return fmt.Sprintf("%s/latest/download/%s", baseURL, assetName), nil
}

func DownloadOfficialOpenClaudeGuestBundle(ctx context.Context, version, outputDir string) (string, error) {
	if outputDir == "" {
		outputDir = DefaultFirecrackerInstallDir()
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}

	var lastErr error
	for _, url := range officialOpenClaudeGuestBundleCandidates(version, runtime.GOOS, runtime.GOARCH) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("download official openclaude guest bundle failed: %s", resp.Status)
			_ = resp.Body.Close()
			continue
		}
		err = extractTarGz(resp.Body, outputDir)
		_ = resp.Body.Close()
		if err != nil {
			return "", err
		}
		return filepath.Join(outputDir, defaultOpenClaudeGuestRootfsName), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("download official openclaude guest bundle failed")
	}
	return "", lastErr
}

func officialOpenClaudeBundleCandidates(version, goos, goarch string) []string {
	candidates := make([]string, 0, 2)
	appendCandidate := func(value string, err error) {
		if err != nil || value == "" {
			return
		}
		for _, candidate := range candidates {
			if candidate == value {
				return
			}
		}
		candidates = append(candidates, value)
	}

	appendCandidate(OfficialOpenClaudeBundleURL(version, goos, goarch))
	appendCandidate(OfficialOpenClaudeBundleURL("latest", goos, goarch))
	return candidates
}

func officialOpenClaudeGuestBundleCandidates(version, goos, goarch string) []string {
	candidates := make([]string, 0, 2)
	appendCandidate := func(value string, err error) {
		if err != nil || value == "" {
			return
		}
		for _, candidate := range candidates {
			if candidate == value {
				return
			}
		}
		candidates = append(candidates, value)
	}

	appendCandidate(OfficialOpenClaudeGuestBundleURL(version, goos, goarch))
	appendCandidate(OfficialOpenClaudeGuestBundleURL("latest", goos, goarch))
	return candidates
}

func userOpenClaudeInstallDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share", "air", "openclaude")
}
