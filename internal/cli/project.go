package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/daltongarrettpayne/grove/internal/model"
	"github.com/daltongarrettpayne/grove/internal/tmux"
	"github.com/daltongarrettpayne/grove/internal/vault"
)

// ── flags ──────────────────────────────────────────────────────────────────

var (
	projectInitCode    bool
	projectInitNoGit   bool
	projectInitClone   string
	projectListJSON    bool
	projectArchiveKill bool
)

// ── command tree ───────────────────────────────────────────────────────────

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage grove projects",
}

var projectInitCmd = &cobra.Command{
	Use:   "init <name>",
	Short: "Create a new project in the vault",
	Args:  cobra.ExactArgs(1),
	Long: `Creates a new project in the vault's 01-Projects tier.

Preconditions:
  GROVE_HOME_ROOT must point to a valid vault directory with a 01-Projects/ subdirectory.
  If --code is set, GROVE_CODE_ROOT must exist.

What it creates:
  <GROVE_HOME_ROOT>/01-Projects/<name>/           vault directory
  <GROVE_HOME_ROOT>/01-Projects/<name>/context.md project metadata file
  <GROVE_CODE_ROOT>/<name>/                       code directory (--code only)
  <GROVE_HOME_ROOT>/01-Projects/<name>/code       symlink to code dir (--code only)

If --clone <url> is provided, the code directory is populated by cloning the
given URL (clone output is streamed to your terminal). If --no-git is set
instead, no git repository is initialised in the code directory.

The name must be kebab-case (lowercase letters, digits, hyphens; no underscores).

Exits 0 on success. Exits non-zero if the name is invalid, the directory
already exists, --clone fails, or a filesystem operation fails.`,
	Example: `  grove project init my-project
  grove project init my-project --code
  grove project init my-project --code --clone https://github.com/org/repo
  grove project init my-project --code --no-git`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProjectInit(args[0], projectInitCode, projectInitNoGit, projectInitClone)
	},
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projects in the vault",
	Long: `Scans the vault's 01-Projects tier and prints every project directory.

For each project the following is reported:
  name      — directory name
  code      — whether a matching directory exists under GROVE_CODE_ROOT
  session   — whether a live tmux session with this name exists
  vault     — absolute path to the vault directory

Human output (default): an aligned table with those four columns.
JSON output (--json):    a JSON array where each object has name, code, session, vault_path.

Hidden directories (names starting with ".") are skipped.

Exits 0. If tmux is not running the session column shows "no" for all rows.`,
	Example: `  grove project list
  grove project list --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProjectList(projectListJSON)
	},
}

var projectArchiveCmd = &cobra.Command{
	Use:   "archive <name>",
	Short: "Move a project from 01-Projects to 04-Archive/01-Projects",
	Args:  cobra.ExactArgs(1),
	Long: `Moves a project vault directory from the active 01-Projects tier into
04-Archive/01-Projects/, preserving the directory name.

Preconditions:
  <GROVE_HOME_ROOT>/01-Projects/<name>/ must exist.
  <GROVE_HOME_ROOT>/04-Archive/01-Projects/<name>/ must NOT already exist
  (to prevent silent clobbering of an existing archive entry).

The 04-Archive/01-Projects/ directory is created if it does not exist.

Only the vault directory is moved; any matching code directory under
GROVE_CODE_ROOT is left in place.

If --kill-session is set and a live tmux session named <name> exists, it is
killed before the move.

