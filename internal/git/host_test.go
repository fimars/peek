package git

import (
	"maps"
	"peek/internal/testutil"
	"testing"
)

// A repository's host follows its remotes: origin first, then the others,
// and self-hosted GitLab is GitLab as the CLI knows it.
func TestCodeHost(t *testing.T) {
	repo := testutil.Repo(t)
	if got := CodeHost(repo, nil); !got.None() {
		t.Errorf("host without a remote %+v", got)
	}
	testutil.Git(t, repo, "remote", "add", "upstream", "https://gitlab.com/group/sub/repo.git")
	if got, want := CodeHost(repo, nil), (Host{Kind: KindGitLab, Page: "https://gitlab.com/group/sub/repo"}); got != want {
		t.Errorf("host %+v, want %+v", got, want)
	}
	// A self-hosted instance whose name says nothing is GitLab as glab
	// knows it, and not otherwise; origin is looked at first.
	testutil.Git(t, repo, "remote", "add", "origin", "git@code.xmhhulu.com:fe/shop.git")
	if got, want := CodeHost(repo, map[string]bool{"code.xmhhulu.com": true}),
		(Host{Kind: KindGitLab, Page: "https://code.xmhhulu.com/fe/shop"}); got != want {
		t.Errorf("host as glab knows it %+v, want %+v", got, want)
	}
	testutil.Git(t, repo, "remote", "remove", "upstream")
	if got := CodeHost(repo, nil); !got.None() {
		t.Errorf("host from an unknown name %+v", got)
	}
}

// A host's pages are each what its site serves.
func TestHostPages(t *testing.T) {
	github := Host{Kind: KindGitHub, Page: "https://github.com/o/r"}
	gitlab := Host{Kind: KindGitLab, Page: "https://gitlab.com/g/s/r"}
	for _, tc := range []struct {
		name      string
		got, want string
	}{
		{"GitHub commit", github.CommitURL("abc"), "https://github.com/o/r/commit/abc"},
		{"GitLab commit", gitlab.CommitURL("abc"), "https://gitlab.com/g/s/r/-/commit/abc"},
		{"GitHub compare", github.CompareURL("a", "b"), "https://github.com/o/r/compare/a...b"},
		{"GitLab compare", gitlab.CompareURL("a", "b"), "https://gitlab.com/g/s/r/-/compare/a...b"},
		{"GitHub file", github.PathURL("abc", "dir/a.go", false), "https://github.com/o/r/blob/abc/dir/a.go"},
		{"GitLab file", gitlab.PathURL("abc", "dir/a.go", false), "https://gitlab.com/g/s/r/-/blob/abc/dir/a.go"},
		{"GitLab folder", gitlab.PathURL("abc", "dir", true), "https://gitlab.com/g/s/r/-/tree/abc/dir"},
		{"GitHub pull request", github.ChangeRequestURL(7), "https://github.com/o/r/pull/7"},
		{"GitLab merge request", gitlab.ChangeRequestURL(7), "https://gitlab.com/g/s/r/-/merge_requests/7"},
		{"GitHub issue", github.IssueURL(7), "https://github.com/o/r/issues/7"},
		{"GitLab issue", gitlab.IssueURL(7), "https://gitlab.com/g/s/r/-/issues/7"},
		{"GitHub branch", github.BranchURL("feat/a#b"), "https://github.com/o/r/tree/feat/a%23b"},
		{"GitLab branch", gitlab.BranchURL("feat/a#b"), "https://gitlab.com/g/s/r/-/tree/feat/a%23b"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	// A host's names for a change request are each its own.
	if got := github.ChangeRequest(7); got != "#7" {
		t.Errorf("GitHub's change request %q", got)
	}
	if got := gitlab.ChangeRequest(7); got != "!7" {
		t.Errorf("GitLab's change request %q", got)
	}
	if !(Host{}).None() {
		t.Error("no host is not none")
	}
	if got := (Host{Kind: KindGitHub}).None(); !got {
		t.Error("a host without a page is not none")
	}
}

// A merge request's page is read as GitLab writes it, and as the query
// names it by number.
func TestParseMergeRequest(t *testing.T) {
	const page = "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha"
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"73", 73},
		{" !73 ", 73},
		{"https://gitlab.xmhhulu.com/fe/shopify-apps-alpha/-/merge_requests/73", 73},
		{"https://gitlab.xmhhulu.com/fe/shopify-apps-alpha/merge_requests/73", 73},
		{"https://gitlab.xmhhulu.com/fe/shopify-apps-alpha/-/merge_requests/73/diffs", 73},
		{"https://gitlab.xmhhulu.com/other/project/-/merge_requests/73", 0},
		{"main", 0},
		{"0", 0},
		{"", 0},
	} {
		n, ok := ParseMergeRequest(tc.in, page)
		if n != tc.want || ok != (tc.want > 0) {
			t.Errorf("ParseMergeRequest(%q) = %d, %v; want %d", tc.in, n, ok, tc.want)
		}
	}
}

