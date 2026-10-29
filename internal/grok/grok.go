// Package grok runs the Grok Build CLI over a repository, for the commit
// messages Ship writes.
package grok

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"peek/internal/commitmsg"
	"peek/internal/git"
	"strings"
	"time"
	"unicode/utf8"
)

// An NDJSON line past maxLine, and the text kept for the answer past
// maxText, are dropped rather than retained.
const (
	maxLine = 1 << 20
	maxText = 1 << 20
)

// Finder-launched apps do not inherit the terminal's PATH. Check where Grok
// Build installs itself as well as the terminal's locations.
func executable() (string, error) {
	if p, err := exec.LookPath("grok"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, ".local", "bin", "grok"),
		filepath.Join(home, ".grok", "bin", "grok"),
		"/opt/homebrew/bin/grok", "/usr/local/bin/grok",
	} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("Grok Build CLI was not found; install Grok Build and run grok login, then retry")
}

// CommitMessage embeds the staged diff and recent subjects in the prompt, so
// Grok Build can answer in one turn without tools. It runs Grok Build's own
// model; the model name, the user's choice for agents that take one, goes
// unused.
func CommitMessage(ctx context.Context, repo string, snapshot git.Snapshot, _ string) (string, error) {
	prompt, _, err := commitmsg.Prompt(ctx, repo, snapshot)
	if err != nil {
		return "", err
	}
	data, err := run(ctx, repo, commitmsg.Schema, prompt, 3*time.Minute, nil)
	if err != nil {
		return "", err
	}
	return commitmsg.Message(data)
}

// JSON runs Grok Build over the repository in a read-only sandbox and
// returns its JSON answer, validated against the schema. Where events is not
// nil, the same NDJSON stream is written to it. If Grok cannot apply that
// profile, the run continues in a throwaway copy of the repository under
// workspace, so its writes cannot reach the repository.
func JSON(ctx context.Context, repo, schemaText, prompt string, timeout time.Duration, events io.Writer) ([]byte, error) {
	return run(ctx, repo, schemaText, prompt, timeout, events)
}

// run is JSON with the sandbox that keeps the repository read-only.
func run(ctx context.Context, repo, schemaText, prompt string, timeout time.Duration, events io.Writer) ([]byte, error) {
	if repo == "" {
		return nil, fmt.Errorf("a repository is required for Grok Build")
	}
	bin, err := executable()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "peek-grok-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	promptFile := filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(promptFile, []byte(prompt), 0o600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{
		"--cwd", repo, "--prompt-file", promptFile, "--json-schema", schemaText,
		"--sandbox", "read-only", "--no-subagents", "--disable-web-search",
		// A headless run cannot ask for approval. The sandbox keeps these
		// reads and git commands read-only, and everything else is refused.
		"--allow", "Bash(git *)", "--allow", "Read", "--allow", "Grep",
		"--reasoning-effort", "max",
		"--output-format", "streaming-json",
	}
	data, err := invoke(ctx, bin, args, events)
	if err == nil || ctx.Err() != nil || !readOnlySandboxRefused(err) {
		return data, err
	}
	// Grok refuses read-only when a runtime socket it would deny
	// (docker.sock) is a symlink into the home directory, as OrbStack and
	// some Docker installs do. A writable workspace cannot keep the
	// repository read-only through permission rules, so the retry reads a
	// copy whose writes go nowhere.
	copied, cleanup, copyErr := copyRepo(repo)
	if copyErr != nil {
		return nil, fmt.Errorf("%w; copying the repository for the retry: %v", err, copyErr)
	}
	defer cleanup()
	// The real repository is out of reach from the copy. Refuse edits of
	// the copy too, so the run still reads the changes it was given.
	retry, retryErr := invoke(ctx, bin, withSandbox(withCWD(args, copied), "workspace",
		"--disallowed-tools", "write,search_replace"), events)
	if retryErr != nil {
		return nil, fmt.Errorf("%w; the retry in a copy failed: %v", err, retryErr)
	}
	return retry, nil
}

