// Package scanner finds git working trees inside container directories and
// converts them into model.Lane values for grove to work with.
package scanner

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/daltongarrettpayne/grove/internal/git"
	"github.com/daltongarrettpayne/grove/internal/model"
)

// ScanSourceSet scans every container directory in the source set and returns
// all lanes found, sorted by (repo, branch). The home lane is NOT included;
// callers prepend it as lane 0.
func ScanSourceSet(sources model.SourceSet) ([]model.Lane, error) {
	var all []model.Lane
	for _, container := range sources {
		lanes, err := scanContainer(container)
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", container, err)
		}
		all = append(all, lanes...)
	}

	// Sort deterministically: (repo, branch). Worktrees of the same repo
	// are clustered together because they share the same Repo value.
	sort.Slice(all, func(i, j int) bool {
		if all[i].Repo != all[j].Repo {
			return all[i].Repo < all[j].Repo
		}
		return all[i].Branch < all[j].Branch
	})
	return all, nil
}

// scanContainer finds every git working tree directly inside container (one
// level deep). Each qualifying subdirectory becomes a Lane.
func scanContainer(container string) ([]model.Lane, error) {
	entries, err := os.ReadDir(container)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", container, err)
	}

	var lanes []model.Lane
	for _, e := range entries {
		if !e.IsDir() || isHidden(e.Name()) {
			continue
		}
		dir := filepath.Join(container, e.Name())

		if !git.IsRepo(dir) {
			continue
		}

		branch, err := git.CurrentBranch(dir)
		if err != nil || branch == "" {
			// Detached HEAD — skip; grove requires a named branch for identity.
			slog.Debug("skipping repo: no branch name", "dir", dir)
			continue
		}

		repoName, err := git.RepoName(dir)
		if err != nil {
			repoName = e.Name()
		}

		kind := model.LaneClone
		if isWT, err := git.IsWorktree(dir); err == nil && isWT {
			kind = model.LaneWorktree
		}

		lanes = append(lanes, model.Lane{
			Kind:   kind,
			Repo:   repoName,
			Branch: branch,
			Dir:    dir,
		})
	}
	return lanes, nil
}

func isHidden(name string) bool {
	return len(name) > 0 && name[0] == '.'
}