Exits 0 on success. Exits non-zero if the source is not found, the destination
already exists, the session kill fails, or the move fails.`,
	Example: `  grove project archive my-project
  grove project archive my-project --kill-session`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProjectArchive(args[0], projectArchiveKill)
	},
}

func init() {
	// init flags
	projectInitCmd.Flags().BoolVar(&projectInitCode, "code", false, "create a matching code directory under GROVE_CODE_ROOT")
	projectInitCmd.Flags().BoolVar(&projectInitNoGit, "no-git", false, "skip git init when creating the code directory")
	projectInitCmd.Flags().StringVar(&projectInitClone, "clone", "", "clone this URL into the code directory instead of git init")

	// list flags
	projectListCmd.Flags().BoolVar(&projectListJSON, "json", false, "emit output as a JSON array")

	// archive flags
	projectArchiveCmd.Flags().BoolVar(&projectArchiveKill, "kill-session", false, "kill the tmux session named <name> if it is running")

	// wire into command tree
	projectCmd.AddCommand(projectInitCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectArchiveCmd)
	rootCmd.AddCommand(projectCmd)
}

// ── runners ────────────────────────────────────────────────────────────────

func runProjectInit(name string, code, noGit bool, cloneURL string) error {
	// Step 1: validate name.
	if err := model.ValidateRepoName(name); err != nil {
		return err
	}

	vaultDir := filepath.Join(cfg.HomeRoot, "01-Projects", name)

	// Step 2: fail if vault dir already exists.
	if _, err := os.Stat(vaultDir); err == nil {
		return fmt.Errorf("vault directory already exists: %s", vaultDir)
	}

	// Step 3: create the vault directory.
	slog.Info("creating vault directory", "path", vaultDir)
	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		return fmt.Errorf("creating vault directory %s: %w", vaultDir, err)
	}

	// Step 4: write context.md.
	today := time.Now().Format("2006-01-02")
	var sourceSet []string
	if code {
		codeDir := filepath.Join(cfg.CodeRoot, name)
		sourceSet = []string{codeDir}
	}
	if err := vault.WriteContextMD(vaultDir, name, today, code, sourceSet); err != nil {
		return fmt.Errorf("writing context.md: %w", err)
	}
	fmt.Printf("created  %s\n", filepath.Join(vaultDir, "context.md"))
	fmt.Printf("created  %s\n", vaultDir)

	// Step 5: code directory (optional).
	if code {
		codeDir := filepath.Join(cfg.CodeRoot, name)

		// Fail if code dir already exists.
		if _, err := os.Stat(codeDir); err == nil {
			return fmt.Errorf("code directory already exists: %s", codeDir)
		}

		slog.Info("creating code directory", "path", codeDir)
		if err := os.MkdirAll(codeDir, 0755); err != nil {
			return fmt.Errorf("creating code directory %s: %w", codeDir, err)
		}

		if cloneURL != "" {
			// Clone — stream output so the user sees progress.
			slog.Info("cloning repository", "url", cloneURL, "dest", codeDir)
			cloneCmd := exec.Command("git", "clone", cloneURL, codeDir)
			cloneCmd.Stdout = os.Stdout
			cloneCmd.Stderr = os.Stderr
			if err := cloneCmd.Run(); err != nil {
				return fmt.Errorf("cloning %s into %s: %w", cloneURL, codeDir, err)
			}
			fmt.Printf("cloned   %s  →  %s\n", cloneURL, codeDir)
		} else if !noGit {
			slog.Info("initialising git repository", "path", codeDir)
			initCmd := exec.Command("git", "init", "-b", "main", codeDir)
			if err := initCmd.Run(); err != nil {
				return fmt.Errorf("git init %s: %w", codeDir, err)
			}
			fmt.Printf("git init %s\n", codeDir)
		} else {
			fmt.Printf("created  %s\n", codeDir)
		}

		// Symlink vault/code → codeDir.
		symlink := filepath.Join(vaultDir, "code")
		slog.Info("creating symlink", "link", symlink, "target", codeDir)
		if err := os.Symlink(codeDir, symlink); err != nil {
			return fmt.Errorf("creating symlink %s → %s: %w", symlink, codeDir, err)
		}
		fmt.Printf("symlink  %s  →  %s\n", symlink, codeDir)
	}

	return nil
}

// projectEntry holds the data collected for a single project during list.
type projectEntry struct {
	Name      string `json:"name"`
	Code      bool   `json:"code"`
	Session   bool   `json:"session"`
	VaultPath string `json:"vault_path"`
}

func runProjectList(asJSON bool) error {
	tierDir := filepath.Join(cfg.HomeRoot, "01-Projects")
	entries, err := os.ReadDir(tierDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No 01-Projects dir at all — return empty gracefully.
			if asJSON {
				fmt.Println("[]")
			}
			return nil
		}
		return fmt.Errorf("reading vault tier %s: %w", tierDir, err)
	}

	var projects []projectEntry
	maxNameLen := len("name") // column header width minimum
	for _, e := range entries {
		if !e.IsDir() || isHiddenName(e.Name()) {
			continue
		}
		name := e.Name()

		// Check for matching code directory.
		codeDir := filepath.Join(cfg.CodeRoot, name)
		_, statErr := os.Stat(codeDir)
		hasCode := statErr == nil

		// Check for live tmux session; if tmux isn't running treat as false.
		hasSession, sessErr := tmux.HasSession(name)
		if sessErr != nil {
			// tmux not running or other non-fatal error — treat as no session.
			slog.Debug("tmux.HasSession returned error, assuming no session", "name", name, "err", sessErr)
			hasSession = false
		}

		projects = append(projects, projectEntry{
			Name:      name,
			Code:      hasCode,
			Session:   hasSession,
			VaultPath: filepath.Join(tierDir, name),
		})

		if len(name) > maxNameLen {
			maxNameLen = len(name)
		}
	}

	if asJSON {
		out, err := json.MarshalIndent(projects, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling project list: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}

	// Human-readable aligned table.
	// Columns: name, code, session, vault
	colName := maxNameLen
	colBool := 7 // length of "session" header

	fmt.Printf("%-*s  %-*s  %-*s  %s\n", colName, "name", colBool, "code", colBool, "session", "vault")
	fmt.Printf("%-*s  %-*s  %-*s  %s\n",
		colName, dashes(colName),
		colBool, dashes(colBool),
		colBool, dashes(colBool),
		dashes(len("vault")),
	)
	for _, p := range projects {
		fmt.Printf("%-*s  %-*s  %-*s  %s\n",
			colName, p.Name,
			colBool, boolStr(p.Code),
			colBool, boolStr(p.Session),
			p.VaultPath,
		)
	}
	return nil
}

func runProjectArchive(name string, killSession bool) error {
	src := filepath.Join(cfg.HomeRoot, "01-Projects", name)

	// Step 1: verify source exists.
	if _, err := os.Stat(src); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("project %q not found at %s", name, src)
		}
		return fmt.Errorf("stat %s: %w", src, err)
	}

	archiveTierDir := filepath.Join(cfg.HomeRoot, "04-Archive", "01-Projects")
	dest := filepath.Join(archiveTierDir, name)

	// Step 2: ensure 04-Archive/01-Projects/ exists.
	if err := os.MkdirAll(archiveTierDir, 0755); err != nil {
		return fmt.Errorf("creating archive tier %s: %w", archiveTierDir, err)
	}

	// Step 3: fail if dest already exists (don't clobber).
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("archive destination already exists: %s", dest)
	}

	// Step 4: optionally kill the tmux session.
	if killSession {
		exists, err := tmux.HasSession(name)
		if err != nil {
			slog.Debug("tmux.HasSession error when checking before kill", "name", name, "err", err)
		} else if exists {
			slog.Info("killing tmux session", "name", name)
			if err := tmux.KillSession(name); err != nil {
				return fmt.Errorf("killing session %q: %w", name, err)
			}
			fmt.Printf("killed   session %s\n", name)
		}
	}

	// Step 5: move the vault directory.
	slog.Info("archiving project", "src", src, "dest", dest)
	if err := os.Rename(src, dest); err != nil {
		return fmt.Errorf("moving %s → %s: %w", src, dest, err)
	}
	fmt.Printf("archived %s\n         → %s\n", src, dest)

	return nil
}

// ── helpers ────────────────────────────────────────────────────────────────

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func dashes(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = '-'
	}
	return string(out)
}
