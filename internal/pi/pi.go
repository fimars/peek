// Package pi runs the Pi coding agent CLI over a repository, for the
// commit messages Ship writes and the reviews and answers of its
// conversations.
package pi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"peek/internal/commitmsg"
	"peek/internal/git"
	"strings"
	"time"
)

// Executable finds the Pi CLI. Finder-launched apps do not inherit the
// terminal's PATH. Check the package managers' locations as well as the
// terminal's.
func Executable() (string, error) {
	if p, err := exec.LookPath("pi"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, ".bun", "bin", "pi"),
		filepath.Join(home, ".local", "bin", "pi"),
		"/opt/homebrew/bin/pi", "/usr/local/bin/pi",
	} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("Pi CLI was not found; install Pi and sign it in, then retry")
}

// Event is what Pi does as it answers, as Stream reports it: the text it
// adds to its answer, or a step it starts, such as a command it runs.
// The steps are named as Codex's, for the progress they tell to read the
// same.
type Event struct {
	// Delta is text Pi adds to its answer.
	Delta string
	// Step is the kind of step Pi starts, "" for none: "reasoning",
	// "commandExecution", "mcpToolCall" or "agentMessage"; Detail tells
	// it, as a command's line.
	Step, Detail string
}

// CommitMessage embeds the staged diff and recent subjects in the prompt,
// so Pi can answer in one turn without tools. It runs the model the user
// chose, Pi's own where the name is "": Codex model names do not name
// Pi's, so the choice is one of Pi's, or none.
func CommitMessage(ctx context.Context, repo string, snapshot git.Snapshot, model string) (string, error) {
	prompt, complete, err := commitmsg.Prompt(ctx, repo, snapshot)
	if err != nil {
		return "", err
	}
	// A change too large to embed whole takes more thought to sum up.
	effort := "low"
	if !complete {
		effort = "medium"
	}
	args := []string{"--print", "--no-session", "-ne", "-ns", "-nc",
		"--tools", "read,grep,find,ls,bash", "--thinking", effort}
	if model != "" {
		args = append(args, "--model", model)
	}
	text, err := once(ctx, repo, args, prompt, 3*time.Minute)
	if err != nil {
		return "", err
	}
	data := extractJSON([]byte(text))
	if len(data) == 0 {
		return "", errors.New("Pi returned no JSON answer")
	}
	return commitmsg.Message(data)
}

// Stream has Pi answer the prompt over the repository: a new
// conversation where session is "", else the one of that id, resumed with
// its earlier turns in mind. Pi keeps the conversation, whose id it
// returns, for the next turn. Each piece of the answer and each step
// goes to on as it happens; the answer is Pi's final text.
func Stream(ctx context.Context, repo, session, model, prompt string, timeout time.Duration, on func(Event)) (string, string, error) {
	if repo == "" {
		return "", session, fmt.Errorf("a repository is required for Pi")
	}
	bin, err := Executable()
	if err != nil {
		return "", session, err
	}
	args := []string{"--mode", "json", "--print", "-ne", "-ns", "-na",
		"--tools", "read,grep,find,ls,bash"}
	if session != "" {
		args = append(args, "--session-id", session)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd, cleanup, err := readOnly(ctx, bin, args)
	if err != nil {
		return "", session, err
	}
	defer cleanup()
	cmd.Dir = repo
	cmd.Stdin = strings.NewReader(prompt)
	events := &eventReader{on: on}
	if events.on == nil {
		events.on = func(Event) {}
	}
	cmd.Stdout = events
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", session, ctx.Err()
		}
		return "", session, fmt.Errorf("Pi failed: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	if events.session != "" {
		session = events.session
	}
	if events.err != nil {
		return "", session, events.err
	}
	text := events.answer()
	if text == "" {
		return "", session, errors.New("Pi returned no answer")
	}
	return text, session, nil
}

// JSON is Stream answered as JSON: the answer's text read for its JSON
// value, which the model may wrap in prose or a fenced block, as review
// answers are.
func JSON(ctx context.Context, repo, session, model, prompt string, timeout time.Duration, on func(Event)) ([]byte, string, error) {
	text, session, err := Stream(ctx, repo, session, model, prompt, timeout, on)
	if err != nil {
		return nil, session, err
	}
	data := extractJSON([]byte(text))
	if len(data) == 0 {
		return nil, session, errors.New("Pi returned no JSON answer")
	}
	return data, session, nil
}

