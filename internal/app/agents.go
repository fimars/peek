package app

import (
	"context"
	"peek/internal/codex"
	"peek/internal/git"
	"peek/internal/grok"

	"github.com/egoist/mygo/ui"
)

// agentKind is the AI agent Ship uses.
type agentKind string

const (
	agentCodex agentKind = "codex"
	agentGrok  agentKind = "grok"
)

// agentDef is an agent's name and icon, and what it writes the commit
// message with: the model the user chose for it, "" for its own.
type agentDef struct {
	kind   agentKind
	name   string
	icon   *ui.SVG
	commit func(context.Context, string, git.Snapshot, string) (string, error)
}

var agentDefs = []agentDef{
	{kind: agentCodex, name: "Codex", icon: codexIcon, commit: codex.CommitMessage},
	{kind: agentGrok, name: "Grok Build", icon: grokIcon, commit: grok.CommitMessage},
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
