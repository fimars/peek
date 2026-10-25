package app

import (
	"os"
	"path/filepath"
	"peek/internal/testutil"
	"testing"
)

func TestExplainUI(t *testing.T) {
	repo := testutil.Repo(t)
	testutil.FakeReviewCodex(t, `{"explanation":"It **adds** the docs."}`)
	a, tt := testApp(t, repo, nil)
	if err := tt.Click("Explain"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.explain.running || a.explain.scope != "Uncommitted changes" || !tt.HasText("Explanation") {
		t.Fatalf("explanation %+v, texts %q", a.explain, tt.Texts())
	}
	if err := tt.Click("Explanation"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("adds") {
		t.Fatalf("the sheet does not show the explanation, texts %q", tt.Texts())
	}
	// The changes changed while the sheet showed, it is out of date.
	if err := os.WriteFile(filepath.Join(repo, "docs/new.md"), []byte("new content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.refresh()
	tt.Frame()
	if !tt.HasText("out of date") {
		t.Fatalf("the stale sheet has no notice, texts %q", tt.Texts())
	}
	if err := tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Explain Again") {
		t.Fatalf("a stale explanation shows, texts %q", tt.Texts())
	}
}

func TestGrokExplainUI(t *testing.T) {
	repo := testutil.Repo(t)
	testutil.FakeGrok(t, `{"explanation":"It **adds** the docs."}`)
	a, tt := testApp(t, repo, nil)
	settle(tt)
	if err := tt.Click("Ship, Review, and Explain with Codex"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Grok Build"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Explain"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.explain.running || a.explain.agent != agentGrok || a.explain.scope != "Uncommitted changes" || !tt.HasText("Explanation") {
		t.Fatalf("explanation %+v, texts %q", a.explain, tt.Texts())
	}
	if err := tt.Click("Explanation"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("adds") {
		t.Fatalf("the sheet does not show Grok Build's explanation, texts %q", tt.Texts())
	}
	if err := tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Ship, Review, and Explain with Grok Build"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Codex"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.explain.agent != agentGrok {
		t.Fatalf("the explanation's agent became %q", a.explain.agent)
	}
}
