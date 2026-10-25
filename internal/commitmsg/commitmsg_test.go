package commitmsg

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"peek/internal/filekind"
	"peek/internal/git"
	"peek/internal/testutil"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPromptEmbedsSmallChanges(t *testing.T) {
	large := strings.Repeat("contents read only from the repository\n", 10000)
	for _, tc := range []struct {
		name         string
		first, large bool
	}{{"small change", false, false}, {"large change", false, true}, {"first commit", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := testutil.RemoteRepo(t)
			if tc.first {
				testutil.Git(t, repo, "switch", "--orphan", "first", "--discard-changes")
			}
			files := map[string][]byte{"small.txt": []byte("a small embedded line\n"), "image.png": testutil.PNG(t), "invalid.txt": {'o', 'k', 0xff, 0xfe, '\n'}}
			if tc.large {
				files["large.txt"] = []byte(large)
			}
			for name, data := range files {
				if err := os.WriteFile(filepath.Join(repo, name), data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			testutil.Git(t, repo, "add", "--all")
			head := ""
			if !tc.first {
				head = testutil.Git(t, repo, "rev-parse", "HEAD")
			}
			snapshot := git.Snapshot{Head: head, Tree: testutil.Git(t, repo, "write-tree")}
			prompt, complete, err := Prompt(context.Background(), repo, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			data := []byte(prompt)
			if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 || bytes.Contains(data, testutil.PNG(t)) {
				t.Fatal("binary or invalid bytes in the prompt")
			}
			want := []string{"HEAD commit: " + snapshot.Head + "\n", "Staged tree: " + snapshot.Tree}
			if tc.first {
				want = append(want, "first commit")
			} else {
				want = append(want, "Recent commit subjects:\nInitial commit")
			}
			if tc.large {
				want = append(want, "Summary:\n", "large.txt", "+a small embedded line", "diff is cut here", "--ignore-submodules=none", "git show "+snapshot.Tree)
				if bytes.Count(data, []byte("contents read only from the repository")) > groupCaps[filekind.Docs]/39 {
					t.Fatal("a file's diff was embedded past its cap")
				}
			} else {
				want = append(want, "+a small embedded line", "do not run commands")
			}
			for _, w := range want {
				if !bytes.Contains(data, []byte(w)) {
					t.Fatalf("prompt missing %q:\n%s", w, prompt)
				}
			}
			if complete != !tc.large {
				t.Fatalf("complete %v, want %v", complete, !tc.large)
			}
		})
	}
}

// The staged .gitattributes decides, though it is new in this commit.
func TestPromptFollowsStagedAttributes(t *testing.T) {
	repo, _ := testutil.RemoteRepo(t)
	for name, data := range map[string]string{".gitattributes": "api/*.go linguist-generated\n", "api/client.go": "package api // marked\n", "main.go": "package main // code\n"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	testutil.Git(t, repo, "add", "--all")
	snapshot := git.Snapshot{Head: testutil.Git(t, repo, "rev-parse", "HEAD"), Tree: testutil.Git(t, repo, "write-tree")}
	prompt, _, err := Prompt(context.Background(), repo, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	bulk := strings.Index(prompt, "## Lockfiles and generated files")
	if bulk < 0 || strings.Index(prompt, "// marked") < bulk || strings.Index(prompt, "// code") > bulk {
		t.Fatalf("staged attributes not followed:\n%s", prompt)
	}
}

// Git before 2.40 rejects check-attr --source; Ship then groups by the
// common rules rather than failing.
func TestPromptWithoutCheckAttrSource(t *testing.T) {
	repo, _ := testutil.RemoteRepo(t)
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nfor a in \"$@\"; do [ \"$a\" = check-attr ] && { echo 'error: unknown option source' >&2; exit 129; }; done\nexec " + real + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main // code\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Git(t, repo, "add", "--all")
	snapshot := git.Snapshot{Head: testutil.Git(t, repo, "rev-parse", "HEAD"), Tree: testutil.Git(t, repo, "write-tree")}
	prompt, _, err := Prompt(context.Background(), repo, snapshot)
	if err != nil || !strings.Contains(prompt, "## Code\n") || !strings.Contains(prompt, "// code") {
		t.Fatalf("diffs not grouped without check-attr: %v\n%s", err, prompt)
	}
}

// One change is a subject alone; several are its body's list, wrapped.
func TestMessageText(t *testing.T) {
	if got := text(" Fix the crash ", []string{"Fix the crash"}); got != "Fix the crash" {
		t.Errorf("one change: %q", got)
	}
	if got := text("", []string{"a", "b"}); got != "" {
		t.Errorf("no subject: %q", got)
	}
	long := strings.Repeat("word ", 20)
	got := text("Rework the window", []string{"- Add a menu", long})
	want := "Rework the window\n\n- Add a menu\n- " + strings.Repeat("word ", 13) + "word\n  " + strings.TrimSpace(strings.Repeat("word ", 6)) + "\n"
	if got != want {
		t.Errorf("several changes:\n%q\nwant\n%q", got, want)
	}
	for _, line := range strings.Split(got, "\n") {
		if len(line) > 72 {
			t.Errorf("line over 72 columns: %q", line)
		}
	}
}

func TestMessageReadsAnswer(t *testing.T) {
	for _, tc := range []struct {
		answer, want string
		invalid      bool
	}{
		{answer: `{"changes":["Update the files"],"subject":"Update the files"}`, want: "Update the files"},
		{answer: `{"changes":["Add a menu","Fix the crash"],"subject":"Rework the window"}`, want: "Rework the window\n\n- Add a menu\n- Fix the crash\n"},
		{answer: `{"changes":["a","b"],"subject":""}`, want: ""},
		{answer: "not JSON", invalid: true},
	} {
		got, err := Message([]byte(tc.answer))
		if tc.invalid {
			if err == nil {
				t.Errorf("%q: accepted an invalid answer", tc.answer)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%q: message %q, %v; want %q", tc.answer, got, err, tc.want)
		}
	}
}
