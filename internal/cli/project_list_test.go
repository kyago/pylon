package cli

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestProjectList(t *testing.T) {
	root := setupDeleteWorkspace(t, "myapp", true)
	oldWorkspace, oldJSON := flagWorkspace, flagJSON
	flagWorkspace = root
	defer func() { flagWorkspace, flagJSON = oldWorkspace, oldJSON }()

	var text bytes.Buffer
	if err := runProjectList(&text); err != nil {
		t.Fatal(err)
	}
	if want := "myapp\t" + filepath.Join(root, "myapp") + "\n"; text.String() != want {
		t.Errorf("text = %q, want %q", text.String(), want)
	}

	flagJSON = true
	var js bytes.Buffer
	if err := runProjectList(&js); err != nil {
		t.Fatal(err)
	}
	if want := `[{"name":"myapp","path":"` + filepath.Join(root, "myapp") + `"}]` + "\n"; js.String() != want {
		t.Errorf("json = %q, want %q", js.String(), want)
	}
}

func TestProjectListEmptyJSON(t *testing.T) {
	root := setupDeleteWorkspace(t, "ghost", false)
	oldWorkspace, oldJSON := flagWorkspace, flagJSON
	flagWorkspace, flagJSON = root, true
	defer func() { flagWorkspace, flagJSON = oldWorkspace, oldJSON }()

	var js bytes.Buffer
	if err := runProjectList(&js); err != nil {
		t.Fatal(err)
	}
	if js.String() != "[]\n" {
		t.Errorf("json = %q, want []", js.String())
	}
}
