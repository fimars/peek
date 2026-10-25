package review

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"peek/internal/codex"
	"time"
)

// Codex has Codex review the snapshot's changes, in its read-only sandbox.
func Codex(ctx context.Context, repo string, snapshot Snapshot, progress func(string, string)) (Result, error) {
	data, err := codex.JSON(ctx, repo, reviewSchema, reviewPrompt(snapshot), 10*time.Minute, progressTo(progress, "", ""))
	if err != nil {
		return Result{}, err
	}
	return parseReview(data)
}

// runCodex has Codex do what the prompt asks over the repository, in a
// read-only sandbox with the user's settings, its JSON answer, of the
// schema, read into result, the answer named what; progress, where not
// nil, is fed its events.
func runCodex(ctx context.Context, repo, schema, prompt string, progress io.Writer, what string, result any) error {
	data, err := codex.JSON(ctx, repo, schema, prompt, 10*time.Minute, progress)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("invalid Codex %s: %w", what, err)
	}
	return nil
}

// CodexExplain has Codex explain the changes of the snapshot, in
// Markdown. As it does to review, Codex reads the changes and the code
// around them itself, in a read-only sandbox, with the user's settings.
func CodexExplain(ctx context.Context, repo string, snapshot Snapshot, progress func(string, string)) (string, error) {
	var answer explainAnswer
	if err := runCodex(ctx, repo, explainSchema, explainPrompt(snapshot),
		progressTo(progress, explainingPhase, "Codex is explaining the changes"),
		"explanation", &answer); err != nil {
		return "", err
	}
	return answer.text()
}
