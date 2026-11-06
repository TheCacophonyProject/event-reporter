package eventreporter

import (
	"testing"
	"time"

	"github.com/TheCacophonyProject/go-utils/logging"
)

func TestUploadScheduleResetsWhenEventsAreSent(t *testing.T) {
	start := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	s := newUploadSchedule(12*time.Hour, start)

	if got := s.wait(start.Add(time.Hour)); got != 11*time.Hour {
		t.Errorf("wait = %v, want 11h", got)
	}

	// Sending events outside the schedule moves it back, so it isn't reached while events keep
	// being sent over connections made for other reasons.
	s.uploaded(start.Add(10 * time.Hour))
	if got := s.wait(start.Add(11 * time.Hour)); got != 11*time.Hour {
		t.Errorf("wait after sending = %v, want 11h", got)
	}

	if got := s.wait(start.Add(48 * time.Hour)); got != 0 {
		t.Errorf("wait once overdue = %v, want 0", got)
	}
}

// Without a connection, events must wait rather than request one, as that can power on the modem.
func TestSendEventsWithoutConnectionDoesNotRequestOne(t *testing.T) {
	if log == nil {
		log = logging.NewLogger("info")
	}
	old := checkConnection
	t.Cleanup(func() { checkConnection = old })
	checks := 0
	checkConnection = func() bool {
		checks++
		return false
	}

	// A nil store and connection requester would panic if they were used.
	if sent := sendEvents(nil, []uint64{1, 2}, nil, false); sent != 0 {
		t.Errorf("sent = %d, want 0", sent)
	}
	if checks != 1 {
		t.Errorf("checked for a connection %d times, want 1", checks)
	}
}

func TestNotifyNewEventDoesNotBlock(t *testing.T) {
	svc := &service{newEventChan: make(chan bool, 1)}
	done := make(chan struct{})
	go func() {
		for range 5 {
			svc.notifyNewEvent()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("notifyNewEvent blocked")
	}
	if len(svc.newEventChan) != 1 {
		t.Errorf("pending notifications = %d, want 1", len(svc.newEventChan))
	}
}
