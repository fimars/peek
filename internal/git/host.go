package git

import (
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	neturl "net/url"
	"strconv"
	"strings"
)

// Kind is the kind of code host a remote is on: the ones whose pages and
// change requests Peek knows.
type Kind int

const (
	KindNone Kind = iota
	KindGitHub
	KindGitLab
)

// Host is a code host a remote is on: its kind and the repository's page
// there, without a trailing slash.
type Host struct {
	Kind Kind
	Page string
}

// None reports whether there is no host to link to, as a repository whose
// remotes are on no known host.
func (h Host) None() bool { return h.Kind == KindNone || h.Page == "" }

// Name names the host as the menus do, "" for none: GitHub or GitLab.
func (h Host) Name() string {
	switch h.Kind {
	case KindGitHub:
		return "GitHub"
	case KindGitLab:
		return "GitLab"
	}
	return ""
}

// ChangeRequest names a pull request or a merge request as its host does:
// #12 on GitHub, !12 on GitLab.
func (h Host) ChangeRequest(number int) string {
	if h.Kind == KindGitLab {
		return "!" + strconv.Itoa(number)
	}
	return "#" + strconv.Itoa(number)
}

// CommitURL is the page of a commit, "" where there is none.
func (h Host) CommitURL(sha string) string {
	if h.None() || sha == "" {
		return ""
	}
	if h.Kind == KindGitLab {
		return h.Page + "/-/commit/" + sha
	}
	return h.Page + "/commit/" + sha
}

// CompareURL is the page comparing two commits, as a range's changes show.
func (h Host) CompareURL(base, head string) string {
	if h.None() {
		return ""
	}
	if h.Kind == KindGitLab {
		return h.Page + "/-/compare/" + base + "..." + head
	}
	return h.Page + "/compare/" + base + "..." + head
}

// PathURL is the page of a file or a folder at a commit.
func (h Host) PathURL(sha, p string, dir bool) string {
	if h.None() {
		return ""
	}
	kind := "blob"
	if dir {
		kind = "tree"
	}
	if h.Kind == KindGitLab {
		return h.Page + "/-/" + kind + "/" + sha + "/" + p
	}
	return h.Page + "/" + kind + "/" + sha + "/" + p
}

// ChangeRequestURL is the page of a pull request or a merge request.
func (h Host) ChangeRequestURL(number int) string {
	if h.None() {
		return ""
	}
	if h.Kind == KindGitLab {
		return h.Page + "/-/merge_requests/" + strconv.Itoa(number)
	}
	return h.Page + "/pull/" + strconv.Itoa(number)
}

// IssueURL is the page of an issue, where a message's #123 refers to one.
func (h Host) IssueURL(number int) string {
	if h.None() {
		return ""
	}
	if h.Kind == KindGitLab {
		return h.Page + "/-/issues/" + strconv.Itoa(number)
	}
	return h.Page + "/issues/" + strconv.Itoa(number)
}

// BranchURL is the page of a branch.
func (h Host) BranchURL(branch string) string {
	if h.None() || branch == "" {
		return ""
	}
	parts := strings.Split(branch, "/")
	for i, p := range parts {
		parts[i] = neturl.PathEscape(p)
	}
	escaped := strings.Join(parts, "/")
	if h.Kind == KindGitLab {
		return h.Page + "/-/tree/" + escaped
	}
	return h.Page + "/tree/" + escaped
}

// GitURL is the remote URL of the repository on its host, which git can
// fetch from.
func (h Host) GitURL() string {
	if h.None() {
		return ""
	}
	return h.Page + ".git"
}

// MergeRequestLink is a merge request's page: the instance it is on, the
// project's path there, and its number.
type MergeRequestLink struct {
	Host    Host
	Project string // group/subgroup/name on the instance
	Number  int
}

// ParseMergeRequestLink reads a merge request's page on GitLab, of any
// instance, self-hosted ones whose names say nothing included: the page's
// shape, /-/merge_requests/12, tells GitLab's, as GitHub's /pull/12 tells
// its own. It is how the user names the instance Peek would not find by
// itself.
func ParseMergeRequestLink(s string) (MergeRequestLink, bool) {
	u, err := neturl.Parse(strings.TrimSpace(s))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return MergeRequestLink{}, false
	}
	p := strings.Trim(u.Path, "/")
	at, sep := strings.Index(p, "/-/merge_requests/"), "/-/merge_requests/"
	if at < 0 {
		at, sep = strings.Index(p, "/merge_requests/"), "/merge_requests/"
	}
	if at < 0 {
		return MergeRequestLink{}, false
	}
	project, rest := p[:at], p[at+len(sep):]
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i] // /diffs, /pipelines and the like
	}
	n, err := strconv.Atoi(rest)
	if project == "" || err != nil || n <= 0 {
		return MergeRequestLink{}, false
	}
	host := strings.ToLower(strings.TrimPrefix(u.Host, "www."))
	return MergeRequestLink{Host: Host{Kind: KindGitLab, Page: "https://" + host + "/" + project}, Project: project, Number: n}, true
}

// RemoteProjectPath is a remote's path to the repository, owner/name or a
// GitLab group's nesting, whatever host it is on: the project's path for
// an instance the user named.
func RemoteProjectPath(repo string) string {
	urls, err := remoteURLs(repo)
	if err != nil {
		return ""
	}
	for _, url := range urls {
		if _, _, path, ok := splitRemote(strings.TrimSpace(url)); ok {
			return path
		}
	}
	return ""
}

