package cutover

import "testing"

func TestDecideCutover(t *testing.T) {
	base := Request{Domain: "pay.example.com", Name: "@", NewContent: "203.0.113.20", OldContent: "203.0.113.10", TTL: 60, Risk: "low", PaymentEvents: []PaymentEvent{{ID: "evt-1", Kind: "authorized", Amount: 1250, Currency: "USD"}}}
	tests := []struct {
		name, risk, status, action, content string
		rollback                            bool
	}{
		{name: "low risk cutover", risk: "low", status: "approved", action: "cutover", content: "203.0.113.20"},
		{name: "medium risk needs review", risk: "medium", status: "review", action: "cutover", content: "203.0.113.20"},
		{name: "high risk is blocked", risk: "high", status: "blocked", action: "cutover", content: "203.0.113.20"},
		{name: "rollback restores prior target", risk: "low", status: "approved", action: "rollback", content: "203.0.113.10", rollback: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.Risk, r.Rollback = tc.risk, tc.rollback
			got, err := Decide(r)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.status || got.Action != tc.action || got.Content != tc.content {
				t.Fatalf("got status=%s action=%s content=%s", got.Status, got.Action, got.Content)
			}
			if got.EventCount != 1 || got.PaymentVolume != 1250 || len(got.Notifications) != 2 {
				t.Fatalf("audit summary was not preserved: %+v", got)
			}
		})
	}
}
