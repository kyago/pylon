package cli

import (
	"fmt"

	"github.com/kyago/pylon/internal/layout"
	"github.com/kyago/pylon/internal/store"
)

func registerProjectInDB(root, projectName, projectDir, stackLang string) error {
	dbPath := layout.DBPath(root)
	s, err := store.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("store open: %w", err)
	}
	defer s.Close()

	if err := s.Migrate(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := s.UpsertProject(&store.ProjectRecord{
		ProjectID: projectName,
		Path:      projectDir,
		Stack:     stackLang,
	}); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}
	return nil
}
