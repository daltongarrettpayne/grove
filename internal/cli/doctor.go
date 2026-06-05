package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/daltongarrettpayne/grove/internal/git"
	"github.com/daltongarrettpayne/grove/internal/model"
	"github.com/daltongarrettpayne/grove/internal/tmux"
)

// finding is a single violation or drift item discovered by doctor.
type finding struct {
	Check  string `json:"check"`
	Path   string `json:"path,omitempty"`
	Detail string `json:"detail"`
}

var (
	doctorJSONFlag  bool
	doctorCheckFlag string
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Audit vault and code directories for convention violations and drift",
	Long: `Scan the vault and code root for convention violations and drift, then report findings.

Preconditions:
  - GROVE_HOME_ROOT must be set or default to ~ (doctor checks whether it has the
    expected tier directories).
  - GROVE_CODE_ROOT is scanned two levels deep: CodeRoot/container/repo-or-worktree.
  - orphaned-session check runs only when inside a tmux session (TMUX must be set).

Check categories:
  vault-present      HomeRoot has expected tier directories (01-Projects, 02-Areas)
  archived-context   archived vault contexts that still have a live tmux session
  repo-naming        repo directories that are not kebab-case
  branch-naming      current branches that do not follow the type/description convention
  worktree-naming    linked worktrees whose directory does not match <repo>-<slug>
  stale-worktree     linked worktrees whose branch no longer exists in the main repo
  detached-head      working trees with a detached HEAD (grove cannot identify as a lane)
  orphaned-session   tmux sessions with no matching vault context (only when in tmux)

Default output (human-readable): findings grouped by check category. If all checks
pass, prints "no issues found".

Exit codes: 0 = clean, 1 = one or more violations found, 2+ = runtime failure.`,
	Example: `  # Run all checks (human-readable output):
  grove doctor

  # Emit findings as JSON (suitable for scripting):
  grove doctor --json

  # Run only the stale-worktree check:
  grove doctor --check stale-worktree

  # Use in CI — non-zero exit if any violations found:
  grove doctor || exit 1`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoctor(doctorJSONFlag, doctorCheckFlag)
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSONFlag, "json", false, "emit findings as a JSON array")
	doctorCmd.Flags().StringVar(&doctorCheckFlag, "check", "", "run only this check category")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(asJSON bool, onlyCheck string) error {
	var all []finding
	var runErr error

	do := func(name string, fn func() ([]finding, error)) {
		if onlyCheck != "" && onlyCheck != name {
			return
		}
		ff, err := fn()
		if err != nil {
			if runErr == nil {
				runErr = err
			}
			all = append(all, finding{Check: name, Detail: "error: " + err.Error()})
			return
		}
		all = append(all, ff...)
	}

	do("vault-present", doctorVaultPresent)
	do("archived-context", doctorArchivedContexts)
	do("repo-naming", doctorRepoNaming)
	do("branch-naming", doctorBranchNaming)
	do("worktree-naming", doctorWorktreeNaming)
	do("stale-worktree", doctorStaleWorktrees)
	do("detached-head", doctorDetachedHeads)
	if os.Getenv("TMUX") != "" {
		do("orphaned-session", doctorOrphanedSessions)
	}

	if asJSON {
		out, _ := json.MarshalIndent(all, "", "  ")
		fmt.Println(string(out))
	} else {
		printFindings(all)
	}

	if runErr != nil {
		return runErr
	}
	if len(all) > 0 {
		os.Exit(1)
	}
	return nil
}

func printFindings(findings []finding) {
	if len(findings) == 0 {
		fmt.Println("no issues found")
		return
	}
	current := ""
	for _, f := range findings {
		if f.Check != current {
			if current != "" {
				fmt.Println()
			}
			fmt.Println(f.Check)
			current = f.Check
		}
		if f.Path != "" {
			fmt.Printf("  %-48s  %s\n", f.Path, f.Detail)
		} else {
			fmt.Printf("  %s\n", f.Detail)
		}
	}
}

func doctorVaultPresent() ([]finding, error) {
	for _, tier := range []string{"01-Projects", "02-Areas"} {
		if _, err := os.Stat(filepath.Join(cfg.HomeRoot, tier)); err == nil {
			return nil, nil
		}
	}
	return []finding{{
		Check:  "vault-present",
		Path:   cfg.HomeRoot,
		Detail: "no vault tier directories found (expected 01-Projects or 02-Areas)",
	}}, nil
}

func doctorArchivedContexts() ([]finding, error) {
	archiveDir := filepath.Join(cfg.HomeRoot, "04-Archive")
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", archiveDir, err)
	}

	var ff []finding
	for _, e := range entries {
		if !e.IsDir() || isHiddenName(e.Name()) {
			continue
		}
		exists, err := tmux.HasSession(e.Name())
		if err != nil || !exists {
			continue
		}
		ff = append(ff, finding{
			Check:  "archived-context",
			Path:   filepath.Join(archiveDir, e.Name()),
			Detail: fmt.Sprintf("context %q is archived but has a live tmux session", e.Name()),
		})
	}
	return ff, nil
}

