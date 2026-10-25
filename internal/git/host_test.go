package git

import (
	"peek/internal/testutil"
	"testing"
)

func TestCodeHost(t *testing.T) {
	gitlab := Host{Kind: KindGitLab, Page: "https://gitlab.com/hyoban/peek"}
	for _, tc := range []struct {
		name    string
		remotes [][2]string
		want    Host
	}{
		{"no remote", nil, Host{}},
		{"origin GitHub", [][2]string{{"origin", "git@github.com:hyoban/peek.git"}}, Host{Kind: KindGitHub, Page: "https://github.com/hyoban/peek"}},
		{"origin GitLab", [][2]string{{"origin", "ssh://git@gitlab.com/hyoban/peek.git"}}, gitlab},
		{"upstream fallback", [][2]string{{"origin", "https://example.com/a/b.git"}, {"upstream", "https://gitlab.com/hyoban/peek.git"}}, gitlab},
		{"unknown host", [][2]string{{"origin", "https://example.com/hyoban/peek.git"}}, Host{}},
		{"nested GitLab group", [][2]string{{"origin", "https://gitlab.com/group/sub/peek.git"}}, Host{Kind: KindGitLab, Page: "https://gitlab.com/group/sub/peek"}},
		{"GitLab with port", [][2]string{{"origin", "https://gitlab.example.com:8443/group/repo.git"}}, Host{Kind: KindGitLab, Page: "https://gitlab.example.com:8443/group/repo"}},
		{"GitHub nested path", [][2]string{{"origin", "https://github.com/group/sub/peek.git"}}, Host{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := testutil.Repo(t)
			for _, remote := range tc.remotes {
				testutil.Git(t, repo, "remote", "add", remote[0], remote[1])
			}
			if got := CodeHost(repo); got != tc.want {
				t.Errorf("CodeHost = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestHostPages(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		host                             Host
		commit, file, dir, issue, branch string
	}{
		{"GitHub",
			Host{Kind: KindGitHub, Page: "https://github.com/hyoban/peek"},
			"https://github.com/hyoban/peek/commit/abc",
			"https://github.com/hyoban/peek/blob/abc/docs/new.md",
			"https://github.com/hyoban/peek/tree/abc/docs",
			"https://github.com/hyoban/peek/issues/12",
			"https://github.com/hyoban/peek/tree/feat/a%23b"},
		{"GitLab",
			Host{Kind: KindGitLab, Page: "https://gitlab.com/group/sub/peek"},
			"https://gitlab.com/group/sub/peek/-/commit/abc",
			"https://gitlab.com/group/sub/peek/-/blob/abc/docs/new.md",
			"https://gitlab.com/group/sub/peek/-/tree/abc/docs",
			"https://gitlab.com/group/sub/peek/-/issues/12",
			"https://gitlab.com/group/sub/peek/-/tree/feat/a%23b"},
		{"none", Host{}, "", "", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := tc.host
			if got := h.CommitURL("abc"); got != tc.commit {
				t.Errorf("CommitURL = %q, want %q", got, tc.commit)
			}
			if got := h.PathURL("abc", "docs/new.md", false); got != tc.file {
				t.Errorf("PathURL file = %q, want %q", got, tc.file)
			}
			if got := h.PathURL("abc", "docs", true); got != tc.dir {
				t.Errorf("PathURL dir = %q, want %q", got, tc.dir)
			}
			if got := h.IssueURL(12); got != tc.issue {
				t.Errorf("IssueURL = %q, want %q", got, tc.issue)
			}
			if got := h.BranchURL("feat/a#b"); got != tc.branch {
				t.Errorf("BranchURL = %q, want %q", got, tc.branch)
			}
		})
	}
}

func TestHostKind(t *testing.T) {
	for kind, want := range map[Kind]string{KindNone: "", KindGitHub: "GitHub", KindGitLab: "GitLab"} {
		if got := (Host{Kind: kind}).Name(); got != want {
			t.Errorf("Kind(%d).Name() = %q, want %q", kind, got, want)
		}
		if got := (Host{Kind: kind}).None(); got != (kind == KindNone) {
			t.Errorf("Kind(%d).None() = %v", kind, got)
		}
	}
}
