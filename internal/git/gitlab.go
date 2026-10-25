package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// glabExecutable finds the GitLab CLI: a Finder-launched app has the
// system's PATH alone.
func glabExecutable() (string, error) {
	if p, err := exec.LookPath("glab"); err == nil {
		return p, nil
	}
	for _, p := range []string{"/opt/homebrew/bin/glab", "/usr/local/bin/glab"} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("GitLab CLI was not found; install glab and run glab auth login")
}

// glab runs the GitLab CLI in the repository, its prompts off: what it
// would ask is the user's to answer in a terminal.
func glab(ctx context.Context, repo string, args ...string) (string, error) {
	bin, err := glabExecutable()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GLAB_NO_PROMPT=1", "NO_COLOR=1")
	cmd.WaitDelay = time.Second
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		// JSON output puts the failure in an envelope of its own, which
		// says what went wrong in a line; what it prints for a terminal
		// spans lines of headings and banners.
		if msg := jsonErrorMessage(out.Bytes()); msg != "" {
			return "", fmt.Errorf("%s", msg)
		}
		if said := strings.TrimSpace(stderr.String()); said != "" {
			return "", fmt.Errorf("%s", said)
		}
		return "", fmt.Errorf("glab %s: %w", strings.Join(args, " "), err)
	}
	return out.String(), nil
}

// jsonErrorMessage reads the message of the CLI's JSON failure, "" where
// its output is not one.
func jsonErrorMessage(data []byte) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return ""
	}
	return strings.TrimSpace(envelope.Error.Message)
}