// once runs Pi with args and the prompt on its input, and returns the
// final text it writes.
func once(ctx context.Context, repo string, args []string, prompt string, timeout time.Duration) (string, error) {
	bin, err := Executable()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd, cleanup, err := readOnly(ctx, bin, args)
	if err != nil {
		return "", err
	}
	defer cleanup()
	cmd.Dir = repo
	cmd.Stdin = strings.NewReader(prompt)
	var out bytes.Buffer
	cmd.Stdout = &out
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("Pi failed: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	return out.String(), nil
}

// readOnly puts the command under macOS's seatbelt, which denies writes
// everywhere but the system's temporary folders and Pi's own, as Codex's
// sandbox does: the repository, and everything else, only reads. Where
// sandbox-exec is missing, the run goes without it, on the prompt's rules
// alone. cleanup removes the profile the wrapper reads.
func readOnly(ctx context.Context, bin string, args []string) (cmd *exec.Cmd, cleanup func(), err error) {
	cmd = exec.CommandContext(ctx, bin, args...)
	wrap, lookErr := exec.LookPath("sandbox-exec")
	if lookErr != nil {
		return cmd, func() {}, nil
	}
	home, _ := os.UserHomeDir()
	tmp := strings.TrimSuffix(os.TempDir(), "/")
	real, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		real = tmp
	}
	profile := "(version 1)\n(allow default)\n(deny file-write*)\n" +
		`(allow file-write* (literal "/dev/null"))` + "\n" +
		`(allow file-write* (subpath "/private/tmp"))` + "\n" +
		`(allow file-write* (subpath "` + tmp + `"))` + "\n" +
		`(allow file-write* (subpath "` + real + `"))` + "\n" +
		`(allow file-write* (subpath "` + filepath.Join(home, ".pi") + `"))` + "\n"
	file, err := os.CreateTemp("", "peek-pi-*.sb")
	if err != nil {
		return cmd, func() {}, nil // the run goes without the wrapper
	}
	if _, err := file.WriteString(profile); err != nil {
		file.Close()
		os.Remove(file.Name())
		return cmd, func() {}, nil
	}
	file.Close()
	wrapped := exec.CommandContext(ctx, wrap, "-f", file.Name(), "--", bin)
	wrapped.Args = append(wrapped.Args, args...)
	return wrapped, func() { os.Remove(file.Name()) }, nil
}

// An event line past maxEvent is dropped rather than read whole: Pi's
// closing events repeat the whole conversation, which the answer keeps
// apart.
const maxEvent = 8 << 20

// eventReader reads Pi's JSONL events as they arrive, handing each to on
// and keeping the answer, the conversation's id and the error that ended
// it.
type eventReader struct {
	on       func(Event)
	session  string
	err      error
	text     strings.Builder // the answer as it streams
	complete []string        // each message's answer, as it ended
	// paragraph tells that the next text of the answer follows an earlier
	// message's, apart from it as a paragraph.
	paragraph bool
	line      []byte
	dropping  bool
}

func (w *eventReader) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		part := p
		if i >= 0 {
			part = p[:i]
		}
		if !w.dropping {
			if len(w.line)+len(part) > maxEvent {
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

// event is one of Pi's events, as the JSON stream shapes it, told apart
// by what it holds.
func (w *eventReader) event(data []byte) {
	var event struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			StopReason   string `json:"stopReason"`
			ErrorMessage string `json:"errorMessage"`
		} `json:"message"`
		AssistantMessageEvent struct {
			Type   string `json:"type"`
			Delta  string `json:"delta"`
			Reason string `json:"reason"`
			Error  string `json:"error"`
		} `json:"assistantMessageEvent"`
		ToolName string `json:"toolName"`
		Args     struct {
			Command string `json:"command"`
		} `json:"args"`
		Success    *bool  `json:"success"`
		FinalError string `json:"finalError"`
	}
	if json.Unmarshal(data, &event) != nil {
		return
	}
	switch event.Type {
	case "session":
		w.session = event.ID
	case "message_start":
		if event.Message.Role == "assistant" && w.text.Len() > 0 {
			w.paragraph = true // the next text is a message apart
		}
	case "message_update":
		switch event.AssistantMessageEvent.Type {
		case "text_delta":
			if d := event.AssistantMessageEvent.Delta; d != "" {
				if w.paragraph {
					w.text.WriteString("\n\n")
					w.on(Event{Delta: "\n\n"})
					w.paragraph = false
				}
				w.text.WriteString(d)
				w.on(Event{Delta: d})
			}
		case "thinking_start":
			w.on(Event{Step: "reasoning"})
		case "error":
			if event.AssistantMessageEvent.Error != "" {
				w.err = errors.New(event.AssistantMessageEvent.Error)
			}
		}
	case "message_end":
		if event.Message.Role != "assistant" {
			return
		}
		var message strings.Builder
		for _, block := range event.Message.Content {
			if block.Type == "text" {
				message.WriteString(block.Text)
			}
		}
		if message.Len() > 0 {
			w.complete = append(w.complete, message.String())
		}
		switch event.Message.StopReason {
		case "error", "aborted", "length":
			if event.Message.ErrorMessage != "" {
				w.err = errors.New(event.Message.ErrorMessage)
			} else if w.err == nil {
				w.err = fmt.Errorf("Pi's answer ended %s", event.Message.StopReason)
			}
		}
	case "tool_execution_start":
		if event.ToolName == "bash" {
			w.on(Event{Step: "commandExecution", Detail: strings.Join(strings.Fields(event.Args.Command), " ")})
			return
		}
		w.on(Event{Step: "mcpToolCall", Detail: event.ToolName})
	case "auto_retry_end":
		if event.Success != nil && !*event.Success && event.FinalError != "" {
			w.err = errors.New(event.FinalError)
		}
	}
}

// answer is the answer as it stands: what the messages that ended said,
// else what streamed, joined as paragraphs.
func (w *eventReader) answer() string {
	if len(w.complete) > 0 {
		return strings.TrimSpace(strings.Join(w.complete, "\n\n"))
	}
	return strings.TrimSpace(w.text.String())
}

// extractJSON reads the first JSON value of text: the whole text, a
// fenced json block, or the first object in prose.
func extractJSON(text []byte) []byte {
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
