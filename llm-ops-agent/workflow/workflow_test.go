package workflow

import (
	"errors"
	"testing"
	"time"
)

func TestDecideValidTransitions(t *testing.T) {
	now := time.Unix(0, 0)
	base := Context{Now: now, Actor: "alice"}
	cases := []struct {
		name  string
		from  State
		event Event
		ctx   Context
		want  State
	}{
		{"new_start_analysis", NEW, EStartAnalysis, base, ANALYZING},
		{"new_force_park", NEW, EForcePark, base, PARKED},
		{"analysis_done", ANALYZING, EAnalysisDone, base, PLANNING},
		{"analysis_failed", ANALYZING, EAnalysisFailed, base, PARKED},
		{"analysis_force_park", ANALYZING, EForcePark, base, PARKED},
		{"plan_ready", PLANNING, EPlanReady, Context{Now: now, Actor: "alice", PlanComplete: true}, WAIT_GATE},
		{"plan_failed", PLANNING, EPlanFailed, base, PARKED},
		{"plan_force_park", PLANNING, EForcePark, base, PARKED},
		{"gate_approved", WAIT_GATE, EGateApproved, Context{Now: now, Actor: "alice", GateApproved: true, ChangeWindow: true}, EXECUTING},
		{"gate_rejected", WAIT_GATE, EGateRejected, base, PARKED},
		{"gate_force_park", WAIT_GATE, EForcePark, base, PARKED},
		{"exec_done", EXECUTING, EExecDone, base, VERIFYING},
		{"exec_failed", EXECUTING, EExecFailed, base, PARKED},
		{"exec_force_park", EXECUTING, EForcePark, base, PARKED},
		{"verify_pass", VERIFYING, EVerifyPass, Context{Now: now, Actor: "alice", VerifyPassed: true}, CLOSED},
		{"verify_failed", VERIFYING, EVerifyFailed, base, PARKED},
		{"verify_force_park", VERIFYING, EForcePark, base, PARKED},
		{"parked_restart_analysis", PARKED, EStartAnalysis, base, ANALYZING},
		{"parked_plan_ready", PARKED, EPlanReady, Context{Now: now, Actor: "alice", PlanComplete: true}, WAIT_GATE},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			intent, err := Decide(tc.from, tc.event, tc.ctx, "case-1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if intent.From != tc.from {
				t.Errorf("from = %v want %v", intent.From, tc.from)
			}
			if intent.To != tc.want {
				t.Errorf("to = %v want %v", intent.To, tc.want)
			}
			if intent.Event != tc.event {
				t.Errorf("event = %v want %v", intent.Event, tc.event)
			}
			if intent.Timeline.Actor != tc.ctx.Actor {
				t.Errorf("timeline actor = %v want %v", intent.Timeline.Actor, tc.ctx.Actor)
			}
			if len(intent.Messages) != 1 {
				t.Fatalf("expected 1 message, got %d", len(intent.Messages))
			}
			msg := intent.Messages[0]
			if msg.Topic != "evt.case.transition.v1" {
				t.Errorf("topic = %s", msg.Topic)
			}
			if msg.Payload["case_id"] != "case-1" {
				t.Errorf("case_id = %v", msg.Payload["case_id"])
			}
			if msg.Payload["from"] != tc.from {
				t.Errorf("payload from = %v want %v", msg.Payload["from"], tc.from)
			}
			if msg.Payload["to"] != tc.want {
				t.Errorf("payload to = %v want %v", msg.Payload["to"], tc.want)
			}
			if msg.Payload["event"] != tc.event {
				t.Errorf("payload event = %v want %v", msg.Payload["event"], tc.event)
			}
		})
	}
}

func TestDecideIllegalTransition(t *testing.T) {
	_, err := Decide(NEW, EAnalysisDone, Context{}, "case-1")
	if !errors.Is(err, ErrIllegal) {
		t.Fatalf("expected ErrIllegal got %v", err)
	}
}

func TestDecideGuardFailures(t *testing.T) {
	now := time.Unix(0, 0)
	cases := []struct {
		name  string
		from  State
		event Event
		ctx   Context
	}{
		{"plan_ready_without_plan", PLANNING, EPlanReady, Context{Now: now, Actor: "alice"}},
		{"gate_approved_without_approval", WAIT_GATE, EGateApproved, Context{Now: now, Actor: "alice", ChangeWindow: true}},
		{"gate_approved_outside_window", WAIT_GATE, EGateApproved, Context{Now: now, Actor: "alice", GateApproved: true}},
		{"verify_pass_without_slo", VERIFYING, EVerifyPass, Context{Now: now, Actor: "alice"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decide(tc.from, tc.event, tc.ctx, "case-1")
			if !errors.Is(err, ErrGuard) {
				t.Fatalf("expected ErrGuard got %v", err)
			}
		})
	}
}