// copyRepo copies the repository into a temporary directory, with APFS
// clones where they are supported, for a run whose sandbox cannot keep the
// repository read-only. cleanup removes the copy.
func copyRepo(repo string) (copied string, cleanup func(), err error) {
	repo, err = filepath.EvalSymlinks(repo)
	if err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp("", "peek-grok-repo-")
	if err != nil {
		return "", nil, err
	}
	copied = filepath.Join(dir, "repo")
	if _, err := exec.Command("cp", "-c", "-R", repo, copied).CombinedOutput(); err != nil {
		os.RemoveAll(copied)
		if out, err := exec.Command("cp", "-R", repo, copied).CombinedOutput(); err != nil {
			os.RemoveAll(dir)
			return "", nil, fmt.Errorf("cp: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}
	return copied, func() { os.RemoveAll(dir) }, nil
}

// invoke runs grok with args and returns its JSON answer.
func invoke(ctx context.Context, bin string, args []string, events io.Writer) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	answer := &answerWriter{}
	if events != nil {
		cmd.Stdout = io.MultiWriter(answer, events)
	} else {
		cmd.Stdout = answer
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	answer.flush()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("Grok Build failed: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	if data, ok := answer.answer(); ok {
		return data, nil
	}
	if answer.eventErr != "" {
		return nil, fmt.Errorf("Grok Build returned no JSON answer: %s", answer.eventErr)
	}
	return nil, errors.New("Grok Build returned no JSON answer")
}

// readOnlySandboxRefused reports Grok's fail-closed start when the
// read-only Seatbelt profile cannot be applied.
func readOnlySandboxRefused(err error) bool {
	return strings.Contains(err.Error(), "could not apply the 'read-only' sandbox profile")
}

// withSandbox copies args, replacing the --sandbox profile, and appends extra.
func withSandbox(args []string, profile string, extra ...string) []string {
	out := append([]string(nil), args...)
	for i := 0; i < len(out)-1; i++ {
		if out[i] == "--sandbox" {
			out[i+1] = profile
			break
		}
	}
	return append(out, extra...)
}

// withCWD copies args, replacing the --cwd directory.
func withCWD(args []string, dir string) []string {
	out := append([]string(nil), args...)
	for i := 0; i < len(out)-1; i++ {
		if out[i] == "--cwd" {
			out[i+1] = dir
			break
		}
	}
	return out
}

// answerWriter reads the streaming JSON's NDJSON as it arrives, keeping the
// text chunks and the end event's structured output.
type answerWriter struct {
	line       []byte
	dropping   bool
	text       []byte
	structured json.RawMessage
	eventErr   string
}

func (w *answerWriter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		part := p
		if i >= 0 {
			part = p[:i]
		}
		if !w.dropping {
			if len(w.line)+len(part) > maxLine {
				w.line, w.dropping = nil, true
			} else {
				w.line = append(w.line, part...)
			}
		}
		if i < 0 {
			break
		}
		if !w.dropping {
			w.event(w.line)
		}
		w.line, w.dropping = w.line[:0], false
		p = p[i+1:]
	}
	return n, nil
}

// flush reads a last line the process wrote without a newline.
func (w *answerWriter) flush() {
	if !w.dropping && len(w.line) > 0 {
		w.event(w.line)
	}
	w.line, w.dropping = nil, false
}

func (w *answerWriter) event(data []byte) {
	var event struct {
		Type       string          `json:"type"`
		Data       string          `json:"data"`
		Structured json.RawMessage `json:"structuredOutput"`
		Message    string          `json:"message"`
	}
	if json.Unmarshal(data, &event) != nil {
		return
	}
	switch event.Type {
	case "text":
		w.addText(event.Data)
	case "end":
		if s := bytes.TrimSpace(event.Structured); len(s) > 0 && string(s) != "null" {
			w.structured = bytes.Clone(s)
		}
	case "error":
		w.eventErr = event.Message
	}
}

func (w *answerWriter) addText(chunk string) {
	room := maxText - len(w.text)
	if room <= 0 || chunk == "" {
		return
	}
	if len(chunk) > room {
		chunk = chunk[:room]
		for len(chunk) > 0 {
			r, size := utf8.DecodeLastRuneInString(chunk)
			if r != utf8.RuneError || size > 1 {
				break
			}
			chunk = chunk[:len(chunk)-size]
		}
	}
	w.text = append(w.text, chunk...)
}

// answer is the JSON to return: the end event's structured output, else the
// JSON the text holds, which the model wraps in a fenced block when
// structured output is null.
func (w *answerWriter) answer() ([]byte, bool) {
	if len(w.structured) > 0 {
		return w.structured, true
	}
	data := extractJSON(w.text)
	return data, len(data) > 0
}

// extractJSON reads the first JSON value of text: the whole text, a fenced
// json block, or the first object in prose.
func extractJSON(text []byte) json.RawMessage {
	trimmed := bytes.TrimSpace(text)
	if json.Valid(trimmed) {
		return bytes.Clone(trimmed)
	}
	for _, fence := range [][]byte{[]byte("```json"), []byte("```")} {
		if i := bytes.Index(text, fence); i >= 0 {
			rest := text[i+len(fence):]
			if j := bytes.Index(rest, []byte("```")); j >= 0 {
				if block := bytes.TrimSpace(rest[:j]); json.Valid(block) {
					return bytes.Clone(block)
				}
			}
		}
	}
	for i := bytes.IndexByte(text, '{'); i >= 0; {
		var raw json.RawMessage
		if json.NewDecoder(bytes.NewReader(text[i:])).Decode(&raw) == nil {
			return raw
		}
		next := bytes.IndexByte(text[i+1:], '{')
		if next < 0 {
			break
		}
		i += next + 1
	}
	return nil
}
