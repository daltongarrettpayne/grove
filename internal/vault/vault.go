// Package vault provides helpers for managing files inside the vault directory
// tree (the PARA-organized knowledge base grove is built around).
package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteContextMD writes a context.md file to dir with YAML frontmatter.
//
// The output format:
//
//	---
//	name: <name>
//	created: <created>
//	code: <true|false>
//	source_set:
//	  - <path>
//	  - <path>
//	---
//
//	_Add a description._
//
// If code is false and sourceSet is empty, the source_set key is omitted.
// Does not use a YAML library — simple string building is used throughout.
func WriteContextMD(dir, name, created string, code bool, sourceSet []string) error {
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", name))
	sb.WriteString(fmt.Sprintf("created: %s\n", created))
	sb.WriteString(fmt.Sprintf("code: %v\n", code))

	if code || len(sourceSet) > 0 {
		sb.WriteString("source_set:\n")
		for _, p := range sourceSet {
			sb.WriteString(fmt.Sprintf("  - %s\n", p))
		}
	}

	sb.WriteString("---\n")
	sb.WriteString("\n_Add a description._\n")

	dest := filepath.Join(dir, "context.md")
	if err := os.WriteFile(dest, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("writing context.md to %s: %w", dest, err)
	}
	return nil
}
