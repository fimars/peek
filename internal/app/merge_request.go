package app

import (
	"context"
	"errors"
	"peek/internal/git"
	"peek/internal/review"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
)

// mergeRequest is a GitLab merge request whose changes the app shows: its
// head fetched to a ref of its own, and its base the commit it left its
// target branch at, so that its changes are a range of the two, as the
// branch's own are.
type mergeRequest struct {
	number       int
	title        string
	branch, into string
	url          string // its page on GitLab
	base, head   string // the commits its changes go from and to, "" until fetched
	loading      bool
	err          string // why its changes do not show
}

// mergeRequestInfo is what a read of the history needs of the merge
// request shown, taken off the UI thread.
type mergeRequestInfo struct {
	number       int
	title        string
	branch, into string
	base, head   string
}

// showMergeRequest fetches a merge request's head and the branch it merges
// into, then shows its changes in the history, as choosing it in the
// switcher asks. A row that names a merge request the list did not have,
// as one pasted from the browser, is looked up first.
func (a *app) showMergeRequest(mr git.MergeRequest) {
	if a.push.running || a.review.running || a.pull.running {
		return
	}
	a.mergeRequest = &mergeRequest{number: mr.Number, title: mr.Title, branch: mr.Branch, into: mr.Target,
		url: a.host.ChangeRequestURL(mr.Number), loading: true}
	a.refresh()
	a.fetchMergeRequest(mr)
}

// showMergeRequestLink shows the merge request a page names, of the
// instance it is on: the user names the instance by pasting its page,
// where the repository's remotes do not tell it.
func (a *app) showMergeRequestLink(link git.MergeRequestLink) {
	if a.push.running || a.review.running || a.pull.running {
		return
	}
	a.setInstance(link.Host.Instance(), link.Host.Page)
	a.mergeRequest = &mergeRequest{number: link.Number, url: link.Host.ChangeRequestURL(link.Number), loading: true}
	a.refresh()
	a.fetchMergeRequest(git.MergeRequest{Number: link.Number})
}

// fetchMergeRequest fetches the merge request's head and target branch for
// the one shown, off the UI thread.
func (a *app) fetchMergeRequest(mr git.MergeRequest) {
	shown := a.mergeRequest
	repo, remote, page := a.repo, a.mergeRequestRemote(), a.host.Page
	if remote == "" {
		shown.loading, shown.err = false, "the repository has no remote to fetch the merge request from"
		a.refresh()
		return
	}
	a.background(func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), fetchMergeRequestTimeout)
		defer cancel()
		if mr.Target == "" {
			// Named but not listed: the host has its branches.
			found, err := findMergeRequest(ctx, repo, page, mr.Number)
			if err != nil {
				return failed(a, shown, err)
			}
			mr = found
		}
		base, head, err := git.FetchMergeRequest(ctx, repo, remote, mr)
		if err != nil {
			return failed(a, shown, err)
		}
		return func() {
			if a.mergeRequest != shown {
				return // another was chosen meanwhile
			}
			shown.title, shown.branch, shown.into = mr.Title, mr.Branch, mr.Target
			shown.loading, shown.base, shown.head = false, base, head
			a.landOnMergeRequest = true
			a.refresh()
		}
	})
}

// failed publishes why the merge request's changes do not come, for the
// history's bar to say, where it still shows.
func failed(a *app, shown *mergeRequest, err error) func() {
	return func() {
		if a.mergeRequest != shown {
			return
		}
		shown.loading, shown.err = false, err.Error()
		a.refresh()
	}
}

// findMergeRequest looks a merge request up among the project's open ones.
func findMergeRequest(ctx context.Context, repo, page string, number int) (git.MergeRequest, error) {
	mrs, err := git.ListMergeRequests(ctx, repo, page)
	if err != nil {
		return git.MergeRequest{}, err
	}
	for _, mr := range mrs {
		if mr.Number == number {
			return mr, nil
		}
	}
	return git.MergeRequest{}, errors.New("merge request !" + strconv.Itoa(number) + " is not open")
}

// mergeRequestRow is the history's row of the merge request's changes,
// -1 where it has none yet.
func (a *app) mergeRequestRow() int {
	mr := a.mergeRequest
	if mr == nil || mr.base == "" || mr.head == "" {
		return -1
	}
	return a.rowOf(git.Range(mr.base, mr.head))
}