// glab's sign-in status nests its hosts, of which only those it reached
// count.
func TestParseGlabHosts(t *testing.T) {
	data := []byte(`{"hosts":[{"host":"GitLab.Xmhhulu.com","state":"success"},{"host":"gitlab.com","state":"error"}]}`)
	got := parseGlabHosts(data)
	if !maps.Equal(got, map[string]bool{"gitlab.xmhhulu.com": true}) {
		t.Errorf("hosts %v", got)
	}
	if got := parseGlabHosts([]byte("not json")); got != nil {
		t.Errorf("hosts of what is not JSON %v", got)
	}
}

// A merge request's ref is Peek's own, and its remote's is GitLab's.
func TestMergeRequestRefs(t *testing.T) {
	if got := MergeRequestRef(73); got != "refs/peek/merge-requests/73" {
		t.Errorf("local ref %q", got)
	}
	if got := MergeRequestRemoteRef(73); got != "refs/merge-requests/73/head" {
		t.Errorf("remote ref %q", got)
	}
}

// A merge request's fetch brings its head and its target branch, and its
// base is where the two left off.
func TestFetchMergeRequest(t *testing.T) {
	repo, remote := testutil.RemoteRepo(t)
	head, base := testutil.MergeRequest(t, repo, remote, "feature/shop", 73)
	gotBase, gotHead, err := FetchMergeRequest(t.Context(), repo, "origin", MergeRequest{Number: 73, Target: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if gotBase != base || gotHead != head {
		t.Errorf("fetched %s..%s, want %s..%s", gotBase, gotHead, base, head)
	}
	if now := testutil.Git(t, repo, "rev-parse", MergeRequestRef(73)); now != head {
		t.Errorf("the local ref is %s, want %s", now, head)
	}
	// The changes are the merge request's alone, the branch's own left
	// out.
	changes, err := LoadChanges(repo, Range(base, head))
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Path != "mr.txt" || changes[0].Added != 1 {
		t.Errorf("changes %+v", changes)
	}
}

// A merge request whose target branch is not fetched says so, rather than
// comparing with another branch.
func TestFetchMergeRequestWithoutTarget(t *testing.T) {
	repo, remote := testutil.RemoteRepo(t)
	testutil.MergeRequest(t, repo, remote, "feature/shop", 73)
	// A target branch the remote does not have.
	if _, _, err := FetchMergeRequest(t.Context(), repo, "origin", MergeRequest{Number: 73, Target: "release"}); err == nil {
		t.Fatal("no error without the target branch on the remote")
	}
}

// A merge request's page names its instance, whatever the instance is,
// and its project and number.
func TestParseMergeRequestLink(t *testing.T) {
	link, ok := ParseMergeRequestLink("https://gitlab.xmhhulu.com/fe/shopify-apps-alpha/-/merge_requests/73")
	if !ok {
		t.Fatal("the page was not read")
	}
	if link.Host != (Host{Kind: KindGitLab, Page: "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha"}) {
		t.Errorf("host %+v", link.Host)
	}
	if link.Project != "fe/shopify-apps-alpha" || link.Number != 73 {
		t.Errorf("link %+v", link)
	}
	if got := link.Host.Instance(); got != "gitlab.xmhhulu.com" {
		t.Errorf("instance %q", got)
	}
	// The page without -/, a nested group, and a tab of it.
	if link, ok := ParseMergeRequestLink("https://gitlab.com/a/b/c/merge_requests/4/diffs"); !ok || link.Number != 4 || link.Project != "a/b/c" {
		t.Errorf("link %+v, %v", link, ok)
	}
	if link, ok := ParseMergeRequestLink("https://code.example.com/g/r/-/merge_requests/9"); !ok || link.Host.Instance() != "code.example.com" {
		t.Errorf("self-hosted link %+v, %v", link, ok)
	}
	for _, s := range []string{
		"https://github.com/o/r/pull/7", // GitHub's own shape
		"https://gitlab.com/a/b/-/merge_requests/",
		"https://gitlab.com/a/b/-/merge_requests/0",
		"main",
		"!7",
		"",
	} {
		if link, ok := ParseMergeRequestLink(s); ok {
			t.Errorf("ParseMergeRequestLink(%q) = %+v, want none", s, link)
		}
	}
}
