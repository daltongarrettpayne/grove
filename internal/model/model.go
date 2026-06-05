// Package model defines the core domain types for grove.
// The vocabulary here matches the design doc: context → lanes → views.
package model

import "strings"

// SourceSet is a list of container directories scanned for working trees.
// One container = a single-project context; multiple = a cockpit.
type SourceSet []string

// LaneKind distinguishes how a lane's working tree was obtained.
type LaneKind int

const (
	LaneHome     LaneKind = iota // lane 0: the context's home directory
	LaneClone                    // an owned git clone
	LaneWorktree                 // a linked git worktree
	LanePointer                  // a pointer lane with no local checkout
)

// Lane is a tmux window. Lane 0 is always the home lane (no repo or branch).
type Lane struct {
	Kind   LaneKind
	Repo   string // repository name — same across all worktrees of a repo
	Branch string // branch name; empty for the home lane
	Dir    string // absolute path to the working tree on disk
	IsHome bool
}

// DisplayRow returns the picker-protocol row for this lane.
//
// Home lane:    "home"
// Regular lane: "<repo><padding>  ·  <branch>"
//
// maxRepoLen is the longest Repo across all non-home lanes in the context,
// used to left-pad repos so the · separators align in a column.
func (l Lane) DisplayRow(maxRepoLen int) string {
	if l.IsHome {
		return "home"
	}
	// strings.Repeat with max(0,…) guards against negative padding when
	// maxRepoLen is somehow shorter than this lane's repo name.
	padding := strings.Repeat(" ", max(0, maxRepoLen-len(l.Repo)))
	return l.Repo + padding + "  ·  " + l.Branch
}

// Context is a tmux session: a name, a home directory, and an ordered list
// of lanes. Lane 0 is always the home lane.
type Context struct {
	Name      string
	HomeDir   string
	SourceSet SourceSet
	Lanes     []Lane
}

// MaxRepoLen returns the length of the longest Repo name across all non-home
// lanes. Pass this to Lane.DisplayRow so the picker rows align correctly.
func (c *Context) MaxRepoLen() int {
	n := 0
	for _, l := range c.Lanes {
		if !l.IsHome && len(l.Repo) > n {
			n = len(l.Repo)
		}
	}
	return n
}
