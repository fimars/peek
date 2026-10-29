package codex

import (
	"context"
	"os"
	"path/filepath"
	"peek/internal/git"
	"peek/internal/testutil"
	"strings"
	"testing"
	"time"
)

func TestCommitMessageReturnsCodexMessage(t *testing.T) {
	testutil.FakeCodex(t)
	repo, _ := testutil.RemoteRepo(t)
	testutil.Git(t, repo, "add", "--all")
	snapshot := git.Snapshot{Head: testutil.Git(t, repo, "rev-parse", "HEAD"), Tree: testutil.Git(t, repo, "write-tree")}
	message, err := CommitMessage(context.Background(), repo, snapshot, "")
	if err != nil || message != "Update the files" {
		t.Fatalf("message %q, %v", message, err)
	}
}

// Opt in to a tiny real CLI request; the regular tests use FakeCodex and
// local bare remotes, so they need neither a login nor network access.
func TestCodexSmoke(t *testing.T) {
	if os.Getenv("PEEK_CODEX_SMOKE") != "1" {
		t.Skip("set PEEK_CODEX_SMOKE=1 to check the installed Codex CLI")
	}
	repo, _ := testutil.RemoteRepo(t)
	testutil.Git(t, repo, "add", "--all")
	snapshot := git.Snapshot{Head: testutil.Git(t, repo, "rev-parse", "HEAD"), Tree: testutil.Git(t, repo, "write-tree")}
	message, err := CommitMessage(context.Background(), repo, snapshot, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(message) == "" {
		t.Fatal("empty message")
	}
	if testutil.Git(t, repo, "rev-parse", "HEAD") != snapshot.Head || testutil.Git(t, repo, "write-tree") != snapshot.Tree {
		t.Fatal("Codex modified HEAD or the index")
	}
	t.Log(message)
}

// A turn starts a conversation Codex keeps, and the next resumes it.
func TestTurn(t *testing.T) {
	testutil.FakeReviewCodex(t, `{"answer":"ok"}`)
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("PEEK_TEST_CODEX_ARGS", argsFile)
	t.Setenv("PEEK_TEST_CODEX_THREAD", "thread-1")
	repo := testutil.Repo(t)
	schema := `{"type":"object"}`
	data, session, err := Turn(context.Background(), repo, "", schema, "First", time.Minute, nil)
	if err != nil || string(data) != `{"answer":"ok"}` || session != "thread-1" {
		t.Fatalf("first turn %q, %q, %v", data, session, err)
	}
	if _, session, err = Turn(context.Background(), repo, session, schema, "Next", time.Minute, nil); err != nil || session != "thread-1" {
		t.Fatalf("next turn %q, %v", session, err)
	}
	args, _ := os.ReadFile(argsFile)
	lines := strings.Split(strings.TrimSpace(string(args)), "\n")
	if len(lines) != 2 || strings.Contains(lines[0], "--ephemeral") || !strings.Contains(lines[0], "--sandbox read-only") ||
		!strings.HasPrefix(lines[1], "exec resume -c sandbox_mode=\"read-only\"") || !strings.HasSuffix(lines[1], "thread-1 -") {
		t.Fatalf("codex ran with %q", lines)
	}
}

// Stream streams the answer through the app server, a new conversation's
// or the one resumed.
func TestStream(t *testing.T) {
	testutil.FakeReviewCodex(t, `{"answer":"It streams."}`)
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("PEEK_TEST_CODEX_ARGS", argsFile)
	t.Setenv("PEEK_TEST_CODEX_THREAD", "thread-1")
	repo := testutil.Repo(t)
	var deltas []string
	on := func(e Event) {
		if e.Delta != "" {
			deltas = append(deltas, e.Delta)
		}
	}
	text, session, err := Stream(context.Background(), repo, "", "First", time.Minute, on)
	if err != nil || text != "It streams." || session != "thread-1" || strings.Join(deltas, "") != "It streams." {
		t.Fatalf("first turn %q, %q, %v, deltas %q", text, session, err, deltas)
	}
	if _, _, err = Stream(context.Background(), repo, session, "Next", time.Minute, on); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(argsFile)
	if lines := strings.Split(strings.TrimSpace(string(args)), "\n"); len(lines) != 3 || lines[2] != "app-server resume thread-1" {
		t.Fatalf("codex ran with %q", lines)
	}
}
