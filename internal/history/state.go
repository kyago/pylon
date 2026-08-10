package history

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// digestTree hashes every file under root, returning a combined digest and a
// per-file (relative path → sha256) artifact map.
func digestTree(root string) (string, map[string]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	sort.Strings(paths)
	return digestPaths(root, paths)
}

func digestPaths(root string, paths []string) (string, map[string]string, error) {
	total := sha256.New()
	artifacts := make(map[string]string, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", nil, err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		sum := sha256.Sum256(data)
		artifacts[rel] = hex.EncodeToString(sum[:])
		total.Write([]byte(rel))
		total.Write([]byte{0})
		total.Write(data)
		total.Write([]byte{0})
	}
	return hex.EncodeToString(total.Sum(nil)), artifacts, nil
}

func validateDigestTree(root string, manifest Manifest) error {
	paths := make([]string, 0, len(manifest.Artifacts))
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() == "manifest.json" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, ok := manifest.Artifacts[relative]; !ok {
			return fmt.Errorf("unexpected checkpoint artifact: %s", relative)
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return err
	}
	if len(paths) != len(manifest.Artifacts) {
		return fmt.Errorf("checkpoint artifact count mismatch: got %d want %d", len(paths), len(manifest.Artifacts))
	}
	sort.Strings(paths)
	digest, artifacts, err := digestPaths(root, paths)
	if err != nil {
		return err
	}
	if digest != manifest.Digest {
		return fmt.Errorf("checkpoint digest mismatch: got %s want %s", digest, manifest.Digest)
	}
	for path, expected := range manifest.Artifacts {
		if artifacts[path] != expected {
			return fmt.Errorf("checkpoint artifact digest mismatch: %s", path)
		}
	}
	return nil
}

func validatePipelineID(id string) error {
	if id == "" || id == "." || id == ".." || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return fmt.Errorf("유효하지 않은 pipeline ID: %q", id)
	}
	return nil
}

func copyIfExists(source, dest string) error {
	data, err := os.ReadFile(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0644)
}
