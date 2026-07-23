// internal/memory/frontmatter.go
package memory

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Entry is a single memory record backed by one markdown file.
type Entry struct {
	ProjectID  string    `yaml:"-"`
	Category   string    `yaml:"category"`
	Key        string    `yaml:"key"`
	Author     string    `yaml:"author,omitempty"`
	Confidence float64   `yaml:"confidence"`
	CreatedAt  time.Time `yaml:"created_at"`
	Content    string    `yaml:"-"`
	Path       string    `yaml:"-"` // 워크스페이스 루트 기준 상대 경로 (slash 구분)
}

const frontmatterDelim = "---\n"

// marshalEntry renders an Entry as markdown with YAML frontmatter.
func marshalEntry(e *Entry) ([]byte, error) {
	head, err := yaml.Marshal(e)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString(frontmatterDelim)
	buf.Write(head)
	buf.WriteString(frontmatterDelim)
	buf.WriteString("\n")
	buf.WriteString(strings.TrimRight(e.Content, "\n"))
	buf.WriteString("\n")
	return buf.Bytes(), nil
}

// parseEntry parses a markdown memory file produced by marshalEntry.
func parseEntry(data []byte) (*Entry, error) {
	text := string(data)
	if !strings.HasPrefix(text, frontmatterDelim) {
		return nil, fmt.Errorf("frontmatter가 없습니다")
	}
	rest := text[len(frontmatterDelim):]
	const sep = "\n---\n"
	idx := strings.Index(rest, sep)
	if idx < 0 {
		return nil, fmt.Errorf("frontmatter 종료 구분자가 없습니다")
	}
	var e Entry
	if err := yaml.Unmarshal([]byte(rest[:idx+1]), &e); err != nil {
		return nil, fmt.Errorf("frontmatter 파싱 실패: %w", err)
	}
	e.Content = strings.TrimSpace(rest[idx+len(sep):])
	return &e, nil
}
