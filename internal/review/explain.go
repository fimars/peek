package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"peek/internal/git"
	"strings"
)

// CommitScope is the scope of an explanation of one commit's changes.
const CommitScope = "Commit"

// explainingPhase is the phase an explanation under way shows as, once
// the agent's events say it is past its tools.
const explainingPhase = "Explaining…"

// explainSchema is the JSON an explanation comes as: Markdown in one
// required field, nothing beside.
const explainSchema = `{"type":"object","properties":{"explanation":{"type":"string"}},"required":["explanation"],"additionalProperties":false}`

// CommitSnapshot is the snapshot of the commit's changes, from its first
// parent, or the empty tree for a first commit.
func CommitSnapshot(repo, hash string) (Snapshot, error) {
	g := func(args ...string) (string, error) {
		out, err := git.Bounded(repo, maxMetadata, args...)
		return strings.TrimSpace(out), err
	}
	base, err := g("rev-parse", "--verify", "--quiet", hash+"^1")
	if err != nil || base == "" {
		if base, err = g("hash-object", "-t", "tree", "/dev/null"); err != nil {
			return Snapshot{}, err
		}
	}
	return Snapshot{ID: CommitScope + "\x00" + hash, Scope: CommitScope, Base: base, Head: hash}, nil
}

// explainPrompt asks for an explanation of the snapshot's changes, with
// the fixed revisions to read them at.
func explainPrompt(snapshot Snapshot) string {
	return "Explain the changes in the scope below to someone about to read them: what they do and why, " +
		"and how their parts fit together. Explore the repository as you need. Do not modify files. " +
		"Answer briefly, in Markdown. Return only the required JSON object.\n\n" + snapshot.explainScope()
}

// explainAnswer is the object the explain schema asks for.
type explainAnswer struct {
	Explanation string `json:"explanation"`
}

// text is the answer's explanation, trimmed; an error where the agent
// returned none.
func (a explainAnswer) text() (string, error) {
	if text := strings.TrimSpace(a.Explanation); text != "" {
		return text, nil
	}
	return "", errors.New("the agent returned no explanation")
}

// parseExplanation reads an explanation from an agent's raw answer.
func parseExplanation(data []byte) (string, error) {
	var answer explainAnswer
	if err := json.Unmarshal(data, &answer); err != nil {
		return "", fmt.Errorf("invalid explanation: %w", err)
	}
	return answer.text()
}

func (s Snapshot) explainScope() string {
	if s.Base != "" {
		return "Scope: " + s.Scope + ", the changes from " + s.Base + " to " + s.Head + ".\n" +
			"git diff --no-ext-diff --no-textconv " + s.Base + " " + s.Head + " --"
	}
	return "Scope: Uncommitted changes, staged, unstaged and untracked, against HEAD " + s.Head + ".\n" +
		"git status, git diff --cached, git diff, and the untracked files themselves."
}
