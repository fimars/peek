package review

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"peek/internal/grok"
	"strings"
	"time"
)

// Grok has Grok Build review the snapshot's changes, in its read-only
// sandbox.
func Grok(ctx context.Context, repo string, snapshot Snapshot, progress func(string, string)) (Result, error) {
	var events io.Writer
	if progress != nil {
		events = &grokProgress{report: progress}
	}
	data, err := grok.JSON(ctx, repo, reviewSchema, reviewPrompt(snapshot)+grokReviewFormat, 10*time.Minute, events)
	if err != nil {
		return Result{}, err
	}
	return parseGrokReview(data)
}

// grokReviewFormat names the schema's fields, which the model has left out
// or named differently despite the schema.
const grokReviewFormat = "\n\nAnswer with one JSON object with status, summary and findings, " +
	"with findings [] when there are none. Give each finding its priority, title, body, path and line, " +
	"with path the repository-relative file name."

// parseGrokReview reads Grok Build's answer. The model sometimes leaves the
// empty list or summary out, and names a finding's file where the schema
// says path.
func parseGrokReview(data []byte) (Result, error) {
	var answer struct {
		Status   string `json:"status"`
		Summary  string `json:"summary"`
		Findings []struct {
			Priority int    `json:"priority"`
			Title    string `json:"title"`
			Body     string `json:"body"`
			Path     string `json:"path"`
			File     string `json:"file"`
			Line     int    `json:"line"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(data, &answer); err != nil {
		return Result{}, fmt.Errorf("invalid review: %w", err)
	}
	result := Result{Status: answer.Status, Summary: answer.Summary, Findings: make([]Finding, len(answer.Findings))}
	for i, f := range answer.Findings {
		path := f.Path
		if path == "" {
			path = f.File
		}
		result.Findings[i] = Finding{Priority: f.Priority, Title: f.Title, Body: f.Body, Path: path, Line: f.Line}
	}
	if strings.TrimSpace(result.Summary) == "" {
		result.Summary = "The review found issues."
		if len(result.Findings) == 0 {
			result.Summary = "No actionable defects found."
		}
	}
	if err := result.validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}
