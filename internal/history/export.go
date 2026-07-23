package history

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (m *Manager) Export(checkin, output string) error {
	if checkin == "" || output == "" {
		return fmt.Errorf("checkin과 output 경로가 필요합니다")
	}
	if !m.IsInitialized() {
		return fmt.Errorf("history 저장소가 초기화되지 않았습니다")
	}
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("export 대상이 이미 존재합니다: %s", output)
	} else if !os.IsNotExist(err) {
		return err
	}
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absOutput), 0755); err != nil {
		return err
	}
	archive, err := os.CreateTemp(m.historyDir(), ".export-*.tar.gz")
	if err != nil {
		return err
	}
	archivePath := archive.Name()
	if err := archive.Close(); err != nil {
		return err
	}
	defer os.Remove(archivePath)
	if _, err := m.Runner.Run(m.Root, "tarball", checkin, archivePath, "--name", "pylon-history", "-R", m.repositoryPath()); err != nil {
		return fmt.Errorf("history export 실패: %w", err)
	}
	if err := extractTarball(archivePath, absOutput); err != nil {
		_ = os.RemoveAll(absOutput)
		return fmt.Errorf("history export 압축 해제 실패: %w", err)
	}
	return nil
}

func extractTarball(archivePath, output string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		clean := filepath.Clean(header.Name)
		if filepath.ToSlash(clean) == "pylon-history" && header.Typeflag == tar.TypeDir {
			continue
		}
		parts := strings.Split(filepath.ToSlash(clean), "/")
		if len(parts) < 2 || parts[0] != "pylon-history" {
			return fmt.Errorf("예상하지 못한 archive 경로: %s", header.Name)
		}
		rel := filepath.FromSlash(strings.Join(parts[1:], "/"))
		target := filepath.Join(output, rel)
		if !strings.HasPrefix(target, output+string(filepath.Separator)) {
			return fmt.Errorf("안전하지 않은 archive 경로: %s", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			mode := os.FileMode(header.Mode) & 0777
			if mode == 0 {
				mode = 0644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, reader)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("지원하지 않는 archive 항목: %s", header.Name)
		}
	}
}
