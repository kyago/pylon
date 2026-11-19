package slug

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user login", "user-login"},
		{"한글테스트", "한글테스트"},
		{"Hello World!", "hello-world"},
		{"---", "task"},
		{"", "task"},
		{"  spaces  ", "spaces"},
		{"로그인 기능 구현", "로그인-기능-구현"},
		{"mixed 한글 text", "mixed-한글-text"},
		{"UPPER CASE", "upper-case"},
		{"special!@#chars", "special-chars"},
		{"multiple   spaces", "multiple-spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Slugify(tt.input)
			if got != tt.expected {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