// closeMergeRequest shows the branch's own history again.
func (a *app) closeMergeRequest() {
	if a.mergeRequest == nil {
		return
	}
	a.mergeRequest = nil
	a.landOnMergeRequest = false
	a.commitRow, a.loadedRow = 0, -1
	a.refresh()
}

// mergeRequestForRead is the merge request as a read of the history takes
// it, nil where none shows or it has not been fetched.
func (a *app) mergeRequestForRead() *mergeRequestInfo {
	mr := a.mergeRequest
	if mr == nil || mr.loading || mr.err != "" || mr.base == "" || mr.head == "" {
		return nil
	}
	return &mergeRequestInfo{number: mr.number, title: mr.title, branch: mr.branch, into: mr.into, base: mr.base, head: mr.head}
}

// mergeRequestNote is why the merge request's changes do not show, ""
// where they do or none shows.
func (a *app) mergeRequestNote() string {
	mr := a.mergeRequest
	switch {
	case mr == nil:
		return ""
	case mr.loading:
		return "Fetching merge request " + a.host.ChangeRequest(mr.number) + "…"
	case mr.err != "":
		return "Couldn’t Fetch Merge Request " + a.host.ChangeRequest(mr.number) + ": " + mr.err
	}
	return ""
}

// mergeRequestSnapshot is the review of the merge request's changes alone;
// a zero Snapshot and the reason where they do not show.
func (a *app) mergeRequestSnapshot() (review.Snapshot, string) {
	mr := a.mergeRequest
	if mr == nil {
		return review.Snapshot{}, ""
	}
	if note := a.mergeRequestNote(); note != "" {
		return review.Snapshot{}, note
	}
	snapshot, err := review.MergeRequestSnapshot(a.repo, "Merge request "+a.host.ChangeRequest(mr.number), mr.base, mr.head)
	if err != nil {
		return review.Snapshot{}, err.Error()
	}
	return snapshot, ""
}

// mergeRequestBar is the bar over the history that says which merge
// request's changes show, and closes them for the branch's own, as
// Xcode's jump bar names what its editor shows.
func (a *app) mergeRequestBar(c *ui.Context) {
	t := c.Theme()
	mr := a.mergeRequest
	services := c.Services()
	ui.Row(c).Shrink(0).FillWidth().Gap(t.Space(1.5)).AlignItems(ui.Center).
		Padding(t.Space(1), t.Space(3), t.Space(1), t.Space(3)).Background(t.Surface).Children(func() {
		if note := a.mergeRequestNote(); note != "" {
			if mr.loading {
				ui.Spinner(c).Size(12, 12).Label("Fetching")
			}
			ui.Text(c, note).FontSize(smallSize).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Grow(1).MinWidth(0).Tooltip(note)
			if mr.err == "" {
				return
			}
		} else {
			ui.Icon(c, pullIcon).Size(12, 12).Shrink(0).TextColor(t.Accent)
			ui.Text(c, a.host.ChangeRequest(mr.number)).FontSize(smallSize).FontWeight(600).TextColor(t.Accent).
				SingleLine().Shrink(0).Tooltip(mr.title)
			ui.Text(c, mr.title).FontSize(smallSize).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		}
		if mr.url != "" {
			ui.Button(c.Key("merge-request-web"), "").Shrink(0).Label("Open on " + a.host.Name()).
				Tooltip("Open " + a.host.ChangeRequest(mr.number) + " on " + a.host.Name()).
				Children(func() { ui.Icon(c, hostIcon(a.host.Kind)).Size(13, 13) }).
				OnClick(func() { services.OpenURL(mr.url) })
		}
		ui.Button(c.Key("merge-request-close"), "").Shrink(0).Label("Close Merge Request").
			Tooltip("Show the branch’s own history").
			Children(func() { ui.Icon(c, clearIcon).Size(10, 10) }).OnClick(a.closeMergeRequest)
	})
}

// setInstance names the GitLab instance the repository's merge requests
// are on, for this session and the project's next ones.
func (a *app) setInstance(instance, page string) {
	a.instance = instance
	if a.projects != nil {
		a.projects.setInstance(a.repo, instance, page)
	}
}

// mergeRequestRemote is the remote a merge request is fetched from: the
// one the branch tracks, else origin, else the sole remote.
func (a *app) mergeRequestRemote() string {
	if remote := git.TrackedRemote(a.repo); remote != "" {
		return remote
	}
	return git.PushRemote(a.repo)
}

// fetchMergeRequestTimeout bounds a fetch: a large merge request on a slow
// connection still comes, and a stuck one gives the app back.
const fetchMergeRequestTimeout = 5 * time.Minute
