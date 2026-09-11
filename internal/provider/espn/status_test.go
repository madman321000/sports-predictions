package espn

import "testing"

func TestGameStatus(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		completed   bool
		want        string
	}{
		{"STATUS_SCHEDULED", "pre", false, "scheduled"},
		{"STATUS_HALFTIME", "in", false, "in_progress"},
		{"STATUS_FINAL_OVERTIME", "post", true, "final"},
		{"STATUS_POSTPONED", "pre", false, "postponed"},
		{"STATUS_CANCELED", "post", false, "canceled"},
		{"STATUS_SUSPENDED", "in", false, "suspended"},
		{"STATUS_DELAYED", "pre", false, "delayed"},
	} {
		got, err := gameStatus(tc.name, tc.state, tc.completed)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %s, %v", tc.name, got, err)
		}
	}
	if _, err := gameStatus("STATUS_UNKNOWN", "pre", false); err == nil {
		t.Fatal("unknown status should fail")
	}
}