// Instance is the host's name, as the user would type it: gitlab.com, or
// a self-hosted instance's.
func (h Host) Instance() string {
	if h.None() {
		return ""
	}
	s := strings.TrimPrefix(strings.TrimPrefix(h.Page, "https://"), "http://")
	host, _, _ := strings.Cut(s, "/")
	return host
}

// LinesURL is the address of lines of a file in a commit's changes: hash
// is a commit, or a Range's changes, and oldSide the side of a deleted
// line. It is "" where the host's pages do not anchor lines.
func (h Host) LinesURL(hash, path string, oldSide bool, start, end int) string {
	if h.None() || path == "" {
		return ""
	}
	base, head, isRange := SplitRange(hash)
	if h.Kind == KindGitLab {
		page := h.Page + "/-/commit/" + hash
		if isRange {
			page = h.Page + "/-/compare/" + base + "..." + head
		}
		// GitLab anchors a file by the first 9 hex digits of its path's
		// SHA-1, and an added line with A before its number.
		sum := fmt.Sprintf("%x", sha1.Sum([]byte(path)))
		number := strconv.Itoa(start + 1)
		if !oldSide {
			number = "A" + number
		}
		return fmt.Sprintf("%s#line_%s_%s", page, sum[:9], number)
	}
	page, prefix := h.Page+"/commit/"+hash, "R"
	if isRange {
		page = h.Page + "/compare/" + base + "..." + head
	}
	if oldSide {
		prefix = "L"
	}
	anchor := fmt.Sprintf("%s%d", prefix, start+1)
	if end > start {
		anchor += fmt.Sprintf("-%s%d", prefix, end+1)
	}
	return fmt.Sprintf("%s#diff-%x%s", page, sha256.Sum256([]byte(path)), anchor)
}

// PathURLFor names a file's page at a commit, the old path's where the
// file was renamed or deleted.
func (h Host) PathURLFor(sha string, ch Change) string {
	p := ch.Path
	if ch.Status == "D" || ch.Status == "R" {
		p = ch.Old
	}
	return h.PathURL(sha, p, false)
}

// CodeHost returns the host of the repository's remotes, origin first and
// the others after it, for the first of them that is on a known host;
// none where none is. glabHosts are the hosts the GitLab CLI is signed in
// to, for self-hosted instances whose names do not say GitLab.
func CodeHost(repo string, glabHosts map[string]bool) Host {
	urls, err := remoteURLs(repo)
	if err != nil {
		return Host{}
	}
	for _, url := range urls {
		if h := hostOf(url, glabHosts); !h.None() {
			return h
		}
	}
	return Host{}
}

// remoteURLs are the repository's remotes' URLs, origin's first.
func remoteURLs(repo string) ([]string, error) {
	out, err := git(repo, "config", "--get-regexp", `^remote\..*\.url$`)
	if err != nil {
		return nil, err
	}
	var origin string
	var others []string
	for line := range strings.Lines(out) {
		key, url, _ := strings.Cut(strings.TrimSpace(line), " ")
		if key == "remote.origin.url" {
			origin = url
			continue
		}
		others = append(others, url)
	}
	if origin != "" {
		return append([]string{origin}, others...), nil
	}
	return others, nil
}

// hostOf returns the host a remote's URL is on, none where it is on no
// known host or malformed.
func hostOf(url string, glabHosts map[string]bool) Host {
	_, host, page, ok := splitRemote(strings.TrimSpace(url))
	if !ok {
		return Host{}
	}
	kind := hostKind(host, glabHosts)
	if kind == KindNone {
		return Host{}
	}
	// GitHub's repositories are owner/name; GitLab's nest in groups.
	if kind == KindGitHub && strings.Count(page, "/") != 1 {
		return Host{}
	}
	return Host{Kind: kind, Page: "https://" + strings.ToLower(host) + "/" + page}
}

// hostKind tells which code host a remote's hostname is: GitHub's, or
// GitLab's, as gitlab.com and self-hosted instances named for it, and
// those the GitLab CLI is signed in to.
func hostKind(host string, glabHosts map[string]bool) Kind {
	host = strings.ToLower(strings.TrimPrefix(host, "www."))
	switch {
	case host == "github.com":
		return KindGitHub
	case host == "gitlab.com", strings.HasPrefix(host, "gitlab."), strings.Contains(host, ".gitlab."):
		return KindGitLab
	case glabHosts[host]:
		return KindGitLab
	}
	return KindNone
}

// isHost reports whether host is name, or www. of it.
func isHost(host, name string) bool {
	return strings.EqualFold(strings.TrimPrefix(host, "www."), name)
}

// splitRemote reads a remote's URL as git writes it, scp-like and with a
// scheme both, into its scheme, host, and the project's path there, which
// GitLab's groups nest and GitHub's is owner/name.
func splitRemote(url string) (scheme, host, page string, ok bool) {
	if u, err := neturl.Parse(url); err == nil && u.Scheme != "" && u.Hostname() != "" {
		page = strings.Trim(strings.TrimPrefix(u.Path, "/"), "/")
		page = strings.TrimSuffix(page, ".git")
		if page == "" {
			return "", "", "", false
		}
		return strings.ToLower(u.Scheme), u.Hostname(), page, true
	}
	// scp-like, as git@host:group/repo
	at := strings.LastIndex(url, "@")
	colon := strings.Index(url, ":")
	if at < 0 || colon < at+1 {
		return "", "", "", false
	}
	host = url[at+1 : colon]
	page = strings.Trim(strings.Trim(url[colon+1:], "/"), "/")
	page = strings.TrimSuffix(page, ".git")
	if host == "" || page == "" {
		return "", "", "", false
	}
	return "ssh", host, page, true
}
