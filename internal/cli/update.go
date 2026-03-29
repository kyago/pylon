package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// httpClient is the HTTP client used for GitHub API and download requests.
var httpClient = &http.Client{Timeout: 5 * time.Minute}

// maxBinarySize is the maximum allowed download size (200 MB).
const maxBinarySize = 200 * 1024 * 1024

const (
	githubRepo    = "kyago/pylon"
	releaseAPIURL = "https://api.github.com/repos/" + githubRepo + "/releases"
)

// calverRe matches CalVer: YYYY.M.SEQ (e.g. 2026.3.1, 2026.12.15).
var calverRe = regexp.MustCompile(`^\d{4}\.\d{1,2}\.\d+$`)

// githubRelease represents a subset of the GitHub Release API response.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

// githubAsset represents a release asset.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update [version]",
		Short: "Update pylon to the latest or a specific version",
		Long: `Downloads and installs pylon from GitHub Releases.

Without arguments, installs the latest version.
With a version argument (e.g. 2026.3.1), installs that specific version.

Examples:
  pylon update          # install latest
  pylon update 2026.3.1 # install specific version`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := resolveUpdateTarget(args)
			if err != nil {
				return err
			}

			fmt.Printf("현재 버전: %s\n", version)

			// Fetch release info
			release, err := fetchRelease(target)
			if err != nil {
				return fmt.Errorf("릴리스 조회 실패: %w", err)
			}

			if target == "latest" {
				fmt.Printf("최신 버전 %s을(를) 설치합니다...\n", release.TagName)
			} else {
				fmt.Printf("버전 %s을(를) 설치합니다...\n", release.TagName)
			}

			// Find matching asset for current OS/arch
			assetName := buildAssetName(release.TagName)
			var downloadURL string
			for _, a := range release.Assets {
				if a.Name == assetName {
					downloadURL = a.BrowserDownloadURL
					break
				}
			}
			if downloadURL == "" {
				return fmt.Errorf("현재 플랫폼(%s/%s)에 맞는 바이너리를 찾을 수 없습니다: %s", runtime.GOOS, runtime.GOARCH, assetName)
			}

			// Download archive
			fmt.Printf("다운로드 중: %s\n", assetName)
			archivePath, err := downloadFile(downloadURL)
			if err != nil {
				return fmt.Errorf("다운로드 실패: %w", err)
			}
			defer os.Remove(archivePath)

			// Extract binary
			binaryPath, err := extractBinary(archivePath, assetName)
			if err != nil {
				return fmt.Errorf("바이너리 추출 실패: %w", err)
			}
			defer os.Remove(binaryPath)

			// Replace current binary
			currentBinary, err := os.Executable()
			if err != nil {
				return fmt.Errorf("현재 바이너리 경로를 찾을 수 없습니다: %w", err)
			}
			currentBinary, err = filepath.EvalSymlinks(currentBinary)
			if err != nil {
				return fmt.Errorf("심볼릭 링크 해석 실패: %w", err)
			}

			if err := replaceBinary(currentBinary, binaryPath); err != nil {
				return fmt.Errorf("바이너리 교체 실패: %w", err)
			}

			fmt.Printf("✅ pylon %s 설치 완료\n", release.TagName)

			// Run config sync using the NEW binary
			fmt.Println("\n설정 동기화 중...")
			doctorCmd := exec.Command(currentBinary, "doctor")
			doctorCmd.Stdout = os.Stdout
			doctorCmd.Stderr = os.Stderr
			if err := doctorCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "⚠ 설정 동기화 실패: %v (수동으로 'pylon doctor'를 실행하세요)\n", err)
			}

			return nil
		},
	}
}

// resolveUpdateTarget determines the version target from CLI args.
func resolveUpdateTarget(args []string) (string, error) {
	if len(args) == 0 {
		return "latest", nil
	}

	ver := strings.TrimSpace(args[0])
	if ver == "" {
		return "latest", nil
	}

	if strings.EqualFold(ver, "latest") {
		return "latest", nil
	}

	if !validateVersion(ver) {
		return "", fmt.Errorf("잘못된 버전 형식입니다: %q (예: 2026.3.1)", ver)
	}

	return ver, nil
}

// validateVersion checks whether s is a valid CalVer string or the keyword "latest".
func validateVersion(s string) bool {
	if s == "" {
		return false
	}
	if strings.EqualFold(s, "latest") {
		return true
	}
	return calverRe.MatchString(s)
}

// fetchRelease fetches release info from GitHub API.
func fetchRelease(target string) (*githubRelease, error) {
	var url string
	if target == "latest" {
		url = releaseAPIURL + "/latest"
	} else {
		url = releaseAPIURL + "/tags/" + target
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("버전 %q을(를) 찾을 수 없습니다", target)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 응답 오류: %s", resp.Status)
	}

	const maxAPIResponse = 1 * 1024 * 1024 // 1 MB
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIResponse)).Decode(&release); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return &release, nil
}

// buildAssetName returns the expected archive filename for the current platform.
func buildAssetName(tag string) string {
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("pylon_%s_%s_%s.%s", tag, runtime.GOOS, runtime.GOARCH, ext)
}

