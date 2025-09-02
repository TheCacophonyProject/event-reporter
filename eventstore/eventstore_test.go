/*
event-reporter - report events to the Cacophony Project API.
Copyright (C) 2018, The Cacophony Project

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package eventstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite

	tempDir string
	store   *EventStore
}

func (s *Suite) SetupTest() {
	tempDir, err := os.MkdirTemp(os.TempDir(), "eventstore_test")
	s.Require().NoError(err)
	s.tempDir = tempDir

	s.store = s.openStore()
}

func (s *Suite) openStore() *EventStore {
	store, err := Open(filepath.Join(s.tempDir, "store.db"), "info")
	s.Require().NoError(err)
	return store
}

func (s *Suite) TearDownTest() {
	if s.store != nil {
		s.store.Close()
		s.store = nil
	}
	if s.tempDir != "" {
		os.RemoveAll(s.tempDir)
		s.tempDir = ""
	}
}

func (s *Suite) TestAddAndGet() {
	time1 := Now()
	time2 := Now().Add(time.Second)
	time3 := Now().Add(2 * time.Second)
	events := map[int64]Event{
		time1.Unix(): {
			Description: EventDescription{Details: map[string]interface{}{"file": "abc"}, Type: "type1"},
			Timestamp:   time1,
		},
		time2.Unix(): {
			Timestamp:   time2,
			Description: EventDescription{Details: map[string]interface{}{"file": "abc"}, Type: "type1"},
		},
		time3.Unix(): {
			Timestamp:   time3,
			Description: EventDescription{Details: map[string]interface{}{"file": "abc"}, Type: "type1"},
		},
	}
	// Test adding data
	for _, e := range events {
		s.NoError(s.store.Add(&e), "error with adding data")
	}

	// Test GetKeys
	keys, err := s.store.GetKeys()
	s.NoError(err, "error returned when getting all keys")
	s.Equal(len(events), len(keys), "error with number of keys returned")

	// Test deleting and getting data
	deleteKey := keys[0]
	deletedEventBytes, err := s.store.Get(deleteKey)
	s.NoError(err, "error returned when deleting data")
	deletedEvent := &Event{}
	json.Unmarshal(deletedEventBytes, deletedEvent)
	s.NoError(s.store.Delete(deleteKey))
	delete(events, deletedEvent.Timestamp.Unix())
	keys, err = s.store.GetKeys()
	s.NoError(err, "error returned when gettign all keys")

	// Read all keys and check against initial data upload to DB
	for _, key := range keys {
		eventBytes, err := s.store.Get(key)
		s.NoError(err)
		s.NotNil(eventBytes)
		event := &Event{}
		s.NoError(json.Unmarshal(eventBytes, event))
		s.Equal(event.Timestamp.Unix(), events[event.Timestamp.Unix()].Timestamp.Unix())
		s.Equal(event.Description, events[event.Timestamp.Unix()].Description)
		delete(events, event.Timestamp.Unix()) // Delete data to check that there is no double up
	}
	// There should be no data missed
	s.Equal(0, len(events))
	log.Println(events)
}

func (s *Suite) TestRateLimit() {
	// Events are close to each other so should be rate limited.
	times := []time.Time{
		Now(),
		Now().Add(time.Second),
		Now().Add(2 * time.Second),
		Now().Add(3 * time.Second),
		Now().Add(4 * time.Second),
		Now().Add(6 * time.Second),
		Now().Add(7 * time.Second),
	}

	description := EventDescription{Details: map[string]interface{}{"file": "abc"}, Type: "rate_limit_check"}

	for _, t := range times {
		event := Event{
			Timestamp:   t,
			Description: description,
		}
		s.NoError(s.store.Add(&event))
	}

	// Test GetKeys
	keys, err := s.store.GetKeys()
	s.NoError(err, "error returned when getting all keys")
	// Check 5 events + 1 rate limit event
	s.Equal(5+1, len(keys), "error with number of keys returned")

	// Check that there was a rate limit event
	rateLimitEvent := false
	for _, key := range keys {
		eventBytes, err := s.store.Get(key)
		s.NoError(err)
		event := &Event{}
		s.NoError(json.Unmarshal(eventBytes, event))
		if event.Description.Type == "rate_limit" {
			rateLimitEvent = true
		}
	}
	if !rateLimitEvent {
		s.Fail("Rate limit event not found")
	}
}

func (s *Suite) TestNoRateLimit() {
	// Events are far apart from each other so shouldn't be rate limited.
	times := []time.Time{
		Now(),
		Now().Add(time.Hour),
		Now().Add(2 * time.Hour),
		Now().Add(3 * time.Hour),
		Now().Add(4 * time.Hour),
		Now().Add(6 * time.Hour),
		Now().Add(7 * time.Hour),
	}

	description := EventDescription{Details: map[string]interface{}{"file": "abc"}, Type: "rate_limit_check"}

	for _, t := range times {
		event := Event{
			Timestamp:   t,
			Description: description,
		}
		s.NoError(s.store.Add(&event))
	}

	// Test GetKeys
	keys, err := s.store.GetKeys()
	s.NoError(err, "error returned when getting all keys")
	// Check 5 events + 1 rate limit event
	s.Equal(7, len(keys), "error with number of keys returned")

	// Check that there was a rate limit event
	rateLimitEvent := false
	for _, key := range keys {
		eventBytes, err := s.store.Get(key)
		s.NoError(err)
		event := &Event{}
		s.NoError(json.Unmarshal(eventBytes, event))
		if event.Description.Type == "rate_limit" {
			rateLimitEvent = true
		}
	}
	if rateLimitEvent {
		s.Fail("Rate limit event found")
	}
}

func (s *Suite) TestRateLimitThenNoRateLimit() {
	// Event will be rate limited then not rate limited.

	// Intervals between events.
	durations := []time.Duration{
		time.Minute,
		time.Minute,
		time.Minute,
		time.Minute,
		time.Minute,
		time.Minute,
		time.Minute,
		time.Minute,
		time.Minute,
		time.Minute,
		time.Minute,
		1 * time.Hour,
		time.Minute,
		time.Minute,
		time.Minute,
		time.Minute,
	}

	// Make event times
	eventTime := time.Now()
	times := []time.Time{eventTime}
	for _, d := range durations {
		eventTime = eventTime.Add(d)
		times = append(times, eventTime)
	}

	// Make events
	description := EventDescription{Details: map[string]interface{}{"file": "abc"}, Type: "rate_limit_check"}
	for _, t := range times {
		event := Event{
			Timestamp:   t,
			Description: description,
		}
		s.NoError(s.store.Add(&event))
	}

	// Test GetKeys
	keys, err := s.store.GetKeys()
	s.NoError(err, "error returned when getting all keys")
	// Check 10 events + 1 rate limit event
	s.Equal(11, len(keys), "error with number of keys returned")

	// Check that there was a rate limit event
	rateLimitEvent := false
	for _, key := range keys {
		eventBytes, err := s.store.Get(key)
		s.NoError(err)
		event := &Event{}
		s.NoError(json.Unmarshal(eventBytes, event))
		if event.Description.Type == "rate_limit" {
			rateLimitEvent = true
		}
	}
	if !rateLimitEvent {
		s.Fail("Rate limit event not found")
	}
}

func TestRun(t *testing.T) {
	suite.Run(t, new(Suite))
}

func Now() time.Time {
	// Truncate necessary to get rid of monotonic clock reading.
	return time.Now().Truncate(time.Second)
}
