package review

import (
	"context"
	"io"
	"peek/internal/codex"
	"time"
)

// Codex has Codex review the snapshot's changes, in its read-only sandbox.
func Codex(ctx context.Context, repo string, snapshot Snapshot, progress func(string, string)) (Result, error) {
	var events io.Writer
	if progress != nil {
		events = &progressWriter{report: progress}
	}
	data, err := codex.JSON(ctx, repo, reviewSchema, reviewPrompt(snapshot), 10*time.Minute, events)
	if err != nil {
		return Result{}, err
	}
	return parseReview(data)
}