// downloadFile downloads a URL to a temporary file and returns its path.
func downloadFile(url string) (string, error) {
	resp, err := httpClient.Get(url) //nolint:gosec // URL is from GitHub API
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("다운로드 응답 오류: %s", resp.Status)
	}

	if resp.ContentLength > 0 && resp.ContentLength > maxBinarySize {
		return "", fmt.Errorf("다운로드 크기 제한 초과: 최대 %d바이트, 응답 크기 %d바이트", maxBinarySize, resp.ContentLength)
	}

	tmp, err := os.CreateTemp("", "pylon-update-*")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxBinarySize+1))
	if err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if n > maxBinarySize {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("다운로드 크기 제한 초과: 최대 %d바이트", maxBinarySize)
	}

	return tmp.Name(), nil
}

// extractBinary extracts the pylon binary from the archive.
func extractBinary(archivePath, assetName string) (string, error) {
	if strings.HasSuffix(assetName, ".zip") {
		return extractFromZip(archivePath)
	}
	return extractFromTarGz(archivePath)
}

// extractFromTarGz extracts the pylon binary from a .tar.gz archive.
func extractFromTarGz(archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		if filepath.Base(hdr.Name) == "pylon" && hdr.Typeflag == tar.TypeReg {
			if hdr.Size > maxBinarySize {
				return "", fmt.Errorf("pylon 바이너리 크기(%d바이트)가 최대 허용 크기(%d바이트)를 초과합니다", hdr.Size, maxBinarySize)
			}
			tmp, err := os.CreateTemp("", "pylon-bin-*")
			if err != nil {
				return "", err
			}
			n, err := io.Copy(tmp, io.LimitReader(tr, maxBinarySize+1))
			if err != nil {
				tmp.Close()
				os.Remove(tmp.Name())
				return "", err
			}
			if n > maxBinarySize {
				tmp.Close()
				os.Remove(tmp.Name())
				return "", fmt.Errorf("pylon 바이너리 크기가 최대 허용 크기(%d바이트)를 초과합니다", maxBinarySize)
			}
			tmp.Close()
			if err := os.Chmod(tmp.Name(), 0o755); err != nil {
				os.Remove(tmp.Name())
				return "", err
			}
			return tmp.Name(), nil
		}
	}

	return "", fmt.Errorf("아카이브에서 pylon 바이너리를 찾을 수 없습니다")
}

// extractFromZip extracts the pylon binary from a .zip archive.
func extractFromZip(archivePath string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		if name == "pylon" || name == "pylon.exe" {
			if f.UncompressedSize64 > maxBinarySize {
				return "", fmt.Errorf("pylon 바이너리 크기(%d바이트)가 최대 허용 크기(%d바이트)를 초과합니다", f.UncompressedSize64, maxBinarySize)
			}
			rc, err := f.Open()
			if err != nil {
				return "", err
			}

			tmp, err := os.CreateTemp("", "pylon-bin-*")
			if err != nil {
				rc.Close()
				return "", err
			}
			n, err := io.Copy(tmp, io.LimitReader(rc, maxBinarySize+1))
			rc.Close()
			if err != nil {
				tmp.Close()
				os.Remove(tmp.Name())
				return "", err
			}
			if n > maxBinarySize {
				tmp.Close()
				os.Remove(tmp.Name())
				return "", fmt.Errorf("pylon 바이너리 크기가 최대 허용 크기(%d바이트)를 초과합니다", maxBinarySize)
			}
			tmp.Close()
			if err := os.Chmod(tmp.Name(), 0o755); err != nil {
				os.Remove(tmp.Name())
				return "", err
			}
			return tmp.Name(), nil
		}
	}

	return "", fmt.Errorf("아카이브에서 pylon 바이너리를 찾을 수 없습니다")
}

// replaceBinary replaces the current binary with the new one.
// On Unix, it uses atomic rename. On Windows, self-update is not supported.
func replaceBinary(dst, src string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("Windows에서는 실행 중인 바이너리를 자동 교체할 수 없습니다. " +
			"GitHub Releases에서 바이너리를 수동으로 다운로드하세요: " +
			"https://github.com/" + githubRepo + "/releases")
	}

	// Read permissions from current binary
	info, err := os.Stat(dst)
	if err != nil {
		return err
	}

	// Write new binary to a temp file in the same directory (same filesystem for atomic rename)
	dstDir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dstDir, ".pylon-update-*")
	if err != nil {
		return fmt.Errorf("임시 파일 생성 실패: %w", err)
	}
	tmpPath := tmp.Name()

	srcFile, err := os.Open(src)
	if err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}

	if _, err := io.Copy(tmp, srcFile); err != nil {
		srcFile.Close()
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	srcFile.Close()
	tmp.Close()

	// Set permissions to match original binary
	if err := os.Chmod(tmpPath, info.Mode()); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// Atomic rename (same filesystem guarantees atomicity on Unix)
	if err := os.Rename(tmpPath, dst); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("바이너리 교체 실패: %w", err)
	}

	return nil
}