// scanAllRepoDirs returns all git working-tree directories found two levels
// deep inside cfg.CodeRoot (CodeRoot / container / repo-or-worktree).
// Unlike scanner.ScanSourceSet this does NOT skip detached HEAD.
func scanAllRepoDirs() ([]string, error) {
	containers, err := os.ReadDir(cfg.CodeRoot)
	if err != nil {
		return nil, fmt.Errorf("reading code root %s: %w", cfg.CodeRoot, err)
	}

	var dirs []string
	for _, ce := range containers {
		if !ce.IsDir() || isHiddenName(ce.Name()) {
			continue
		}
		container := filepath.Join(cfg.CodeRoot, ce.Name())
		repos, err := os.ReadDir(container)
		if err != nil {
			continue
		}
		for _, re := range repos {
			if !re.IsDir() || isHiddenName(re.Name()) {
				continue
			}
			dir := filepath.Join(container, re.Name())
			if git.IsRepo(dir) {
				dirs = append(dirs, dir)
			}
		}
	}
	return dirs, nil
}

func doctorRepoNaming() ([]finding, error) {
	dirs, err := scanAllRepoDirs()
	if err != nil {
		return nil, err
	}

	var ff []finding
	for _, dir := range dirs {
		name := filepath.Base(dir)
		if !model.IsKebabCase(name) {
			ff = append(ff, finding{
				Check:  "repo-naming",
				Path:   dir,
				Detail: fmt.Sprintf("directory %q is not kebab-case", name),
			})
		}
	}
	return ff, nil
}

func doctorBranchNaming() ([]finding, error) {
	dirs, err := scanAllRepoDirs()
	if err != nil {
		return nil, err
	}

	var ff []finding
	for _, dir := range dirs {
		branch, err := git.CurrentBranch(dir)
		if err != nil || branch == "" {
			continue // detached HEAD handled by detached-head check
		}
		if err := model.ValidateBranchName(branch); err != nil {
			ff = append(ff, finding{
				Check:  "branch-naming",
				Path:   dir,
				Detail: err.Error(),
			})
		}
	}
	return ff, nil
}

func doctorWorktreeNaming() ([]finding, error) {
	dirs, err := scanAllRepoDirs()
	if err != nil {
		return nil, err
	}

	var ff []finding
	for _, dir := range dirs {
		isWT, err := git.IsWorktree(dir)
		if err != nil || !isWT {
			continue
		}
		branch, err := git.CurrentBranch(dir)
		if err != nil || branch == "" {
			continue
		}
		repoName, err := git.RepoName(dir)
		if err != nil {
			continue
		}
		expected := repoName + "-" + slugifyBranch(branch)
		actual := filepath.Base(dir)
		if actual != expected {
			ff = append(ff, finding{
				Check:  "worktree-naming",
				Path:   dir,
				Detail: fmt.Sprintf("directory %q should be %q (repo=%s branch=%s)", actual, expected, repoName, branch),
			})
		}
	}
	return ff, nil
}

func doctorStaleWorktrees() ([]finding, error) {
	dirs, err := scanAllRepoDirs()
	if err != nil {
		return nil, err
	}

	var ff []finding
	for _, dir := range dirs {
		isWT, err := git.IsWorktree(dir)
		if err != nil || !isWT {
			continue
		}
		branch, err := git.CurrentBranch(dir)
		if err != nil || branch == "" {
			continue // detached HEAD handled separately
		}
		// --git-common-dir gives the absolute path to the main repo's .git dir.
		commonDir, err := git.CommonDir(dir)
		if err != nil {
			continue
		}
		mainRepoDir := filepath.Dir(commonDir)
		exists, err := git.BranchExists(mainRepoDir, branch)
		if err != nil || exists {
			continue
		}
		ff = append(ff, finding{
			Check:  "stale-worktree",
			Path:   dir,
			Detail: fmt.Sprintf("branch %q no longer exists in main repo", branch),
		})
	}
	return ff, nil
}

func doctorDetachedHeads() ([]finding, error) {
	dirs, err := scanAllRepoDirs()
	if err != nil {
		return nil, err
	}

	var ff []finding
	for _, dir := range dirs {
		branch, err := git.CurrentBranch(dir)
		if err != nil {
			continue
		}
		if branch == "" {
			ff = append(ff, finding{
				Check:  "detached-head",
				Path:   dir,
				Detail: "HEAD is detached; grove cannot identify this as a lane",
			})
		}
	}
	return ff, nil
}

func doctorOrphanedSessions() ([]finding, error) {
	sessions, err := tmux.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}

	validContexts := make(map[string]bool)
	for _, tier := range []string{"01-Projects", "02-Areas"} {
		tierDir := filepath.Join(cfg.HomeRoot, tier)
		entries, err := os.ReadDir(tierDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() && !isHiddenName(e.Name()) {
				validContexts[e.Name()] = true
			}
		}
	}

	var ff []finding
	for _, session := range sessions {
		if !validContexts[session] {
			ff = append(ff, finding{
				Check:  "orphaned-session",
				Path:   session,
				Detail: fmt.Sprintf("tmux session %q has no matching context in vault tiers", session),
			})
		}
	}
	return ff, nil
}
