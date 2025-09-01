package workflow

import (
	"errors"
	"time"
)

type State string
type Event string

const (
	NEW       State = "NEW"
	ANALYZING State = "ANALYZING"
	PLANNING  State = "PLANNING"
	WAIT_GATE State = "WAIT_GATE"
	EXECUTING State = "EXECUTING"
	VERIFYING State = "VERIFYING"
	CLOSED    State = "CLOSED"
	PARKED    State = "PARKED"
)

const (
	EStartAnalysis  Event = "start_analysis"
	EAnalysisDone   Event = "analysis_done"
	EAnalysisFailed Event = "analysis_failed"
	EPlanReady      Event = "plan_ready"
	EPlanFailed     Event = "plan_failed"
	EGateApproved   Event = "gate_approved"
	EGateRejected   Event = "gate_rejected"
	EExecDone       Event = "exec_done"
	EExecFailed     Event = "exec_failed"
	EVerifyPass     Event = "verify_pass"
	EVerifyFailed   Event = "verify_failed"
	EForcePark      Event = "force_park"
)

var ErrIllegal = errors.New("illegal transition")
var ErrGuard = errors.New("guard not satisfied")

var table = map[State]map[Event]State{
	NEW:       {EStartAnalysis: ANALYZING, EForcePark: PARKED},
	ANALYZING: {EAnalysisDone: PLANNING, EAnalysisFailed: PARKED, EForcePark: PARKED},
	PLANNING:  {EPlanReady: WAIT_GATE, EPlanFailed: PARKED, EForcePark: PARKED},
	WAIT_GATE: {EGateApproved: EXECUTING, EGateRejected: PARKED, EForcePark: PARKED},
	EXECUTING: {EExecDone: VERIFYING, EExecFailed: PARKED, EForcePark: PARKED},
	VERIFYING: {EVerifyPass: CLOSED, EVerifyFailed: PARKED, EForcePark: PARKED},
	CLOSED:    {},
	PARKED:    {EStartAnalysis: ANALYZING, EPlanReady: WAIT_GATE},
}

type Context struct {
	Now          time.Time
	PlanComplete bool
	GateApproved bool
	ChangeWindow bool
	VerifyPassed bool
	Reason       string
	Actor        string
	IdemKey      string
	Extras       map[string]any
}

type TransitionIntent struct {
	From     State
	To       State
	Event    Event
	Timeline TimelineEntry
	Messages []OutboxMsg
}

type TimelineEntry struct {
	At     time.Time
	Actor  string
	Event  string
	Reason string
	Extras map[string]any
}

type OutboxMsg struct {
	Topic   string
	Payload map[string]any
}

type guardFn func(ctx Context) error

var guards = map[State]map[Event]guardFn{
	PLANNING: {
		EPlanReady: func(c Context) error {
			if !c.PlanComplete {
				return ErrGuard
			}
			return nil
		},
	},
	WAIT_GATE: {
		EGateApproved: func(c Context) error {
			if !c.GateApproved || !c.ChangeWindow {
				return ErrGuard
			}
			return nil
		},
	},
	VERIFYING: {
		EVerifyPass: func(c Context) error {
			if !c.VerifyPassed {
				return ErrGuard
			}
			return nil
		},
	},
}

func Decide(from State, ev Event, ctx Context, caseID string) (TransitionIntent, error) {
	next, ok := table[from][ev]
	if !ok {
		return TransitionIntent{}, ErrIllegal
	}
	if g, ok := guards[from][ev]; ok {
		if err := g(ctx); err != nil {
			return TransitionIntent{}, err
		}
	}
	intent := TransitionIntent{
		From:  from,
		To:    next,
		Event: ev,
		Timeline: TimelineEntry{
			At:     ctx.Now,
			Actor:  ctx.Actor,
			Event:  string(ev),
			Reason: ctx.Reason,
			Extras: ctx.Extras,
		},
		Messages: []OutboxMsg{
			{Topic: "evt.case.transition.v1", Payload: map[string]any{
				"case_id": caseID,
				"from":    from,
				"to":      next,
				"event":   ev,
				"actor":   ctx.Actor,
				"at":      ctx.Now.UTC(),
			}},
		},
	}
	return intent, nil
}
