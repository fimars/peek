// Package codex runs the Codex CLI over a repository, for the commit
// messages Ship writes and the reviews Review asks for.
package codex

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"peek/internal/commitmsg"
	"peek/internal/git"
	"strings"
	"time"
)

// Finder-launched apps do not inherit the terminal's PATH. Check common
// CLI locations as well as the Codex desktop app's bundled executable.
func executable() (string, error) {
	if p, err := exec.LookPath("codex"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, ".local", "bin", "codex"),
		"/opt/homebrew/bin/codex", "/usr/local/bin/codex",
		"/Applications/Codex.app/Contents/Resources/codex",
	} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("Codex CLI was not found; install Codex and run codex login, then retry")
}

const commitModel = "gpt-6-luna" // fast and affordable, for easier tasks

// CommitMessage embeds the staged diff and recent subjects in the prompt, so
// Codex can answer in one turn without tools. It skips the user's Codex
// config, whose plugins, hooks and reasoning effort suit coding sessions,
// not this, and uses a small model with light reasoning, enough for a
// commit message.
func CommitMessage(ctx context.Context, repo string, snapshot git.Snapshot) (string, error) {
	prompt, complete, err := commitmsg.Prompt(ctx, repo, snapshot)
	if err != nil {
		return "", err
	}
	// A change too large to embed whole takes more thought to sum up.
	effort := "low"
	if !complete {
		effort = "medium"
	}
	data, err := run(ctx, repo, commitmsg.Schema, prompt, 3*time.Minute, nil,
		"--ignore-user-config", "--ignore-rules", "--model", commitModel, "-c", `model_reasoning_effort="`+effort+`"`)
	if err != nil {
		return "", err
	}
	return commitmsg.Message(data)
}

// JSON runs Codex over the repository in a read-only sandbox and returns its
// response, validated against the schema; no file contents are embedded in
// the prompt. Where events is not nil, Codex streams its JSONL events to it.
func JSON(ctx context.Context, repo, schemaText, prompt string, timeout time.Duration, events io.Writer) ([]byte, error) {
	return run(ctx, repo, schemaText, prompt, timeout, events)
}

// run is JSON with extra arguments for codex exec.
func run(ctx context.Context, repo, schemaText, prompt string, timeout time.Duration, events io.Writer, extra ...string) ([]byte, error) {
	if repo == "" {
		return nil, fmt.Errorf("a repository is required for Codex")
	}
	bin, err := executable()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "peek-codex-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	schema := filepath.Join(dir, "schema.json")
	output := filepath.Join(dir, "message.json")
	if err := os.WriteFile(schema, []byte(schemaText), 0o600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"exec", "--ephemeral", "--sandbox", "read-only",
		"--skip-git-repo-check", "--cd", repo, "--color", "never",
		"--output-schema", schema, "--output-last-message", output}
	if events != nil {
		args = append(args, "--json")
	}
	args = append(args, extra...)
	cmd := exec.CommandContext(ctx, bin, append(args, "-")...)
	if events != nil {
		cmd.Stdout = events
	}
	cmd.Stdin = strings.NewReader(prompt)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("Codex failed: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	return os.ReadFile(output)
}
