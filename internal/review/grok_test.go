package review

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"peek/internal/testutil"
	"slices"
	"strings"
	"testing"
)

func TestGrokReviews(t *testing.T) {
	for _, tc := range []struct {
		name, response, verdict string
		findings                int
	}{
		{"pass", testutil.PassingReview, "passed", 0},
		{"fail", testutil.FailingReview, "failed", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.FakeGrok(t, tc.response)
			repo := testutil.Repo(t)
			result, err := Grok(context.Background(), repo, reviewTestSnapshot(t, repo), nil)
			if err != nil || result.Verdict != tc.verdict || len(result.Findings) != tc.findings {
				t.Fatalf("result %+v, %v; want %s with %d findings", result, err, tc.verdict, tc.findings)
			}
		})
	}
}

// The model sometimes leaves the schema's empty list or summary out of an
// otherwise complete review.
func TestGrokFillsIncompleteFields(t *testing.T) {
	for _, tc := range []struct{ name, response string }{
		{"missing summary", `{"status":"complete","findings":[]}`},
		{"missing findings", `{"status":"complete","summary":"No defects"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.FakeGrok(t, tc.response)
			repo := testutil.Repo(t)
			result, err := Grok(context.Background(), repo, reviewTestSnapshot(t, repo), nil)
			if err != nil || result.Verdict != "passed" || result.Summary == "" || result.Findings == nil {
				t.Fatalf("result %+v, %v; want a passing review with a summary and an empty list", result, err)
			}
		})
	}
}

// Grok Build names a finding's file where the schema says path.
func TestGrokReadsFileAsPath(t *testing.T) {
	testutil.FakeGrok(t, `{"status":"complete","summary":"Empty inputs now crash.","findings":[{"priority":2,"title":"Handle empty inputs","body":"Restore the guard.","file":"calc.py","line":2}]}`)
	repo := testutil.Repo(t)
	result, err := Grok(context.Background(), repo, reviewTestSnapshot(t, repo), nil)
	if err != nil || result.Verdict != "failed" || len(result.Findings) != 1 || result.Findings[0].Path != "calc.py" {
		t.Fatalf("result %+v, %v; want a failed finding on calc.py", result, err)
	}
}

func TestGrokPassesContextWithoutFileContents(t *testing.T) {
	testutil.FakeGrok(t, testutil.PassingReview)
	promptFile := filepath.Join(t.TempDir(), "prompt.txt")
	t.Setenv("PEEK_TEST_GROK_PROMPT", promptFile)
	repo, _ := testutil.RemoteRepo(t)
	testutil.Git(t, repo, "add", "--all")
	testutil.Git(t, repo, "commit", "-q", "-m", "Update the files")
	snapshot := reviewTestSnapshot(t, repo)
	if snapshot.Base == "" {
		t.Fatalf("want unpushed commits: %+v", snapshot)
	}
	if _, err := Grok(context.Background(), repo, snapshot, nil); err != nil {
		t.Fatal(err)
	}
	prompt, err := os.ReadFile(promptFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Scope: Unpushed commits.", "Base: " + snapshot.Base, "Head: " + snapshot.Head, "with path the repository-relative file name"} {
		if !strings.Contains(string(prompt), want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(string(prompt), "package main") {
		t.Fatalf("file contents leaked into the prompt:\n%s", prompt)
	}
}

func eventLine(t *testing.T, event any) string {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestGrokReportsProgress(t *testing.T) {
	review := "```json\n" + testutil.PassingReview + "\n```"
	events := strings.Join([]string{
		eventLine(t, map[string]any{"type": "tool_call", "title": "Read", "rawInput": map[string]string{"command": "git diff --stat"}}),
		`{"type":"tool_call_update","status":"completed"}`,
		eventLine(t, map[string]string{"type": "thought", "data": "hidden reasoning"}),
		eventLine(t, map[string]string{"type": "text", "data": review}),
		`{"type":"end","structuredOutput":null}`,
	}, "\n") + "\n"
	testutil.FakeGrokEvents(t, events)
	repo := testutil.Repo(t)
	var phases, details []string
	result, err := Grok(context.Background(), repo, reviewTestSnapshot(t, repo), func(phase, detail string) {
		phases = append(phases, phase)
		details = append(details, detail)
		if strings.Contains(detail, "hidden reasoning") {
			t.Fatal("reasoning displayed")
		}
	})
	if err != nil || result.Verdict != "passed" {
		t.Fatalf("result %+v, %v", result, err)
	}
	for _, want := range []string{"Inspecting…", "Reviewing…", "Checking…"} {
		if !slices.Contains(phases, want) {
			t.Fatalf("progress %q missing %q", phases, want)
		}
	}
	if !slices.Contains(details, "git diff --stat") {
		t.Fatalf("details %q missing the tool call's command", details)
	}
}

func TestGrokRejectsInvalidReview(t *testing.T) {
	for _, tc := range []struct{ name, response, want string }{
		{"invalid review type", `{"status":5}`, "invalid review"},
		{"not an object", `"not JSON"`, "invalid review"},
		{"incomplete finding", `{"status":"complete","summary":"Found","findings":[{"priority":2}]}`, "incomplete review finding"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.FakeGrok(t, tc.response)
			repo := testutil.Repo(t)
			if _, err := Grok(context.Background(), repo, reviewTestSnapshot(t, repo), nil); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
}

// Opt in to a tiny real CLI review; the regular tests use FakeGrok and
// need neither a login nor network access.
func TestGrokReviewSmoke(t *testing.T) {
	if os.Getenv("PEEK_GROK_SMOKE") != "1" {
		t.Skip("set PEEK_GROK_SMOKE=1 to check the installed Grok Build CLI")
	}
	repo := testutil.Repo(t)
	snapshot := reviewTestSnapshot(t, repo)
	var phases []string
	result, err := Grok(context.Background(), repo, snapshot, func(phase, detail string) {
		phases = append(phases, phase+" "+detail)
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != "passed" && result.Verdict != "failed" {
		t.Fatalf("review %+v has no verdict", result)
	}
	t.Log("Progress:", phases)
	t.Log(result.Summary)
}

func TestRunAroundGrokDetectsChanges(t *testing.T) {
	testutil.FakeGrok(t, testutil.PassingReview)
	repo := testutil.Repo(t)
	snapshot := reviewTestSnapshot(t, repo)
	run := func(ctx context.Context, repo string, snapshot Snapshot) (Result, error) {
		result, err := Grok(ctx, repo, snapshot, nil)
		if err := os.WriteFile(filepath.Join(repo, "docs/new.md"), []byte("changed during review\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return result, err
	}
	if _, err := Run(context.Background(), repo, snapshot, run); !errors.Is(err, ErrChanged) {
		t.Fatalf("accepted a result after the changes changed: %v", err)
	}
}
