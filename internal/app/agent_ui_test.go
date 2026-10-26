package app

import (
	"os"
	"path/filepath"
	"peek/internal/testutil"
	"slices"
	"strings"
	"testing"
)

func TestAgentDefaultsToCodex(t *testing.T) {
	a := newApp("/tmp/repo")
	if got := a.agentKind(); got != agentCodex {
		t.Fatalf("agent %q, want Codex", got)
	}
	if def := agentDefOf(a.agentKind()); def.name != "Codex" {
		t.Fatalf("agent named %q, want Codex", def.name)
	}
}

func TestParseAgent(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  agentKind
	}{
		{"grok", agentGrok}, {"codex", agentCodex}, {"", agentCodex}, {"other", agentCodex},
	} {
		if got := parseAgent(tc.value); got != tc.want {
			t.Errorf("parseAgent(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestAgentMenu(t *testing.T) {
	a, tt := testApp(t, testutil.Repo(t), nil)
	settle(tt)
	if err := tt.Click("Ship with Codex"); err != nil {
		t.Fatal(err)
	}
	if got, want := tt.Menu(), []string{"Codex", "Grok Build"}; !slices.Equal(got, want) {
		t.Fatalf("menu %q, want %q", got, want)
	}
	if err := tt.ChooseMenuItem("Grok Build"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.agentKind() != agentGrok || !tt.HasText("Ship with Grok Build") {
		t.Fatalf("agent %q, texts %q", a.agentKind(), tt.Texts())
	}
	a.push = pushState{running: true}
	tt.Frame()
	if err := tt.Click("Ship with Grok Build"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Menu(); len(got) != 0 {
		t.Fatalf("menu %q opened while shipping", got)
	}
}

func TestAgentChoicePersists(t *testing.T) {
	p := &projects{dir: t.TempDir()}
	if got := p.readAgent(); got != agentCodex {
		t.Fatalf("missing file gave %q, want Codex", got)
	}
	for _, content := range []string{"", "codex\n", "nonsense\n"} {
		if err := os.WriteFile(filepath.Join(p.dir, agentFile), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := p.readAgent(); got != agentCodex {
			t.Fatalf("%q gave %q, want Codex", content, got)
		}
	}
	p.setAgent(agentGrok)
	if p.agent != agentGrok {
		t.Fatalf("agent %q, want Grok Build", p.agent)
	}
	if data, err := os.ReadFile(filepath.Join(p.dir, agentFile)); err != nil || string(data) != "grok\n" {
		t.Fatalf("agent file %q, %v; want %q", data, err, "grok\n")
	}
	if got := (&projects{dir: p.dir}).readAgent(); got != agentGrok {
		t.Fatalf("a new projects value read %q, want Grok Build", got)
	}
}

func TestAgentShipFlow(t *testing.T) {
	repo, remote := testutil.RemoteRepo(t)
	testutil.FakeGrok(t, `{"changes":["Update the files"],"subject":"Update the files"}`)
	prompt := filepath.Join(t.TempDir(), "prompt")
	t.Setenv("PEEK_TEST_GROK_PROMPT", prompt)
	a, tt := testApp(t, repo, nil)
	settle(tt)
	if err := tt.Click("Ship with Codex"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Grok Build"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.agentKind() != agentGrok {
		t.Fatalf("agent %q, want Grok Build", a.agentKind())
	}
	if err := tt.Click("Ship"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.push.running || !tt.HasText("Shipped") {
		t.Fatalf("push %+v, texts %q", a.push, tt.Texts())
	}
	if subject := testutil.Git(t, remote, "log", "-1", "--format=%s"); subject != "Update the files" {
		t.Fatalf("remote subject %q", subject)
	}
	if got, err := os.ReadFile(prompt); err != nil || !strings.Contains(string(got), "Staged tree:") {
		t.Fatalf("commit prompt %q, %v", got, err)
	}
}
