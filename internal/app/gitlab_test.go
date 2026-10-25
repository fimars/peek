package app

import (
	"peek/internal/git"
	"peek/internal/testutil"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// A repository on a self-hosted GitLab is on GitLab as glab knows it: its
// pages and its merge requests are GitLab's, and !names a merge request.
func TestGitLabHost(t *testing.T) {
	repo := testutil.Repo(t)
	testutil.Git(t, repo, "remote", "add", "origin", "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha.git")
	testutil.FakeGlab(t, "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha", 73, "feature/shop")
	a, tt := testApp(t, repo, nil)
	settle(tt)
	want := git.Host{Kind: git.KindGitLab, Page: "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha"}
	if a.host != want {
		t.Fatalf("host %+v, want %+v", a.host, want)
	}
	if !tt.HasText("Open on GitLab") {
		t.Errorf("no GitLab link, texts %q", tt.Texts())
	}
	if got := a.host.ChangeRequestURL(73); got != "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha/-/merge_requests/73" {
		t.Errorf("merge request's page %q", got)
	}
}

// The host is GitLab's without the CLI too, where its name says so.
func TestGitLabHostByName(t *testing.T) {
	repo := testutil.Repo(t)
	testutil.Git(t, repo, "remote", "add", "origin", "https://gitlab.example.com/group/sub/repo.git")
	a, tt := testApp(t, repo, nil)
	settle(tt)
	if a.host.Kind != git.KindGitLab || a.host.Page != "https://gitlab.example.com/group/sub/repo" {
		t.Fatalf("host %+v", a.host)
	}
}

// The switcher lists a GitLab project's merge requests, and choosing one
// shows its changes without checking out a branch.
func TestSwitcherMergeRequest(t *testing.T) {
	repo, remote := testutil.RemoteRepo(t)
	testutil.Git(t, repo, "remote", "set-url", "origin", "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha.git")
	testutil.Git(t, repo, "config", "url."+remote+".insteadOf", "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha.git")
	head, base := testutil.MergeRequest(t, repo, remote, "feature/shop", 73)
	testutil.FakeGlab(t, "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha", 73, "feature/shop")
	a, tt := testApp(t, repo, nil)
	settle(tt)
	a.openSwitcher()
	tt.Frame()
	if !tt.HasText("Merge Requests") || !tt.HasText("!73 Add the merge request") {
		t.Fatalf("no merge request in %q", tt.Texts())
	}
	if err := tt.Click("!73 Add the merge request"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	mr := a.mergeRequest
	if mr == nil || mr.err != "" {
		t.Fatalf("no merge request shown: %+v", mr)
	}
	if mr.base != base || mr.head != head {
		t.Errorf("range %s..%s, want %s..%s", mr.base, mr.head, base, head)
	}
	// Its changes are the history's entry, as the branch's would be.
	entry := git.Range(base, head)
	found := false
	for _, cm := range a.commits {
		if cm.Hash == entry {
			found = true
			if !strings.Contains(cm.Subject, "!73") {
				t.Errorf("entry %q, want the merge request's", cm.Subject)
			}
		}
	}
	if !found {
		t.Errorf("no %s in the history %v", entry, a.commits)
	}
	// The branch is where it was: nothing is checked out.
	if a.branch != "main" {
		t.Errorf("branch %q, want main", a.branch)
	}
	// Review reads the merge request's own range.
	snapshot, note := a.mergeRequestSnapshot()
	if note != "" || !snapshot.Range || snapshot.Base != base || snapshot.Head != head {
		t.Errorf("review of %+v: %+v", snapshot, note)
	}
	a.closeMergeRequest()
	settle(tt)
	if a.mergeRequest != nil {
		t.Fatal("the merge request still shows")
	}
	for _, cm := range a.commits {
		if cm.Hash == entry {
			t.Error("the merge request's changes still in the history")
		}
	}
}

// A merge request's changes are the diff it makes, as a range, without
// the branch's own commits.
func TestMergeRequestChanges(t *testing.T) {
	repo, remote := testutil.RemoteRepo(t)
	testutil.Git(t, repo, "remote", "set-url", "origin", "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha.git")
	testutil.Git(t, repo, "config", "url."+remote+".insteadOf", "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha.git")
	head, base := testutil.MergeRequest(t, repo, remote, "feature/shop", 73)
	testutil.FakeGlab(t, "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha", 73, "feature/shop")
	a, tt := testApp(t, repo, nil)
	settle(tt)
	a.showMergeRequest(git.MergeRequest{Number: 73, Title: "Add shop", Branch: "feature/shop", Target: "main"})
	settle(tt)
	// Its files show with the work tree's, as the diff from base to head.
	a.commitRow = a.rowOf(git.Range(base, head))
	a.loadedRow = -1
	settle(tt)
	if _, ok := a.changes["mr.txt"]; !ok {
		t.Fatalf("no mr.txt in %v", a.changes)
	}
	d := a.diffOf["mr.txt"]
	if d == nil || d.change.Added != 1 {
		t.Fatalf("mr.txt's diff %+v", d)
	}
	if a.changes["main.go"].Status != "" {
		t.Errorf("the branch's own changes show: %v", a.changes)
	}
	_ = head
}

// Review follows what shows: the merge request's own range while it does,
// and the uncommitted changes of the branch's again once it closes.
func TestReviewFollowsMergeRequest(t *testing.T) {
	repo, remote := testutil.RemoteRepo(t)
	testutil.Git(t, repo, "remote", "set-url", "origin", "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha.git")
	testutil.Git(t, repo, "config", "url."+remote+".insteadOf", "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha.git")
	head, base := testutil.MergeRequest(t, repo, remote, "feature/shop", 73)
	testutil.FakeGlab(t, "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha", 73, "feature/shop")
	a, tt := testApp(t, repo, nil)
	settle(tt)
	if a.reviewSource.Scope != "Uncommitted changes" {
		t.Fatalf("review of %q before the merge request", a.reviewSource.Scope)
	}
	a.showMergeRequest(git.MergeRequest{Number: 73, Title: "Add shopify app", Branch: "feature/shop", Target: "main"})
	settle(tt)
	s := a.reviewSource
	if !s.Range || s.Base != base || s.Head != head || s.Scope != "Merge request !73" {
		t.Errorf("review of %+v, want %s..%s", s, base, head)
	}
	a.closeMergeRequest()
	settle(tt)
	if a.reviewSource.Range || a.reviewSource.Scope != "Uncommitted changes" {
		t.Errorf("review of %+v after closing", a.reviewSource)
	}
}

// A merge request's fetch is the target branch's too, for its base.
func TestFetchMergeRequest(t *testing.T) {
	repo, remote := testutil.RemoteRepo(t)
	testutil.Git(t, repo, "remote", "set-url", "origin", "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha.git")
	testutil.Git(t, repo, "config", "url."+remote+".insteadOf", "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha.git")
	head, base := testutil.MergeRequest(t, repo, remote, "feature/shop", 73)
	gotBase, gotHead, err := git.FetchMergeRequest(t.Context(), repo, "origin",
		git.MergeRequest{Number: 73, Target: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if gotBase != base || gotHead != head {
		t.Errorf("fetched %s..%s, want %s..%s", gotBase, gotHead, base, head)
	}
}

// A named merge request, pasted from the browser, is looked up among the
// open ones.
func TestNamedMergeRequest(t *testing.T) {
	repo, remote := testutil.RemoteRepo(t)
	testutil.Git(t, repo, "remote", "set-url", "origin", "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha.git")
	testutil.Git(t, repo, "config", "url."+remote+".insteadOf", "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha.git")
	testutil.MergeRequest(t, repo, remote, "feature/shop", 73)
	testutil.FakeGlab(t, "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha", 73, "feature/shop")
	a, tt := testApp(t, repo, nil)
	settle(tt)
	a.openSwitcher()
	tt.Frame()
	a.switcher.query = "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha/-/merge_requests/73"
	rows, _ := a.rows()
	if len(rows) == 0 || rows[0].target.mr != 73 {
		t.Fatalf("rows for the page: %+v", rows)
	}
	a.switcher.query = "!73"
	rows, _ = a.rows()
	if len(rows) == 0 || rows[0].target.mr != 73 {
		t.Fatalf("rows for !73: %+v", rows)
	}
	// A merge request the list does not have is looked up by number.
	a.switcher.query = "!99"
	rows, _ = a.rows()
	if len(rows) == 0 || rows[0].target.mr != 99 {
		t.Fatalf("rows for !99: %+v", rows)
	}
}

// The file and line menus point at the host's pages, each as it writes
// them.
func TestHostMenus(t *testing.T) {
	repo := testutil.Repo(t)
	testutil.Git(t, repo, "remote", "add", "origin", "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha.git")
	a, tt := testApp(t, repo, nil)
	settle(tt)
	sha := a.commits[1].Hash
	if got := a.host.PathURL(sha, "src/a.go", false); got != "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha/-/blob/"+sha+"/src/a.go" {
		t.Errorf("file's page %q", got)
	}
	if got := a.host.PathURL(sha, "src", true); got != "https://gitlab.xmhhulu.com/fe/shopify-apps-alpha/-/tree/"+sha+"/src" {
		t.Errorf("folder's page %q", got)
	}
	if got := a.host.BranchURL("feat/a#b"); !strings.Contains(got, "feat/a%23b") {
		t.Errorf("branch's page %q", got)
	}
	_ = tt
}

// A pasted merge request's page names the instance Peek would not find by
// itself: it is GitLab from then on, and the project's path is the
// remote's.
func TestMergeRequestLinkNamesInstance(t *testing.T) {
	repo, remote := testutil.RemoteRepo(t)
	// A remote whose name says nothing of GitLab, as a self-hosted one's
	// may.
	testutil.Git(t, repo, "remote", "set-url", "origin", "git@code.xmhhulu.com:fe/shopify-apps-alpha.git")
	testutil.Git(t, repo, "config", "url."+remote+".insteadOf", "git@code.xmhhulu.com:fe/shopify-apps-alpha.git")
	testutil.MergeRequest(t, repo, remote, "feature/shop", 73)
	testutil.FakeGlab(t, "https://code.xmhhulu.com/fe/shopify-apps-alpha", 73, "feature/shop")
	a, tt := testApp(t, repo, nil)
	settle(tt)
	if !a.host.None() {
		t.Fatalf("host %+v before the page names it", a.host)
	}
	a.openSwitcher()
	tt.Frame()
	// The instance field shows the repository's, such as it is.
	if !tt.HasText("GitLab") {
		t.Errorf("no instance field in %q", tt.Texts())
	}
	a.switcher.query = "https://code.xmhhulu.com/fe/shopify-apps-alpha/-/merge_requests/73"
	tt.Frame()
	rows, _ := a.rows()
	if len(rows) != 1 || rows[0].target.mr != 73 {
		t.Fatalf("rows %+v", rows)
	}
	if a.switcher.link == nil || a.switcher.link.Host.Instance() != "code.xmhhulu.com" {
		t.Fatalf("link %+v", a.switcher.link)
	}
	a.submitSwitch()
	settle(tt)
	if a.instance != "code.xmhhulu.com" {
		t.Errorf("instance %q", a.instance)
	}
	want := git.Host{Kind: git.KindGitLab, Page: "https://code.xmhhulu.com/fe/shopify-apps-alpha"}
	if a.host != want {
		t.Errorf("host %+v, want %+v", a.host, want)
	}
	if a.mergeRequest == nil || a.mergeRequest.err != "" || a.mergeRequest.base == "" {
		t.Fatalf("merge request %+v", a.mergeRequest)
	}
}

// The instance is the user's to type, for a repository whose remotes do
// not tell it, and it is kept for the project's next opens.
func TestInstanceField(t *testing.T) {
	repo, _ := testutil.RemoteRepo(t)
	testutil.Git(t, repo, "remote", "set-url", "origin", "git@code.xmhhulu.com:fe/shopify-apps-alpha.git")
	testutil.FakeGlab(t, "https://code.xmhhulu.com/fe/shopify-apps-alpha", 73, "feature/shop")
	dir := t.TempDir()
	p := &projects{avatars: newAvatars(), apps: map[string]*app{}, dir: dir, instances: map[string]instancePage{}}
	a := newApp(repo)
	a.inline, a.projects = true, p
	a.loadCommits()
	tt := ui.NewTester(a.view, 1200, 800)
	settle(tt)
	a.openSwitcher()
	tt.Frame()
	if err := tt.Click("GitLab Instance"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// As the user types it, with the scheme and a trailing slash too.
	*a.switcher.instance = "https://code.xmhhulu.com/"
	a.applyInstance()
	settle(tt)
	if a.instance != "code.xmhhulu.com" {
		t.Fatalf("instance %q", a.instance)
	}
	if want := "https://code.xmhhulu.com/fe/shopify-apps-alpha"; a.host.Page != want {
		t.Errorf("page %q, want %q", a.host.Page, want)
	}
	// Kept for the next launch, by repository.
	if got := p.readInstances()[repo]; got.instance != "code.xmhhulu.com" || got.page != a.host.Page {
		t.Errorf("kept %+v", got)
	}
}
