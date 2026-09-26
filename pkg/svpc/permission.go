package svpc

import "github.com/svpc-ai/svpc/internal/config"

// Decision is the answer to an approval request.
type Decision int

const (
	// Deny refuses the action. This is the zero value, so a Decision that was never
	// set refuses rather than allows: a caller that forgets to handle a request
	// gets the safe outcome.
	Deny Decision = iota
	// Allow permits this one action.
	Allow
	// AllowSession permits this action and anything equivalent in this session, so
	// a command run in a loop is approved once rather than once per iteration.
	AllowSession
)

// String names the decision, for a log line or a test failure.
func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case AllowSession:
		return "allow_session"
	default:
		return "deny"
	}
}

// Request is a question the agent is asking before it acts.
//
// It arrives on the goroutine running the turn, and the turn does not continue
// until it is answered. The wait is bounded: after five minutes the action is
// refused and the model is told so, which it can react to by asking differently or
// explaining itself. A caller that hangs therefore fails safe rather than forever.
//
// The approval is scoped by Tool, Action and Path, not by the request id, so
// AllowSession covers the same command in the same directory for the rest of the
// session.
type Request struct {
	// ID identifies this one request, and is what a UI shows next to its answer.
	ID string
	// SessionID is the conversation asking.
	SessionID string
	// Tool is the tool's name, such as "bash" or "edit".
	Tool string
	// Action is what it wants to do, in the tool's own words.
	Action string
	// Description is the sentence the model wrote to justify it. It is the text to
	// show a person, because the tool and action alone do not say what is about to
	// happen to their files.
	Description string
	// Path is the directory the approval is scoped to. Empty means the project's own
	// working directory.
	Path string
}

// configValidate is named here so the one caller that wants a message a person can
// act on does not have to import the configuration package itself, which would put
// an internal package in this file's imports for no other reason.
func configValidate() error { return config.Validate() }
