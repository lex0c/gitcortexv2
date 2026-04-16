package git

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var DebugLogging bool

func debugf(format string, args ...interface{}) {
	if !DebugLogging {
		return
	}
	log.Printf("debug: "+format, args...)
}

type CommitMeta struct {
	SHA            string
	Tree           string
	Parents        []string
	AuthorName     string
	AuthorEmail    string
	AuthorDate     string
	CommitterName  string
	CommitterEmail string
	CommitterDate  string
	Message        string
}

// CommitAtOffset resolves the SHA at a given chronological offset in the history.
// Used for backward-compatible resume from offset-based state files.
func CommitAtOffset(ctx context.Context, repo, branch string, offset int, commandTimeout time.Duration, firstParent bool) (string, error) {
	if offset < 0 {
		return "", fmt.Errorf("offset must be non-negative")
	}

	branchRef := branch
	if branchRef == "" {
		branchRef = "HEAD"
	}

	countArgs := []string{"-C", repo, "rev-list"}
	if firstParent {
		countArgs = append(countArgs, "--first-parent")
	}
	countArgs = append(countArgs, "--count", branchRef)

	countCtx, cancelCount := context.WithTimeout(ctx, commandTimeout)
	countCmd := exec.CommandContext(countCtx, "git", countArgs...)
	countOut, err := countCmd.Output()
	cancelCount()
	if err != nil {
		if errors.Is(countCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("rev-list --count timed out: %w", countCtx.Err())
		}
		return "", fmt.Errorf("rev-list --count: %w", err)
	}

	totalCommits, err := strconv.Atoi(strings.TrimSpace(string(countOut)))
	if err != nil {
		return "", fmt.Errorf("parse rev-list --count output: %w", err)
	}
	if offset >= totalCommits {
		return "", nil
	}

	skipFromTip := totalCommits - offset - 1
	if skipFromTip < 0 {
		skipFromTip = 0
	}

	args := []string{"-C", repo, "rev-list"}
	if firstParent {
		args = append(args, "--first-parent")
	}
	if skipFromTip > 0 {
		args = append(args, fmt.Sprintf("--skip=%d", skipFromTip))
	}
	args = append(args, "--max-count=1", branchRef)

	cmdCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	out, err := exec.CommandContext(cmdCtx, "git", args...).CombinedOutput()
	if err != nil {
		if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("rev-list offset lookup timed out: %w", cmdCtx.Err())
		}
		return "", fmt.Errorf("rev-list offset lookup: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return strings.TrimSpace(string(out)), nil
}

func DetectDefaultBranch(repo string) string {
	out, err := exec.Command("git", "-C", repo, "symbolic-ref", "refs/remotes/origin/HEAD").Output()
	if err == nil {
		ref := strings.TrimSpace(string(out))
		if parts := strings.SplitN(ref, "refs/remotes/origin/", 2); len(parts) == 2 {
			return parts[1]
		}
	}

	for _, branch := range []string{"main", "master"} {
		if err := exec.Command("git", "-C", repo, "rev-parse", "--verify", "--quiet", branch).Run(); err == nil {
			return branch
		}
	}

	return "HEAD"
}

func IsValidSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
