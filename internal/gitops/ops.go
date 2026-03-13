package gitops

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain/agent"
)

// MergeStrategy determines what happens to a branch after successful completion.
type MergeStrategy string

const (
	// StrategyPR creates a pull request targeting the default branch.
	StrategyPR MergeStrategy = "pr"
	// StrategyMerge performs a fast-forward or merge commit.
	StrategyMerge MergeStrategy = "merge"
	// StrategyManual leaves the branch for human review.
	StrategyManual MergeStrategy = "manual"
)

// Ops provides git operations for repository management within volumes.
type Ops struct {
	logger  *slog.Logger
	gitPath string
}

// NewOps creates a git operations handler.
func NewOps(logger *slog.Logger) (*Ops, error) {
	if logger == nil {
		logger = slog.Default()
	}
	path, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("gitops: git CLI not found in PATH: %w", err)
	}
	return &Ops{
		logger:  logger.With("component", "gitops"),
		gitPath: path,
	}, nil
}

// Clone clones a repository into the specified directory.
func (g *Ops) Clone(ctx context.Context, repoURL, targetDir string) error {
	_, err := g.git(ctx, "", "clone", repoURL, targetDir)
	if err != nil {
		return fmt.Errorf("gitops: clone %s: %w", repoURL, err)
	}
	g.logger.InfoContext(ctx, "repository cloned", "repo_url", repoURL, "target", targetDir)
	return nil
}

// Fetch fetches latest changes from the remote in the given directory.
func (g *Ops) Fetch(ctx context.Context, repoDir string) error {
	_, err := g.git(ctx, repoDir, "fetch", "--all", "--prune")
	if err != nil {
		return fmt.Errorf("gitops: fetch in %s: %w", repoDir, err)
	}
	g.logger.InfoContext(ctx, "fetched latest", "repo_dir", repoDir)
	return nil
}

// CreateBranch creates a new branch for an agent session from the default branch.
func (g *Ops) CreateBranch(ctx context.Context, repoDir string, sessionID uuid.UUID, baseBranch string) (string, error) {
	branchName := agent.NewBranchName(sessionID)

	// Ensure we're on the base branch and up to date
	if _, err := g.git(ctx, repoDir, "checkout", baseBranch); err != nil {
		return "", fmt.Errorf("gitops: checkout %s: %w", baseBranch, err)
	}
	if _, err := g.git(ctx, repoDir, "pull", "--ff-only", "origin", baseBranch); err != nil {
		// Non-fatal: the remote may not have changes, or we may be detached
		g.logger.WarnContext(ctx, "pull failed (non-fatal)", "error", err, "branch", baseBranch)
	}

	// Create and checkout the session branch
	if _, err := g.git(ctx, repoDir, "checkout", "-b", branchName); err != nil {
		return "", fmt.Errorf("gitops: create branch %s: %w", branchName, err)
	}

	g.logger.InfoContext(ctx, "branch created",
		"branch", branchName,
		"base", baseBranch,
		"session_id", sessionID,
	)
	return branchName, nil
}

// CheckoutBranch checks out an existing branch.
func (g *Ops) CheckoutBranch(ctx context.Context, repoDir, branchName string) error {
	_, err := g.git(ctx, repoDir, "checkout", branchName)
	if err != nil {
		return fmt.Errorf("gitops: checkout %s: %w", branchName, err)
	}
	return nil
}

// DeleteBranch deletes a local branch.
func (g *Ops) DeleteBranch(ctx context.Context, repoDir, branchName string) error {
	_, err := g.git(ctx, repoDir, "branch", "-D", branchName)
	if err != nil {
		return fmt.Errorf("gitops: delete branch %s: %w", branchName, err)
	}
	g.logger.InfoContext(ctx, "branch deleted", "branch", branchName)
	return nil
}

// CurrentBranch returns the name of the currently checked-out branch.
func (g *Ops) CurrentBranch(ctx context.Context, repoDir string) (string, error) {
	out, err := g.git(ctx, repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("gitops: current branch: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// HasChanges checks if the working tree has uncommitted changes.
func (g *Ops) HasChanges(ctx context.Context, repoDir string) (bool, error) {
	out, err := g.git(ctx, repoDir, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("gitops: status: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}

// PushBranch pushes a branch to the remote.
func (g *Ops) PushBranch(ctx context.Context, repoDir, branchName string) error {
	_, err := g.git(ctx, repoDir, "push", "-u", "origin", branchName)
	if err != nil {
		return fmt.Errorf("gitops: push %s: %w", branchName, err)
	}
	g.logger.InfoContext(ctx, "branch pushed", "branch", branchName)
	return nil
}

// git runs a git command in the specified directory.
func (g *Ops) git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, g.gitPath, args...) //nolint:gosec // args are constructed internally, not from user input
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
