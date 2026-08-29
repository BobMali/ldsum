// Package githooks_test exercises the repository's shell hooks: commit-msg,
// which git runs via core.hooksPath, and the Claude Code PreToolUse guard.
package githooks_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Relative to the repository root, unlike commit-msg which sits beside this file.
const guardHook = ".claude/hooks/commit-guard.sh"

// Tests run in the package directory, so the repository root is one level up.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	t.Fatalf("run hook: %v", err)
	return 0
}

func TestCommitMsg(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    int
	}{
		{"conventional subject", "feat(cmd): add verify subcommand", 0},
		{"scope omitted", "docs: explain the package layout", 0},
		{"breaking change", "feat(cmd)!: rename verify to check", 0},
		{"description at the limit", "feat: " + strings.Repeat("a", 66), 0},
		{"merge is left to git", "Merge branch 'main' into feature", 0},
		{"fixup is left to git", "fixup! feat(cmd): add verify subcommand", 0},
		{"body and comments ignored", "# please enter a message\nfeat(hash): stream input through a buffer\n\nAvoids loading whole files into memory.", 0},
		{"missing type", "update code", 1},
		{"capitalised with trailing period", "feat: Added verify command.", 1},
		{"unknown scope", "feat(parser): add verify subcommand", 1},
		{"empty message", "# only a comment\n\n", 1},
		{"description too long", "feat: " + strings.Repeat("a", 67), 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
			if err := os.WriteFile(path, []byte(tt.message+"\n"), 0o644); err != nil {
				t.Fatalf("write message: %v", err)
			}

			cmd := exec.Command("bash", "commit-msg", path)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if got := exitCode(t, cmd.Run()); got != tt.want {
				t.Errorf("exit = %d, want %d\nstderr: %s", got, tt.want, &stderr)
			}
		})
	}
}

func TestCommitGuard(t *testing.T) {
	root := repoRoot(t)
	hook := filepath.Join(root, guardHook)

	tests := []struct {
		name    string
		command string
		want    int
	}{
		{"not a git command", "ls -la", 0},
		{"conventional commit", `git commit -m "feat(cmd): add verify subcommand"`, 0},
		{"grep -n on the same line", `grep -n TODO main.go && git commit -m "feat(cmd): add x"`, 0},
		{"echo -n on the same line", `git commit -m "feat(cmd): add x" && echo -n done`, 0},
		{"two conventional commits", `git commit -m "feat(cmd): add x" && git commit -m "fix(run): drop y"`, 0},
		{"no message defers to commit-msg", "git commit", 0},
		{"merge is left to git", `git commit -m "Merge branch main into feature"`, 0},
		{"short bypass flag", `git commit -n -m "feat(cmd): add x"`, 2},
		{"long bypass flag", `git commit --no-verify -m "feat(cmd): add x"`, 2},
		{"bypass in the second commit", `git commit -m "feat(cmd): add x" && git commit -n -m "fix(run): drop y"`, 2},
		{"missing type", `git commit -m "update code"`, 2},
		{"single quoted subject", `git commit -m 'misc changes'`, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]any{
				"tool_name":  "Bash",
				"tool_input": map[string]string{"command": tt.command},
			})
			if err != nil {
				t.Fatalf("encode payload: %v", err)
			}

			cmd := exec.Command("bash", hook)
			cmd.Stdin = bytes.NewReader(payload)
			cmd.Env = append(os.Environ(), "CLAUDE_PROJECT_DIR="+root)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if got := exitCode(t, cmd.Run()); got != tt.want {
				t.Errorf("exit = %d, want %d\nstderr: %s", got, tt.want, &stderr)
			}
		})
	}
}

// Covers the wiring, not the script: the hook only runs when core.hooksPath is
// set, which a fresh clone does not do.
func TestCommitMsgUnderGit(t *testing.T) {
	hooksDir := filepath.Join(repoRoot(t), "githooks")
	dir := t.TempDir()

	git := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		// Ignore the developer's own git config so the test is hermetic.
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
		)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.name", "Test"},
		{"config", "user.email", "test@example.com"},
		{"config", "core.hooksPath", hooksDir},
	} {
		if out, err := git(args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if out, err := git("add", "a.txt"); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	if out, err := git("commit", "-m", "update code"); err == nil {
		t.Errorf("commit with an invalid subject succeeded:\n%s", out)
	}
	if out, err := git("commit", "-m", "feat(cmd): add verify subcommand"); err != nil {
		t.Fatalf("commit with a valid subject failed: %v\n%s", err, out)
	}
}
