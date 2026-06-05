package model_test

import (
	"testing"

	"github.com/daltongarrettpayne/grove/internal/model"
)

func TestIsKebabCase(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"grove", true},
		{"grove-api", true},
		{"my-repo-123", true},
		{"a", true},
		{"123", true},
		{"Grove", false},   // uppercase
		{"my_repo", false}, // underscore
		{"my repo", false}, // space
		{"my-Repo", false}, // mixed case
		{"-starts-with-hyphen", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := model.IsKebabCase(tt.input)
			if got != tt.want {
				t.Errorf("IsKebabCase(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateBranchName(t *testing.T) {
	valid := []string{
		"main",
		"master",
		"develop",
		"staging",
		"prod",
		"feat/user-auth",
		"fix/nil-panic",
		"chore/update-deps",
		"refactor/extract-handler",
		"docs/api-reference",
		"test/add-unit-tests",
		"hotfix/login-crash",
		"feat/x",
	}

	for _, b := range valid {
		t.Run("valid/"+b, func(t *testing.T) {
			if err := model.ValidateBranchName(b); err != nil {
				t.Errorf("ValidateBranchName(%q) returned unexpected error: %v", b, err)
			}
		})
	}

	invalid := []struct {
		branch string
		reason string
	}{
		{"featureAuth", "no slash"},
		{"feature/auth", "unknown type"},
		{"feat/User_Auth", "uppercase + underscore in slug"},
		{"feat/_bad", "slug starts with hyphen-like char"},
		{"feat/", "empty slug"},
		{"fix", "no slash, not a base branch"},
		{"FEAT/thing", "uppercase type"},
	}

	for _, tt := range invalid {
		t.Run("invalid/"+tt.branch, func(t *testing.T) {
			if err := model.ValidateBranchName(tt.branch); err == nil {
				t.Errorf("ValidateBranchName(%q) expected error (%s) but got nil", tt.branch, tt.reason)
			}
		})
	}
}
