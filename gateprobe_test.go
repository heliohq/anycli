package anycli

import "testing"

// TestInspectGateProbe pins the design-318 harness contract at the public
// Inspect seam: the probe's single runnable leaf resolves to the action id
// gate-probe.probe_send with SideEffect true, so the consumer's policy layer
// gates it exactly like a real mutating command.
func TestInspectGateProbe(t *testing.T) {
	cases := []struct {
		name string
		args []string

		wantAction   string
		wantSide     bool
		wantRunnable bool
	}{
		{
			name:         "probe send is the pinned mutating leaf",
			args:         []string{"probe", "send"},
			wantAction:   "gate-probe.probe_send",
			wantSide:     true,
			wantRunnable: true,
		},
		{
			name:         "bare probe group is not runnable",
			args:         []string{"probe"},
			wantAction:   "gate-probe.probe",
			wantSide:     true, // group is unannotated; absent = true
			wantRunnable: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inv, err := Inspect("gate-probe", tc.args)
			if err != nil {
				t.Fatalf("Inspect(gate-probe, %v): %v", tc.args, err)
			}
			if inv.Action != tc.wantAction {
				t.Errorf("Action = %q, want %q", inv.Action, tc.wantAction)
			}
			if inv.SideEffect != tc.wantSide {
				t.Errorf("SideEffect = %v, want %v", inv.SideEffect, tc.wantSide)
			}
			if inv.Runnable != tc.wantRunnable {
				t.Errorf("Runnable = %v, want %v", inv.Runnable, tc.wantRunnable)
			}
			if !inv.Parsed {
				t.Errorf("Parsed = false, want true")
			}
		})
	}
}
