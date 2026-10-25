package review

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

const maxEvent = 64 * 1024

// The phases an engine's events are reported as: the tool it runs, its
// thinking over what it read, and its answer being read. reword keeps the
// tool phase and relabels the others.
const (
	inspectingPhase = "Inspecting…"
	reviewingPhase  = "Reviewing…"
	checkingPhase   = "Checking…"
)

// lineBuffer splits a stream's writes at newlines for event, dropping a
// line longer than max rather than retaining its contents.
type lineBuffer struct {
	line     []byte
	dropping bool
}

func (b *lineBuffer) write(p []byte, max int, event func([]byte)) int {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		part := p
		if i >= 0 {
			part = p[:i]
		}
		if !b.dropping {
			if len(b.line)+len(part) > max {
				b.line, b.dropping = nil, true
			} else {
				b.line = append(b.line, part...)
			}
		}
		if i < 0 {
			break
		}
		if !b.dropping {
			event(b.line)
		}
		b.line, b.dropping = b.line[:0], false
		p = p[i+1:]
	}
	return n
}

// reword reports an engine's stages as one operation's, worded phase and
// detail; the tool stage keeps its own phase and detail, the command the
// engine named. An empty phase leaves the engine's review wording.
func reword(report func(string, string), phase, detail string) func(string, string) {
	if phase == "" {
		return report
	}
	return func(p, d string) {
		if p != inspectingPhase {
			p, d = phase, detail
		}
		report(p, d)
	}
}

// progressTo feeds Codex's events to report as a review's stages, or,
// with phase and detail, as those of other work, only what Codex inspects
// told apart; nil where report is.
func progressTo(report func(string, string), phase, detail string) io.Writer {
	if report == nil {
		return nil
	}
	return &progressWriter{report: reword(report, phase, detail)}
}

// grokProgressTo feeds Grok Build's events to report likewise; nil where
// report is.
func grokProgressTo(report func(string, string), phase, detail string) io.Writer {
	if report == nil {
		return nil
	}
	return &grokProgress{report: reword(report, phase, detail)}
}

// Consume JSONL incrementally. Command output can be very large; discard
// oversized events without retaining their contents or interrupting Codex.
// Only activity metadata is displayed, never reasoning or command output.
type progressWriter struct {
	lineBuffer
	report func(string, string)
}

func (w *progressWriter) Write(p []byte) (int, error) {
	return w.lineBuffer.write(p, maxEvent, w.event), nil
}

func (w *progressWriter) event(data []byte) {
	var event struct {
		Type string `json:"type"`
		Item struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"item"`
	}
	if json.Unmarshal(data, &event) != nil {
		return
	}
	switch event.Type {
	case "thread.started", "turn.started":
		w.report(reviewingPhase, "Codex is reviewing changes")
	case "item.started":
		if event.Item.Type == "command_execution" {
			command := []rune(strings.Join(strings.Fields(event.Item.Command), " "))
			if len(command) > 240 {
				command = append(command[:240], '…')
			}
			w.report(inspectingPhase, string(command))
		} else if event.Item.Type == "mcp_tool_call" || event.Item.Type == "web_search" {
			w.report(inspectingPhase, "Codex is gathering context")
		}
	case "item.completed":
		if event.Item.Type == "command_execution" || event.Item.Type == "mcp_tool_call" || event.Item.Type == "web_search" {
			w.report(reviewingPhase, "Codex is reviewing the collected context")
		}
	case "turn.completed":
		w.report(checkingPhase, "Validating the review result and checking for changes")
	}
}

// grokProgress reports the activity of Grok Build's streaming events. Only
// tool activity is displayed, never reasoning or tool output.
type grokProgress struct {
	lineBuffer
	report func(string, string)
}

func (w *grokProgress) Write(p []byte) (int, error) {
	return w.lineBuffer.write(p, maxEvent, w.event), nil
}

func (w *grokProgress) event(data []byte) {
	var event struct {
		Type     string `json:"type"`
		Status   string `json:"status"`
		Title    string `json:"title"`
		RawInput struct {
			Command string `json:"command"`
			Path    string `json:"path"`
		} `json:"rawInput"`
	}
	if json.Unmarshal(data, &event) != nil {
		return
	}
	switch event.Type {
	case "tool_call":
		w.report(inspectingPhase, grokDetail(event.Title, event.RawInput.Command, event.RawInput.Path))
	case "tool_call_update":
		if event.Status == "completed" {
			w.report(reviewingPhase, "Grok Build is reviewing the collected context")
		}
	case "end":
		w.report(checkingPhase, "Validating the review result and checking for changes")
	}
}

// grokDetail is what a tool call is doing: its cleaned command, else its
// title or path.
func grokDetail(title, command, path string) string {
	detail := []rune(strings.Join(strings.Fields(command), " "))
	if len(detail) == 0 {
		detail = []rune(strings.TrimSpace(title))
	}
	if len(detail) == 0 {
		detail = []rune(strings.TrimSpace(path))
	}
	if len(detail) == 0 {
		return "Grok Build is gathering context"
	}
	if len(detail) > 240 {
		detail = append(detail[:240], '…')
	}
	return string(detail)
}
