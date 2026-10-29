package pi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"peek/internal/git"
	"peek/internal/testutil"
	"strconv"
)

// events are the JSON-mode events of a run that reads a file and answers
// in two messages, as the ones Pi writes.
var events = `{"type":"session","id":"s-7","cwd":"/repo"}
{"type":"agent_start"}
{"type":"message_start","message":{"role":"assistant","content":[],"stopReason":"pending"}}
{"type":"message_update","assistantMessageEvent":{"type":"thinking_start","contentIndex":0}}
{"type":"message_update","assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"It "}}
{"type":"message_update","assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"moves."}}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"It moves."}],"stopReason":"stop"}}
{"type":"tool_execution_start","toolCallId":"c1","toolName":"bash","args":{"command":"git diff --stat HEAD"}}
{"type":"message_start","message":{"role":"assistant","content":[],"stopReason":"pending"}}
{"type":"message_update","assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"Again."}}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Again."}],"stopReason":"stop"}}
{"type":"agent_settled","aborted":false}
`

func TestStreamReadsEvents(t *testing.T) {
	testutil.FakePiEvents(t, events)
	var deltas, steps []string
	text, session, err := Stream(context.Background(), testutil.Repo(t), "", "", "Ask", time.Minute, func(e Event) {
		if e.Delta != "" {
			deltas = append(deltas, e.Delta)
		}
		if e.Step != "" {
			steps = append(steps, e.Step)
		}
	})
	if err != nil || text != "It moves.\n\nAgain." || session != "s-7" {
		t.Fatalf("text %q, session %q, %v", text, session, err)
	}
	// The streamed answer joins its messages' text as paragraphs, and the
	// steps tell the command it ran.
	if want := []string{"It ", "moves.", "\n\n", "Again."}; !slices.Equal(deltas, want) {
		t.Fatalf("deltas %q, want %q", deltas, want)
	}
	if !slices.Contains(steps, "reasoning") || !slices.Contains(steps, "commandExecution") {
		t.Fatalf("steps %q", steps)
	}
}

func TestStreamPrefersCompletedMessages(t *testing.T) {
	// The message that ended says what the deltas only guessed at.
	testutil.FakePiEvents(t, `{"type":"session","id":"s-1"}
{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"guessed"}}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"final"}],"stopReason":"stop"}}
`)
	text, _, err := Stream(context.Background(), testutil.Repo(t), "", "", "Ask", time.Minute, nil)
	if err != nil || text != "final" {
		t.Fatalf("text %q, %v", text, err)
	}
}

func TestStreamReportsFailedAnswer(t *testing.T) {
	testutil.FakePiEvents(t, `{"type":"session","id":"s-2"}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":""}],"stopReason":"error","errorMessage":"quota exceeded"}}
`)
	if _, _, err := Stream(context.Background(), testutil.Repo(t), "", "", "Ask", time.Minute, nil); err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("err %v, want quota exceeded", err)
	}
}

func TestStreamRequiresAnAnswer(t *testing.T) {
	testutil.FakePiEvents(t, `{"type":"session","id":"s-3"}
{"type":"agent_settled","aborted":false}
`)
	if _, _, err := Stream(context.Background(), testutil.Repo(t), "", "", "Ask", time.Minute, nil); err == nil || !strings.Contains(err.Error(), "no answer") {
		t.Fatalf("err %v, want no answer", err)
	}
}

func TestStreamFails(t *testing.T) {
	t.Setenv("PEEK_TEST_PI_FAIL", "1")
	testutil.FakePiEvents(t, "")
	if _, _, err := Stream(context.Background(), testutil.Repo(t), "", "", "Ask", time.Minute, nil); err == nil || !strings.Contains(err.Error(), "Pi failed") {
		t.Fatalf("err %v, want Pi failed", err)
	}
}

func TestJSONReadsProseWrappedAnswer(t *testing.T) {
	// The answer's text holds a fenced JSON block, which the event quotes
	// as JSON itself.
	answer := "Sure:\n```json\n" + `{"changes":["Update the files"],"theme":"","subject":"Update the files"}` + "\n```\n"
	event := fmt.Sprintf(`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":%s}],"stopReason":"stop"}}`, strconv.Quote(answer))
	testutil.FakePiEvents(t, `{"type":"session","id":"s-4"}`+"\n"+event+"\n")
	data, _, err := JSON(context.Background(), testutil.Repo(t), "", "", "Ask", time.Minute, nil)
	if err != nil || !strings.Contains(string(data), "Update the files") {
		t.Fatalf("data %s, %v", data, err)
	}
}

