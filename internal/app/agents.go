package app

import (
	"context"
	"peek/internal/codex"
	"peek/internal/git"
	"peek/internal/grok"
	"peek/internal/review"

	"github.com/egoist/mygo/ui"
)

// agentKind is the AI agent Ship, Review, and Explain use.
type agentKind string

const (
	agentCodex agentKind = "codex"
	agentGrok  agentKind = "grok"
)

// agentDef is an agent's name and icon, and what it writes the commit
// message, reviews the changes with, and explains them with.
type agentDef struct {
	kind    agentKind
	name    string
	icon    *ui.SVG
	commit  func(context.Context, string, git.Snapshot) (string, error)
	review  func(context.Context, string, review.Snapshot, func(string, string)) (review.Result, error)
	explain func(context.Context, string, review.Snapshot, func(string, string)) (string, error)
}

var agentDefs = []agentDef{
	{kind: agentCodex, name: "Codex", icon: codexIcon, commit: codex.CommitMessage, review: review.Codex, explain: review.CodexExplain},
	{kind: agentGrok, name: "Grok Build", icon: grokIcon, commit: grok.CommitMessage, review: review.Grok, explain: review.GrokExplain},
}

// agentDefOf is the agent's definition, or Codex's for an unknown value.
func agentDefOf(k agentKind) agentDef {
	for _, d := range agentDefs {
		if d.kind == k {
			return d
		}
	}
	return agentDefs[0]
}

func parseAgent(s string) agentKind {
	if s == string(agentGrok) {
		return agentGrok
	}
	return agentCodex
}
