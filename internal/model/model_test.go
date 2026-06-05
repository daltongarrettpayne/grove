package model_test

import (
	"encoding/json"
	"testing"

	"github.com/daltongarrettpayne/grove/internal/model"
)

func TestLaneKindString(t *testing.T) {
	tests := []struct {
		name string
		kind model.LaneKind
		want string
	}{
		{"home", model.LaneHome, "home"},
		{"clone", model.LaneClone, "clone"},
		{"worktree", model.LaneWorktree, "worktree"},
		{"pointer", model.LanePointer, "pointer"},
		{"unknown", model.LaneKind(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.kind.String()
			if got != tt.want {
				t.Errorf("LaneKind(%d).String() = %q, want %q", int(tt.kind), got, tt.want)
			}
		})
	}
}

func TestLaneKindMarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		kind model.LaneKind
		want string // expected JSON bytes as a string (including quotes)
	}{
		{"clone", model.LaneClone, `"clone"`},
		{"worktree", model.LaneWorktree, `"worktree"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.kind)
			if err != nil {
				t.Fatalf("MarshalJSON error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("json.Marshal(%v) = %s, want %s", tt.kind, got, tt.want)
			}
		})
	}
}

func TestLaneDisplayRow(t *testing.T) {
	tests := []struct {
		name       string
		lane       model.Lane
		maxRepoLen int
		want       string
	}{
		{
			name:       "home lane",
			lane:       model.Lane{IsHome: true},
			maxRepoLen: 10,
			want:       "home",
		},
		{
			name:       "exact fit no padding",
			lane:       model.Lane{Repo: "grove", Branch: "main"},
			maxRepoLen: 5,
			want:       "grove  ·  main",
		},
		{
			name:       "padding applied",
			lane:       model.Lane{Repo: "grove", Branch: "main"},
			maxRepoLen: 10,
			want:       "grove       ·  main",
		},
		{
			name:       "full width repo no padding",
			lane:       model.Lane{Repo: "repo-alpha", Branch: "feat/x"},
			maxRepoLen: 10,
			want:       "repo-alpha  ·  feat/x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.lane.DisplayRow(tt.maxRepoLen)
			if got != tt.want {
				t.Errorf("DisplayRow(%d) = %q, want %q", tt.maxRepoLen, got, tt.want)
			}
		})
	}
}

func TestParseWindowName(t *testing.T) {
	tests := []struct {
		input      string
		wantRepo   string
		wantBranch string
		wantIsHome bool
	}{
		{
			input:      "home",
			wantIsHome: true,
		},
		{
			input:      "grove  ·  main",
			wantRepo:   "grove",
			wantBranch: "main",
		},
		{
			// DisplayRow with padding: repo padded to 10, separator, branch
			input:      "grove       ·  feat/auth",
			wantRepo:   "grove",
			wantBranch: "feat/auth",
		},
		{
			// worktree new format: no padding
			input:      "kalashnikov.ai  ·  feat/user-auth",
			wantRepo:   "kalashnikov.ai",
			wantBranch: "feat/user-auth",
		},
		{
			// Unrecognised format: no separator
			input:      "my-custom-window",
			wantRepo:   "my-custom-window",
			wantBranch: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			repo, branch, isHome := model.ParseWindowName(tt.input)
			if repo != tt.wantRepo || branch != tt.wantBranch || isHome != tt.wantIsHome {
				t.Errorf("ParseWindowName(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.input, repo, branch, isHome,
					tt.wantRepo, tt.wantBranch, tt.wantIsHome)
			}
		})
	}
}

func TestContextMaxRepoLen(t *testing.T) {
	tests := []struct {
		name  string
		lanes []model.Lane
		want  int
	}{
		{
			name:  "empty context",
			lanes: nil,
			want:  0,
		},
		{
			name:  "home lane only",
			lanes: []model.Lane{{IsHome: true, Repo: ""}},
			want:  0,
		},
		{
			name: "multiple non-home lanes",
			lanes: []model.Lane{
				{Repo: "ab"},
				{Repo: "longer"},
			},
			want: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &model.Context{Lanes: tt.lanes}
			got := ctx.MaxRepoLen()
			if got != tt.want {
				t.Errorf("MaxRepoLen() = %d, want %d", got, tt.want)
			}
		})
	}
}
