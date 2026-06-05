package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/daltongarrettpayne/grove/internal/model"
	"github.com/daltongarrettpayne/grove/internal/scanner"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage grove sessions",
}

var sessionOpenCmd = &cobra.Command{
	Use:   "open <context>",
	Short: "Open or attach to a context session (idempotent)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionOpen(args[0])
	},
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available contexts as JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionList()
	},
}

func init() {
	sessionCmd.AddCommand(sessionOpenCmd)
	sessionCmd.AddCommand(sessionListCmd)
	rootCmd.AddCommand(sessionCmd)
}

func runSessionOpen(name string) error {
	// TODO(build-order-3): scan source set, build session idempotently, attach
	return nil
}

func runSessionList() error {
	// Vault tiers whose subdirectories are workspace contexts, per the design doc.
	tiers := []string{"01-Projects", "02-Areas"}

	var contexts []model.Context
	for _, tier := range tiers {
		tierDir := filepath.Join(cfg.HomeRoot, tier)
		entries, err := os.ReadDir(tierDir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("reading vault tier %s: %w", tierDir, err)
		}
		for _, e := range entries {
			if !e.IsDir() || isHiddenName(e.Name()) {
				continue
			}
			name := e.Name()
			homeDir := filepath.Join(tierDir, name)

			// lanes is initialised as empty (not nil) so JSON encodes [] not null.
			lanes := make([]model.Lane, 0)
			codeContainer := filepath.Join(cfg.CodeRoot, name)
			if _, statErr := os.Stat(codeContainer); statErr == nil {
				scanned, scanErr := scanner.ScanSourceSet(model.SourceSet{codeContainer})
				if scanErr != nil {
					return fmt.Errorf("scanning %s: %w", codeContainer, scanErr)
				}
				lanes = scanned
			}

			contexts = append(contexts, model.Context{
				Name:      name,
				HomeDir:   homeDir,
				SourceSet: model.SourceSet{codeContainer},
				Lanes:     lanes,
			})
		}
	}

	out, err := json.MarshalIndent(contexts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling contexts: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

// isHiddenName reports whether a directory name starts with a dot.
func isHiddenName(name string) bool {
	return len(name) > 0 && name[0] == '.'
}
