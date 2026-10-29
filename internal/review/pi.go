package review

import (
	"context"
	"encoding/json"
	"fmt"
	"peek/internal/pi"
	"time"
)

// PiSession is Pi in a conversation Pi keeps, whose id it returns, for
// requests to follow up on the review, as CodexSession is Codex's.
func PiSession(ctx context.Context, repo string, snapshot Snapshot, request string, progress func(string, string)) (Result, string, error) {
	data, session, err := pi.JSON(ctx, repo, "", "", reviewPrompt(snapshot, request), 10*time.Minute,
		piProgress(progress))
	if err != nil {
		return Result{}, session, err
	}
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return Result{}, session, fmt.Errorf("invalid Pi review: %w", err)
	}
	err = result.validate()
	return result, session, err
}

// AskPi has Pi answer the request about the changes of the snapshot, as
// Ask has Codex: the conversation's id it returns is Pi's, whose
// conversations Codex's are apart from.
func AskPi(ctx context.Context, repo, session string, snapshot Snapshot, request string, progress func(string, string), delta func(string)) (string, string, error) {
	text, session, err := pi.Stream(ctx, repo, session, "", askPrompt(session, snapshot, request), 10*time.Minute,
		func(e pi.Event) { eventProgress(e.Delta, e.Step, e.Detail, "Pi", progress, delta) })
	if err != nil {
		return "", session, err
	}
	if text == "" {
		return "", session, fmt.Errorf("Pi returned no answer")
	}
	return text, session, nil
}

// piProgress tells Pi's events to the review's progress, where it is not
// nil, under Pi's name.
func piProgress(progress func(string, string)) func(pi.Event) {
	if progress == nil {
		return nil
	}
	return func(e pi.Event) { eventProgress(e.Delta, e.Step, e.Detail, "Pi", progress, nil) }
}
