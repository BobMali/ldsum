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

const (
	testGuardHook     = ".claude/hooks/test-guard.sh"
	worktreeGuardHook = ".claude/hooks/worktree-guard.sh"
)

// asksPermission runs a PreToolUse guard and reports whether it asked. These
// guards signal by printing a decision on stdout and exiting 0, unlike
// commit-guard, which blocks with exit 2.
func asksPermission(t *testing.T, hook string, payload map[string]any) bool {
	t.Helper()
	root := repoRoot(t)

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, hook))
	cmd.Stdin = bytes.NewReader(encoded)
	cmd.Env = append(os.Environ(), "CLAUDE_PROJECT_DIR="+root)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("hook %s failed: %v\nstderr: %s", hook, err, stderr.String())
	}

	out := stdout.String()
	if out == "" {
		return false
	}
	if !strings.Contains(out, `"permissionDecision":"ask"`) {
		t.Fatalf("hook %s produced output that is not an ask decision: %q", hook, out)
	}
	return true
}

func bashPayload(command string) map[string]any {
	return map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]string{"command": command},
	}
}

// The test guard lets a shell read a test file and asks before anything that
// could rewrite one. Reading is judged by the command word, so a command it
// cannot positively identify as read-only asks by default.
func TestTestGuardOnBash(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		wantsAsk bool
	}{
		{"running the suite names no file", "go test ./...", false},
		{"reading a test file", "cat internal/hash/hash_test.go", false},
		{"grepping a test file", `grep -n "func Test" internal/hash/hash_test.go`, false},
		{"staging a test file", "git add internal/hash/hash_test.go", false},
		{"listing unformatted files", "gofmt -l internal/hash/hash_test.go", false},
		{"searching for test files", `find . -name "*_test.go"`, false},
		{"a non-test source file", "sed -i '' s/a/b/ cmd/root.go", false},
		{"editing in place", "sed -i '' s/a/b/ internal/hash/hash_test.go", true},
		{"redirecting over a test file", "echo x > internal/hash/hash_test.go", true},
		{"rewriting with gofmt", "gofmt -w internal/hash/hash_test.go", true},
		{"find running a command", `find . -name "*_test.go" -exec sed -i '' s/a/b/ {} +`, true},
		{"find deleting", `find . -name "*_test.go" -delete`, true},
		{"an eval wrapper", `eval "sed -i '' s/a/b/ internal/hash/hash_test.go"`, true},
		{"discarding a test file", "git checkout -- internal/hash/hash_test.go", true},
		{"an interpreter", `python3 -c "open('internal/hash/hash_test.go','w')"`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := asksPermission(t, testGuardHook, bashPayload(tt.command)); got != tt.wantsAsk {
				t.Errorf("asked = %v, want %v for %q", got, tt.wantsAsk, tt.command)
			}
		})
	}
}

// Creating a test file is free and so is adding to one; only a rewrite asks.
func TestTestGuardOnEdits(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "sample_test.go")
	const contents = "package sample\n\nfunc TestOne(t *testing.T) {}\n"
	if err := os.WriteFile(existing, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	absent := filepath.Join(t.TempDir(), "absent_test.go")

	edit := func(path, old, new string) map[string]any {
		return map[string]any{
			"tool_name": "Edit",
			"tool_input": map[string]string{
				"file_path": path, "old_string": old, "new_string": new,
			},
		}
	}
	write := func(path, content string) map[string]any {
		return map[string]any{
			"tool_name":  "Write",
			"tool_input": map[string]string{"file_path": path, "content": content},
		}
	}

	tests := []struct {
		name     string
		payload  map[string]any
		wantsAsk bool
	}{
		{"a file that does not exist yet", write(absent, contents), false},
		{"appending a case", edit(existing, "}\n", "}\n\nfunc TestTwo(t *testing.T) {}\n"), false},
		{"inserting before an anchor", edit(existing, "func TestOne", "func TestTwo(t *testing.T) {}\n\nfunc TestOne"), false},
		{"writing the file plus more", write(existing, contents+"\nfunc TestTwo(t *testing.T) {}\n"), false},
		{"a non-test file", edit("cmd/root.go", "a", "b"), false},
		{"renaming a test", edit(existing, "func TestOne", "func TestUno"), true},
		{"deleting a block", edit(existing, "func TestOne(t *testing.T) {}", ""), true},
		{"widening an assertion", edit(existing, "want: 5", "want: 55"), true},
		{"overwriting the file", write(existing, "package sample\n"), true},
		{"truncating the file", write(existing, contents[:20]), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := asksPermission(t, testGuardHook, tt.payload); got != tt.wantsAsk {
				t.Errorf("asked = %v, want %v", got, tt.wantsAsk)
			}
		})
	}
}

// The worktree guard covers the two git commands that discard work with no way
// back. The everyday undo commands are deliberately left alone.
func TestWorktreeGuard(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		wantsAsk bool
	}{
		{"discarding every change", "git reset --hard", true},
		{"discarding back to a ref", "git reset --hard HEAD~1", true},
		{"after changing directory", "cd /tmp/x && git reset --hard", true},
		{"from another repo", "git -C /tmp/repo reset --hard", true},
		{"deleting untracked files", "git clean -f", true},
		{"deleting untracked directories", "git clean -fd", true},
		{"deleting ignored files too", "git clean -xdf", true},
		{"the long force flag", "git clean --force", true},
		{"unstaging", "git reset HEAD~1", false},
		{"keeping the working tree", "git reset --soft HEAD~1", false},
		{"a dry run", "git clean -n", false},
		{"a long dry run", "git clean --dry-run", false},
		{"discarding one path", "git checkout -- README.md", false},
		{"switching branch", "git checkout main", false},
		{"restoring one path", "git restore internal/run/verify.go", false},
		{"stashing", "git stash", false},
		{"a subject that mentions them", `git commit -m "docs: explain reset --hard and clean -f"`, false},
		{"reading history", "git log --oneline -5", false},
		{"no git at all", "go test ./...", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := asksPermission(t, worktreeGuardHook, bashPayload(tt.command)); got != tt.wantsAsk {
				t.Errorf("asked = %v, want %v for %q", got, tt.wantsAsk, tt.command)
			}
		})
	}
}