func TestCommitMessageReturnsPiMessage(t *testing.T) {
	testutil.FakePi(t, `{"changes":["Update the files"],"theme":"","subject":"Update the files"}`)
	repo, _ := testutil.RemoteRepo(t)
	testutil.Git(t, repo, "add", "--all")
	snapshot := git.Snapshot{Head: testutil.Git(t, repo, "rev-parse", "HEAD"), Tree: testutil.Git(t, repo, "write-tree")}
	promptFile, modelFile := filepath.Join(t.TempDir(), "prompt"), filepath.Join(t.TempDir(), "model")
	t.Setenv("PEEK_TEST_PI_PROMPT", promptFile)
	t.Setenv("PEEK_TEST_PI_MODEL", modelFile)
	message, err := CommitMessage(context.Background(), repo, snapshot, "fast-model")
	if err != nil || message != "Update the files" {
		t.Fatalf("message %q, %v", message, err)
	}
	prompt, err := os.ReadFile(promptFile)
	if err != nil || !strings.Contains(string(prompt), "Write the Git commit message") {
		t.Fatalf("prompt %q, %v", prompt, err)
	}
	if model, err := os.ReadFile(modelFile); err != nil || string(model) != "fast-model" {
		t.Fatalf("model %q, %v", model, err)
	}
}

// The user's chosen model is one of Pi's, or none of it: passed through
// where it names one, left out where it is "".
func TestCommitMessageWithoutAModel(t *testing.T) {
	testutil.FakePi(t, `{"changes":["Update the files"],"theme":"","subject":"Update the files"}`)
	repo, _ := testutil.RemoteRepo(t)
	testutil.Git(t, repo, "add", "--all")
	snapshot := git.Snapshot{Head: testutil.Git(t, repo, "rev-parse", "HEAD"), Tree: testutil.Git(t, repo, "write-tree")}
	modelFile := filepath.Join(t.TempDir(), "model")
	t.Setenv("PEEK_TEST_PI_MODEL", modelFile)
	if _, err := CommitMessage(context.Background(), repo, snapshot, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(modelFile); !os.IsNotExist(err) {
		t.Fatalf("model chosen %v, want the agent's own", err)
	}
}

func TestStreamRequiresPi(t *testing.T) {
	repo := testutil.Repo(t) // git first; without it on the PATH there is no pi either
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir()) // nor in the package managers' places
	if _, _, err := Stream(context.Background(), repo, "", "", "Ask", time.Minute, nil); err == nil || !strings.Contains(err.Error(), "Pi CLI was not found") {
		t.Fatalf("err %v, want Pi CLI was not found", err)
	}
}

// Opt in to a tiny real CLI request; the regular tests use FakePi, so
// they need neither a login nor network access.
func TestPiSmoke(t *testing.T) {
	if os.Getenv("PEEK_PI_SMOKE") != "1" {
		t.Skip("set PEEK_PI_SMOKE=1 to check the installed Pi CLI")
	}
	repo := testutil.Repo(t)
	text, _, err := Stream(context.Background(), repo, "", "", "Reply with only the word: ready", time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(text)
	testutil.Git(t, repo, "add", "--all")
	snapshot := git.Snapshot{Head: testutil.Git(t, repo, "rev-parse", "HEAD"), Tree: testutil.Git(t, repo, "write-tree")}
	message, err := CommitMessage(context.Background(), repo, snapshot, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(message)
	if strings.TrimSpace(message) == "" {
		t.Fatal("empty message")
	}
}

func TestExtractJSON(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{`{"a":1}`, `{"a":1}`},
		{"prose ```json\n{\"a\":2}\n``` more", `{"a":2}`},
		{"Here it is:\n{\"a\":3}\nEnjoy.", `{"a":3}`},
		{`{"a":4`, ``},
	} {
		if got := string(extractJSON([]byte(tc.text))); got != tc.want {
			t.Errorf("extractJSON(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}
