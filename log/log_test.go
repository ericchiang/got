package log

import "testing"

func TestLevel(t *testing.T) {
	l := New(Info).(*logger)
	if l.debug != nil {
		t.Errorf("expected log level 'Info' to disable debug logging")
	}
}
