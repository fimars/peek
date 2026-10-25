package codex

import (
	"context"
	"os"
	"peek/internal/git"
	"peek/internal/testutil"
	"strings"
	"testing"
)

func TestCommitMessageReturnsCodexMessage(t *testing.T) {
	testutil.FakeCodex(t)
	repo, _ := testutil.RemoteRepo(t)
	testutil.Git(t, repo, "add", "--all")
	snapshot := git.Snapshot{Head: testutil.Git(t, repo, "rev-parse", "HEAD"), Tree: testutil.Git(t, repo, "write-tree")}
	message, err := CommitMessage(context.Background(), repo, snapshot)
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
	message, err := CommitMessage(context.Background(), repo, snapshot)
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
