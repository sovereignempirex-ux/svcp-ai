package svpc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agent "github.com/svpc-ai/svpc/internal/llm/agent"
)

// Event is one thing that happened during a turn.
//
// A turn produces many of these, in order, and the ones a caller does not care
// about can be ignored. Text arrives in pieces as the model writes it, so a
// caller that assembles an answer should append rather than assign.
//
// Exactly one of Text, Tool, ToolError or Error is set. SessionID is set on the
// first event of a turn, and Permission on an event that needs an answer.
type Event struct {
	// SessionID is the conversation the turn is in, sent once at the start.
	SessionID string
	// Text is part of the model's answer.
	Text string
	// Tool is a tool the agent is using. Running means it started, Done means it
	// finished, and neither being set with a non-empty Error means it failed.
	Tool Tool
	// ToolError is a tool that failed. The turn carries on: a failed command is
	// something the model reacts to, not a reason to stop.
	ToolError ToolError
	// Permission is an approval request. The turn is waiting, and nothing happens
	// until Options.OnPermission answers it or the request times out.
	Permission *Request
	// Error ends the turn. Nothing after it.
	Error error
}

// Tool is a tool call the agent made.
type Tool struct {
	ID    string
	Name  string
	Input string
	// Running is true between the tool starting and finishing.
	Running bool
	Done    bool
}

// ToolError is a tool that failed.
type ToolError struct {
	ID    string
	Error string
}

// Ask runs one turn and reports what happens as it happens.
//
// The callback is called on the agent's own goroutine, in order, and must not
// block: a callback that blocks holds up the tools that follow. Returning false
// stops the turn being watched but does not cancel it — use the context for that.
//
// An empty session starts a new conversation. The id of the conversation in use
// arrives on the first event, so a caller that wants to continue it later keeps
// that.
func (a *Agent) Ask(ctx context.Context, sessionID, prompt string, onEvent func(Event) bool) error {
	if err := a.check(); err != nil {
		return err
	}
	if strings.TrimSpace(prompt) == "" {
		return errors.New("svpc: the prompt is empty")
	}
	if a.core == nil {
		if a.startErr != nil {
			if verr := configValidate(); verr != nil {
				return fmt.Errorf("%w: %v", ErrNoProvider, verr)
			}
			return fmt.Errorf("%w: %v", ErrNoProvider, a.startErr)
		}
		return ErrNoProvider
	}

	events, err := a.core.CoderAgent.Run(ctx, sessionID, prompt)
	if err != nil {
		// The agent refuses a second turn in the same session, because the tools of
		// the first are still running. Saying so is more use than the raw error.
		if errors.Is(err, agent.ErrSessionBusy) {
			return fmt.Errorf("svpc: this conversation already has a turn running")
		}
		return fmt.Errorf("svpc: starting the turn: %w", err)
	}

	first := true
	for ev := range events {
		if onEvent == nil {
			continue
		}
		out := Event{}
		if first && ev.SessionID != "" {
			out.SessionID = ev.SessionID
			first = false
		}
		out.Text = ev.Message.Content().String()

		for _, call := range ev.Message.ToolCalls() {
			out.Tool = Tool{
				ID:      call.ID,
				Name:    call.Name,
				Input:   summarise(call.Input),
				Running: !call.Finished,
				Done:    call.Finished,
			}
			if !onEvent(out) {
				return nil
			}
			out.Tool = Tool{}
		}
		for _, res := range ev.Message.ToolResults() {
			if res.IsError {
				out.ToolError = ToolError{ID: res.ToolCallID, Error: firstLine(res.Content)}
				if !onEvent(out) {
					return nil
				}
				out.ToolError = ToolError{}
			}
		}
		if ev.Error != nil {
			out.Error = ev.Error
			if !onEvent(out) {
				return nil
			}
			out.Error = nil
		}
		if out.Text != "" {
			if !onEvent(out) {
				return nil
			}
		}
	}
	return nil
}

// Answer runs one turn and returns the reply as one string, for a caller that does
// not need to watch it happen. Tool activity is discarded, so a program that only
// wants the answer does not have to know that tools exist.
//
// The session id comes back too, because a caller that wants a second turn needs
// it and there is nowhere else to get it: it is the one value the turn discovers
// rather than the one it was given.
//
// An empty sessionID starts a new conversation.
func (a *Agent) Answer(ctx context.Context, sessionID, prompt string) (answer, session string, err error) {
	var b strings.Builder
	err = a.Ask(ctx, sessionID, prompt, func(ev Event) bool {
		if ev.SessionID != "" {
			session = ev.SessionID
		}
		b.WriteString(ev.Text)
		return true
	})
	if err != nil {
		return "", "", err
	}
	return b.String(), session, nil
}

// summarise keeps a tool's input short enough to show. The raw input is a JSON
// document that can run to pages, and a caller displaying it wants the shape of it
// rather than all of it.
func summarise(input string) string {
	const limit = 200
	s := strings.TrimSpace(input)
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}

// firstLine is what a caller can put in front of a person: a tool error is often a
// stack of output, and the reason is in the first line of it.
func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// The agent publishes a request and waits; this is the other end of that wait.
// Answering through the service rather than from the callback keeps the decision
// with the thing that owns the policy, so a denial here and a denial from the
// window are the same event as far as the tool is concerned.
func (a *Agent) watchPermissions(ctx context.Context) {
	svc := a.core.Permissions
	go func() {
		for ev := range svc.Subscribe(ctx) {
			req := ev.Payload
			switch a.opts.OnPermission(Request{
				ID:          req.ID,
				SessionID:   req.SessionID,
				Tool:        req.ToolName,
				Action:      req.Action,
				Description: req.Description,
				Path:        req.Path,
			}) {
			case Allow:
				svc.Grant(req)
			case AllowSession:
				// Both: this one now, and no more of the same in this session, so a
				// caller that answers "yes, always" is not asked again per command.
				svc.Grant(req)
				svc.AutoApproveSession(req.SessionID)
			default:
				svc.Deny(req)
			}
		}
	}()
}
