package model

import (
	"fmt"
	"regexp"
	"strings"
)

// kebabRe matches a valid kebab-case segment: lowercase letters, digits, and
// hyphens, starting with a letter or digit. Underscores and uppercase are not
// allowed.
var kebabRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// branchTypes is the set of valid conventional branch type prefixes.
var branchTypes = map[string]bool{
	"feat":     true,
	"fix":      true,
	"chore":    true,
	"refactor": true,
	"docs":     true,
	"test":     true,
	"hotfix":   true,
}

// baseBranches are well-known long-lived branch names that are always valid.
var baseBranches = map[string]bool{
	"main":    true,
	"master":  true,
	"develop": true,
	"staging": true,
	"prod":    true,
}

// IsKebabCase reports whether s is a valid kebab-case identifier.
func IsKebabCase(s string) bool {
	return kebabRe.MatchString(s)
}

// ValidateRepoName returns an error if name is not a valid grove project/repo
// name. Valid names are kebab-case: lowercase letters, digits, and hyphens,
// starting with a letter or digit. No underscores, no uppercase.
func ValidateRepoName(name string) error {
	if !IsKebabCase(name) {
		return fmt.Errorf(
			"name %q must be kebab-case (lowercase letters, digits, hyphens only; no underscores or uppercase)",
			name,
		)
	}
	return nil
}

// ValidateBranchName returns an error if branch does not conform to grove's
// branch naming convention.
//
// Valid forms:
//   - base branches: main, master, develop, staging, prod
//   - conventional branches: <type>/<kebab-slug>
//     valid types: feat, fix, chore, refactor, docs, test, hotfix
//     e.g. feat/user-auth, fix/nil-panic, chore/update-deps
func ValidateBranchName(branch string) error {
	if baseBranches[branch] {
		return nil
	}
	parts := strings.SplitN(branch, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf(
			"branch %q does not follow convention: use <type>/<description>, e.g. feat/user-auth "+
				"(valid types: feat, fix, chore, refactor, docs, test, hotfix)",
			branch,
		)
	}
	branchType, slug := parts[0], parts[1]
	if !branchTypes[branchType] {
		return fmt.Errorf(
			"branch %q has unknown type %q: valid types are feat, fix, chore, refactor, docs, test, hotfix",
			branch, branchType,
		)
	}
	if !IsKebabCase(slug) {
		return fmt.Errorf(
			"branch %q: description %q must be lowercase kebab-case (letters, digits, hyphens only, no underscores)",
			branch, slug,
		)
	}
	return nil
}
