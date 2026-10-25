package grok

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"peek/internal/git"
	"peek/internal/testutil"
	"strings"
	"testing"
	"time"
)

const answer = `{"status":"complete","summary":"No actionable defects found.","findings":[]}`

func ndjson(t *testing.T, event any) string {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestJSONReadsStructuredOutput(t *testing.T) {
	testutil.FakeGrok(t, answer)
	repo := t.TempDir()
	promptFile := filepath.Join(t.TempDir(), "prompt")
	cwdFile := filepath.Join(t.TempDir(), "cwd")
	schemaFile := filepath.Join(t.TempDir(), "schema")
	t.Setenv("PEEK_TEST_GROK_PROMPT", promptFile)
	t.Setenv("PEEK_TEST_GROK_CWD", cwdFile)
	t.Setenv("PEEK_TEST_GROK_SCHEMA", schemaFile)
	const prompt, schema = "Review this change.", `{"type":"object"}`
	data, err := JSON(context.Background(), repo, schema, prompt, time.Minute, nil)
	if err != nil || string(data) != answer {
		t.Fatalf("answer %q, %v", data, err)
	}
	if got, err := os.ReadFile(promptFile); err != nil || string(got) != prompt {
		t.Fatalf("prompt file %q, %v; want %q", got, err, prompt)
	}
	if got, err := os.ReadFile(cwdFile); err != nil || string(got) != repo {
		t.Fatalf("cwd %q, %v; want %q", got, err, repo)
	}
	if got, err := os.ReadFile(schemaFile); err != nil || string(got) != schema {
		t.Fatalf("schema %q, %v; want %q", got, err, schema)
	}
}

func TestJSONFallsBackToFencedText(t *testing.T) {
	events := ndjson(t, map[string]string{"type": "text", "data": "Here is the review:\n```json\n" + answer + "\n```\nDone."}) + "\n" +
		`{"type":"end","structuredOutput":null}`
	testutil.FakeGrokEvents(t, events)
	data, err := JSON(context.Background(), t.TempDir(), "{}", "prompt", time.Minute, nil)
	if err != nil || string(data) != answer {
		t.Fatalf("answer %q, %v", data, err)
	}
}

func TestJSONStreamsEvents(t *testing.T) {
	toolCall := ndjson(t, map[string]any{"type": "tool_call", "title": "Read", "rawInput": map[string]string{"command": "git diff"}})
	events := toolCall + "\n" +
		ndjson(t, map[string]string{"type": "text", "data": "```json\n" + answer + "\n```"}) + "\n" +
		`{"type":"end","structuredOutput":null}` + "\n"
	testutil.FakeGrokEvents(t, events)
	var seen bytes.Buffer
	data, err := JSON(context.Background(), t.TempDir(), "{}", "prompt", time.Minute, &seen)
	if err != nil || string(data) != answer {
		t.Fatalf("answer %q, %v", data, err)
	}
	if !strings.Contains(seen.String(), toolCall) {
		t.Fatalf("events writer saw %q, want the tool call", seen.String())
	}
}

func TestJSONReportsErrorEvent(t *testing.T) {
	testutil.FakeGrokEvents(t, `{"type":"error","message":"quota exceeded"}`+"\n")
	_, err := JSON(context.Background(), t.TempDir(), "{}", "prompt", time.Minute, nil)
	if err == nil || !strings.Contains(err.Error(), "Grok Build returned no JSON answer: quota exceeded") {
		t.Fatalf("error %v, want the error event's message", err)
	}
}

func TestJSONRequiresGrok(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	_, err := JSON(context.Background(), t.TempDir(), "{}", "prompt", time.Minute, nil)
	if err == nil || !strings.Contains(err.Error(), "Grok Build CLI was not found") {
		t.Fatalf("error %v, want a missing CLI", err)
	}
}

func TestJSONCancels(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "grok"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	if _, err := JSON(ctx, t.TempDir(), "{}", "prompt", time.Minute, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v, want cancellation", err)
	}
}

func TestJSONDropsOversizedLine(t *testing.T) {
	events := ndjson(t, map[string]string{"type": "text", "data": strings.Repeat("x", maxLine+1)}) + "\n" +
		ndjson(t, map[string]string{"type": "text", "data": "```json\n" + answer + "\n```"}) + "\n"
	testutil.FakeGrokEvents(t, events)
	data, err := JSON(context.Background(), t.TempDir(), "{}", "prompt", time.Minute, nil)
	if err != nil || string(data) != answer {
		t.Fatalf("answer %q, %v", data, err)
	}
}

func TestAnswerWriterSplitsWrites(t *testing.T) {
	stream := ndjson(t, map[string]string{"type": "text", "data": "```json\n" + answer + "\n```"}) + "\n" +
		`{"type":"end","structuredOutput":null}`
	var w answerWriter
	for i := range len(stream) {
		if _, err := w.Write([]byte{stream[i]}); err != nil {
			t.Fatal(err)
		}
	}
	w.flush()
	data, ok := w.answer()
	if !ok || string(data) != answer {
		t.Fatalf("answer %q, %v", data, ok)
	}
}

// Opt in to a tiny real CLI request; the regular tests use FakeGrok and
// local bare remotes, so they need neither a login nor network access.
func TestGrokCommitMessageSmoke(t *testing.T) {
	if os.Getenv("PEEK_GROK_SMOKE") != "1" {
		t.Skip("set PEEK_GROK_SMOKE=1 to check the installed Grok Build CLI")
	}
	repo, _ := testutil.RemoteRepo(t)
	testutil.Git(t, repo, "add", "--all")
	snapshot := git.Snapshot{Head: testutil.Git(t, repo, "rev-parse", "HEAD"), Tree: testutil.Git(t, repo, "write-tree")}
	message, err := CommitMessage(context.Background(), repo, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(message) == "" {
		t.Fatal("empty message")
	}
	if testutil.Git(t, repo, "rev-parse", "HEAD") != snapshot.Head || testutil.Git(t, repo, "write-tree") != snapshot.Tree {
		t.Fatal("Grok Build modified HEAD or the index")
	}
	t.Log(message)
}

func TestExtractJSON(t *testing.T) {
	for _, tc := range []struct{ name, text, want string }{
		{"whole text", `{"a":1}`, `{"a":1}`},
		{"fenced block", "Prose.\n```json\n{\"a\":1}\n```\nMore prose.", `{"a":1}`},
		{"object in prose", `The review is {"a":1} and more.`, `{"a":1}`},
		{"braces in prose", `Here {is} the review: {"a":1}`, `{"a":1}`},
		{"array", `[1,2]`, `[1,2]`},
		{"nothing", "no json here", ""},
		{"invalid fenced block", "```json\nnope\n```", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(extractJSON([]byte(tc.text))); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