// GitLabHosts returns the hosts the GitLab CLI is signed in to, by name:
// a self-hosted instance's name need not say GitLab. It runs the CLI, so
// its answer is kept for a while.
func GitLabHosts() map[string]bool {
	glabHosts.mu.Lock()
	defer glabHosts.mu.Unlock()
	if time.Since(glabHosts.at) < glabHostsFor && glabHosts.hosts != nil {
		return glabHosts.hosts
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, _ := glab(ctx, "", "auth", "status", "--all", "--output", "json")
	glabHosts.at, glabHosts.hosts = time.Now(), parseGlabHosts([]byte(out))
	return glabHosts.hosts
}

// glabHostsFor is how long the CLI's answer is kept: a sign-in is the
// user's to change, and the answer is the same until then.
const glabHostsFor = 5 * time.Minute

var glabHosts struct {
	mu    sync.Mutex
	at    time.Time
	hosts map[string]bool
}

// parseGlabHosts reads glab auth status's JSON, which nests the hosts
// under hosts, keeping those it could reach and sign in to.
func parseGlabHosts(data []byte) map[string]bool {
	var status struct {
		Hosts []struct {
			Host  string `json:"host"`
			State string `json:"state"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return nil
	}
	signed := map[string]bool{}
	for _, h := range status.Hosts {
		if h.Host != "" && h.State == "success" {
			signed[strings.ToLower(h.Host)] = true
		}
	}
	return signed
}

// MergeRequest is an open merge request, as glab lists it.
type MergeRequest struct {
	Number int    `json:"iid"`
	Title  string `json:"title"`
	Branch string `json:"source_branch"`
	Target string `json:"target_branch"`
	Draft  bool   `json:"draft"`
	Sha    string `json:"sha"`
	Author struct {
		Username string `json:"username"`
	} `json:"author"`
}

// MergeRequestRef is the local ref a merge request's head is fetched to.
func MergeRequestRef(number int) string {
	return "refs/peek/merge-requests/" + strconv.Itoa(number)
}

// MergeRequestRemoteRef is the ref GitLab publishes a merge request's
// head at.
func MergeRequestRemoteRef(number int) string {
	return "refs/merge-requests/" + strconv.Itoa(number) + "/head"
}

// ListMergeRequests lists a project's open merge requests, as glab does:
// page is the project's page, whose host it signs in to.
func ListMergeRequests(ctx context.Context, repo, page string) ([]MergeRequest, error) {
	out, err := glab(ctx, repo, "mr", "list", "--repo", page, "--output", "json",
		"--per-page", "50", "--order", "updated_at", "--sort", "desc")
	if err != nil {
		return nil, err
	}
	return parseMergeRequests([]byte(out))
}

func parseMergeRequests(data []byte) ([]MergeRequest, error) {
	var mrs []MergeRequest
	if err := json.Unmarshal(data, &mrs); err != nil {
		return nil, fmt.Errorf("read glab's merge requests: %w", err)
	}
	return mrs, nil
}

// MergeRequestBase returns the commit a merge request left its target
// branch at: the merge base of the two, as the fetched refs have them.
func MergeRequestBase(repo string, mr MergeRequest) (string, error) {
	head := MergeRequestRef(mr.Number)
	for _, remote := range mergeRequestRemotes(repo) {
		ref := "refs/remotes/" + remote + "/" + mr.Target
		if _, err := git(repo, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
			continue
		}
		if base, err := git(repo, "merge-base", ref, head); err == nil {
			return strings.TrimSpace(base), nil
		}
	}
	return "", fmt.Errorf("the merge request's target branch %s is not fetched yet", mr.Target)
}

// mergeRequestRemotes are the remotes a merge request's target branch may
// be fetched to, as remote-tracking refs.
func mergeRequestRemotes(repo string) []string {
	out, err := git(repo, "config", "--get-regexp", `^remote\..*\.url$`)
	if err != nil {
		return nil
	}
	var remotes []string
	for line := range strings.Lines(out) {
		key, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		if name, ok := strings.CutPrefix(key, "remote."); ok {
			if name, ok := strings.CutSuffix(name, ".url"); ok && name != "" {
				remotes = append(remotes, name)
			}
		}
	}
	return remotes
}

// FetchMergeRequest fetches a merge request's head, and the branch it
// merges into, and returns the commit they left off at and the head, for
// the changes to show as a range.
func FetchMergeRequest(ctx context.Context, repo, remote string, mr MergeRequest) (base, head string, err error) {
	refspecs := []string{"+" + MergeRequestRemoteRef(mr.Number) + ":" + MergeRequestRef(mr.Number)}
	// The target branch, for the merge base, from the same remote.
	if mr.Target != "" {
		refspecs = append([]string{fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", mr.Target, remote, mr.Target)}, refspecs...)
	}
	args := append([]string{"fetch", "--quiet", "--no-tags", "--no-write-fetch-head", "--", remote}, refspecs...)
	if _, err := Run(ctx, repo, args...); err != nil {
		return "", "", err
	}
	head, err = Run(ctx, repo, "rev-parse", "--verify", MergeRequestRef(mr.Number)+"^{commit}")
	if err != nil {
		return "", "", fmt.Errorf("the merge request was fetched without its head: %w", err)
	}
	base, err = MergeRequestBase(repo, mr)
	return base, head, err
}

// CheckoutMergeRequest checks out a merge request as glab does, as a
// local branch tracking its source.
func CheckoutMergeRequest(ctx context.Context, repo, page string, number int) error {
	_, err := glab(ctx, repo, "mr", "checkout", strconv.Itoa(number), "--repo", page)
	return err
}

// ParseMergeRequest reads a merge request's number, as 12 or !12, or its
// page on GitLab at page, as the browser shows it.
func ParseMergeRequest(s, page string) (int, bool) {
	s = strings.TrimSpace(s)
	if n, ok := mergeRequestPageNumber(s, page); ok {
		return n, ok
	}
	n, err := strconv.Atoi(strings.TrimPrefix(s, "!"))
	return n, err == nil && n > 0
}

// mergeRequestPageNumber reads a merge request's number from its page,
// where the page is the project's at page.
func mergeRequestPageNumber(s, page string) (int, bool) {
	page = strings.TrimSuffix(strings.Replace(page, "http://", "https://", 1), "/")
	s = strings.Replace(s, "http://", "https://", 1)
	if page == "" || !strings.HasPrefix(s, page+"/") {
		return 0, false
	}
	rest, ok := strings.CutPrefix(strings.TrimPrefix(s[len(page)+1:], "-/"), "merge_requests/")
	if !ok {
		return 0, false
	}
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	n, err := strconv.Atoi(rest)
	return n, err == nil && n > 0
}
