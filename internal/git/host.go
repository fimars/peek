package git

import (
	"net"
	neturl "net/url"
	"slices"
	"strconv"
	"strings"
)

// Kind is the code host a repository is on.
type Kind int

const (
	KindNone Kind = iota
	KindGitHub
	KindGitLab
)

// Host is the code host a repository's remote is on, and the page of the
// repository there.
type Host struct {
	Kind Kind
	Page string
}

// None reports whether the repository is on no recognized host.
func (h Host) None() bool { return h.Kind == KindNone }

// Name is the host's name, as GitHub or GitLab.
func (h Host) Name() string {
	switch h.Kind {
	case KindGitHub:
		return "GitHub"
	case KindGitLab:
		return "GitLab"
	}
	return ""
}

// urlPrefix is the repository's page followed by the separator its URLs
// use.
func (h Host) urlPrefix() string {
	if h.None() || h.Page == "" {
		return ""
	}
	if h.Kind == KindGitLab {
		return h.Page + "/-/"
	}
	return h.Page + "/"
}

// CommitURL is the commit's page.
func (h Host) CommitURL(sha string) string {
	if p := h.urlPrefix(); p != "" {
		return p + "commit/" + sha
	}
	return ""
}

// PathURL is the page of a file or a folder at the commit.
func (h Host) PathURL(sha, path string, dir bool) string {
	p := h.urlPrefix()
	if p == "" {
		return ""
	}
	if dir {
		return p + "tree/" + sha + "/" + path
	}
	return p + "blob/" + sha + "/" + path
}

// IssueURL is the page of the issue or pull request, numbered.
func (h Host) IssueURL(number int) string {
	if p := h.urlPrefix(); p != "" {
		return p + "issues/" + strconv.Itoa(number)
	}
	return ""
}

// BranchURL is the page of the branch.
func (h Host) BranchURL(branch string) string {
	if p := h.urlPrefix(); p != "" {
		return p + "tree/" + escapePath(branch)
	}
	return ""
}

// CodeHost returns the code host the repository's remotes are on, origin
// first, and the page of the repository there; none where no remote is
// on a host it recognizes.
func CodeHost(repo string) Host {
	out, err := git(repo, "config", "--get-regexp", `^remote\..*\.url$`)
	if err != nil {
		return Host{}
	}
	var found Host
	for line := range strings.Lines(out) {
		key, url, _ := strings.Cut(strings.TrimSpace(line), " ")
		h := hostOf(url)
		if h.None() {
			continue
		}
		if key == "remote.origin.url" {
			return h
		}
		if found.None() {
			found = h
		}
	}
	return found
}

// CurrentBranchURL returns the page of the branch checked out, on the
// remote it tracks, "" where it tracks none, as a branch never pushed or
// a detached HEAD. The remote's default branch has the repository's own
// page.
func CurrentBranchURL(repo string) string {
	head, err := git(repo, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return ""
	}
	out, err := git(repo, "for-each-ref", "--format=%(upstream:remotename)%00%(upstream:remoteref)", strings.TrimSpace(head))
	remote, ref, _ := strings.Cut(strings.TrimSpace(out), "\x00")
	branch, ok := strings.CutPrefix(ref, "refs/heads/")
	if err != nil || remote == "" || !ok {
		return ""
	}
	url, err := git(repo, "config", "--get", "remote."+remote+".url")
	host := hostOf(strings.TrimSpace(url))
	if err != nil || host.None() {
		return ""
	}
	if branch == remoteDefaultBranch(repo, remote) {
		return host.Page
	}
	return host.BranchURL(branch)
}

// hostOf returns the code host a remote's URL names, and the page of the
// repository there, as SSH, scp-like and HTTPS URLs name it, none for a
// host it does not recognize.
func hostOf(raw string) Host {
	host, path, ok := splitRemote(strings.TrimSpace(raw))
	if !ok {
		return Host{}
	}
	host = strings.ToLower(host)
	path = strings.TrimPrefix(strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git"), "/")
	kind := kindOf(hostName(host))
	if !isRepoPath(kind, path) {
		return Host{}
	}
	return Host{Kind: kind, Page: "https://" + host + "/" + path}
}

// hostName is the host without the port a self-hosted instance may have.
func hostName(host string) string {
	if name, _, err := net.SplitHostPort(host); err == nil {
		return name
	}
	return host
}

// splitRemote returns the host and the path of a remote's URL.
func splitRemote(raw string) (host, path string, ok bool) {
	if strings.Contains(raw, "://") {
		u, err := neturl.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return "", "", false
		}
		switch u.Scheme {
		case "https", "http", "ssh", "git":
		default:
			return "", "", false
		}
		return u.Host, u.Path, true
	}
	at := strings.LastIndex(raw, "@")
	if at < 0 {
		return "", "", false
	}
	colon := strings.Index(raw[at:], ":")
	if colon < 0 {
		return "", "", false
	}
	return raw[at+1 : at+colon], raw[at+colon+1:], true
}

// kindOf returns the kind of code host the hostname names.
func kindOf(host string) Kind {
	switch {
	case host == "github.com":
		return KindGitHub
	case host == "gitlab.com", strings.HasPrefix(host, "gitlab."), strings.Contains(host, ".gitlab."):
		return KindGitLab
	}
	return KindNone
}

// isRepoPath reports whether path is a repository on a host of this kind.
// GitHub takes owner/name; GitLab takes any group nesting.
func isRepoPath(kind Kind, path string) bool {
	parts := strings.Split(path, "/")
	if slices.Contains(parts, "") {
		return false
	}
	switch kind {
	case KindGitHub:
		return len(parts) == 2
	case KindGitLab:
		return len(parts) >= 2
	}
	return false
}

// escapePath escapes the path's segments, keeping the slashes between
// them.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = neturl.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
