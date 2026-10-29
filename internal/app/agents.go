package app

import (
	"context"
	"peek/internal/codex"
	"peek/internal/git"
	"peek/internal/pi"
	"peek/internal/review"

	"github.com/egoist/mygo/ui"
)

// agentKind is the AI agent the conversations and Ship use.
type agentKind string

const (
	agentCodex agentKind = "codex"
	agentPi    agentKind = "pi"
)

// agentDef is an agent's name and icon, and what writes its commit
// messages, reviews and answers: the model the user chose for its commit
// messages, "" for its own.
type agentDef struct {
	kind   agentKind
	name   string
	icon   *ui.SVG
	commit func(context.Context, string, git.Snapshot, string) (string, error)
	review func(context.Context, string, review.Snapshot, string, func(string, string)) (review.Result, string, error)
	ask    func(context.Context, string, string, review.Snapshot, string, func(string, string), func(string)) (string, string, error)
}

var agentDefs = []agentDef{
	{kind: agentCodex, name: "Codex", icon: codexIcon, commit: codex.CommitMessage, review: review.CodexSession, ask: review.Ask},
	{kind: agentPi, name: "Pi", icon: piIcon, commit: pi.CommitMessage, review: review.PiSession, ask: review.AskPi},
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
	if s == string(agentPi) {
		return agentPi
	}
	return agentCodex
}
