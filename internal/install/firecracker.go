package install

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/darunshen/AIR/internal/buildinfo"
)

const (
	defaultOfficialReleaseBaseURL = "https://github.com/darunshen/AIR/releases/download"
	defaultSystemInstallDir       = "/usr/lib/air/firecracker"
	defaultLocalInstallDir        = "/usr/local/lib/air/firecracker"
)

func OfficialFirecrackerBundleURL(version, goos, goarch string) (string, error) {
	if override := os.Getenv("AIR_OFFICIAL_FIRECRACKER_BUNDLE_URL"); override != "" {
		return override, nil
	}
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	if goos != "linux" {
		return "", fmt.Errorf("official firecracker bundle is only available on linux hosts")
	}
	if goarch != "amd64" && goarch != "arm64" {
		return "", fmt.Errorf("unsupported architecture for official firecracker bundle: %s", goarch)
	}
	baseURL := os.Getenv("AIR_OFFICIAL_RELEASE_BASE_URL")
	if baseURL == "" {
		baseURL = defaultOfficialReleaseBaseURL
	}
	assetName := fmt.Sprintf("air_firecracker_%s_%s.tar.gz", goos, goarch)
	if strings.HasPrefix(version, "v") {
		return fmt.Sprintf("%s/%s/%s", strings.TrimRight(baseURL, "/"), version, assetName), nil
	}
	baseURL = strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/download")
	return fmt.Sprintf("%s/latest/download/%s", baseURL, assetName), nil
}

func DefaultFirecrackerInstallDir() string {
	for _, candidate := range []string{
		defaultSystemInstallDir,
		defaultLocalInstallDir,
		userInstallDir(),
	} {
		if candidate == "" {
			continue
		}
		if canWriteDir(candidate) {
			return candidate
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Join(cwd, "assets", "firecracker")
	}
	return "assets/firecracker"
}

func DownloadOfficialFirecrackerBundle(ctx context.Context, version, outputDir string) (string, error) {
	if outputDir == "" {
		outputDir = DefaultFirecrackerInstallDir()
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}

	var lastErr error
	for _, url := range officialBundleCandidates(version, runtime.GOOS, runtime.GOARCH) {
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
			lastErr = fmt.Errorf("download official firecracker bundle failed: %s", resp.Status)
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
		lastErr = errors.New("download official firecracker bundle failed")
	}
	return "", lastErr
}

func BuildCustomInstallGuide(outputDir string) string {
	if outputDir == "" {
		outputDir = DefaultFirecrackerInstallDir()
	}
	return strings.Join([]string{
		fmt.Sprintf("请自行准备以下文件并放到 %s：", outputDir),
		"- firecracker",
		"- hello-vmlinux.bin",
		"- hello-rootfs-air.ext4 或 hello-rootfs.ext4",
		"放好后执行：air doctor --provider firecracker --human",
	}, "\n")
}

func CurrentVersion() string {
	return buildinfo.Current().Version
}

func officialBundleCandidates(version, goos, goarch string) []string {
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

	appendCandidate(OfficialFirecrackerBundleURL(version, goos, goarch))
	appendCandidate(OfficialFirecrackerBundleURL("latest", goos, goarch))
	return candidates
}

func extractTarGz(reader io.Reader, outputDir string) error {
	gzr, err := gzip.NewReader(reader)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		targetPath := filepath.Join(outputDir, header.Name)
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(outputDir)+string(os.PathSeparator)) &&
			filepath.Clean(targetPath) != filepath.Clean(outputDir) {
			return fmt.Errorf("invalid archive entry path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, tr); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry type: %d", header.Typeflag)
		}
	}
}

func userInstallDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share", "air", "firecracker")
}

func canWriteDir(path string) bool {
	parent := path
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		testFile := filepath.Join(path, ".air-write-check")
		file, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return false
		}
		_ = file.Close()
		_ = os.Remove(testFile)
		return true
	}
	parent = nearestExistingDir(filepath.Dir(path))
	if parent == "" {
		return false
	}
	testFile := filepath.Join(parent, ".air-write-check")
	file, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return false
	}
	_ = file.Close()
	_ = os.Remove(testFile)
	return true
}

func nearestExistingDir(path string) string {
	current := path
	for current != "" && current != "." && current != string(os.PathSeparator) {
		info, err := os.Stat(current)
		if err == nil && info.IsDir() {
			return current
		}
		current = filepath.Dir(current)
	}
	if info, err := os.Stat(string(os.PathSeparator)); err == nil && info.IsDir() {
		return string(os.PathSeparator)
	}
	return ""
}
