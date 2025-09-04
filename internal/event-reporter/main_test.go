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

package eventreporter

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/TheCacophonyProject/event-reporter/v3/eventstore"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite

	tempDir string
	store   *eventstore.EventStore
}

func (s *Suite) SetupTest() {
	tempDir, err := os.MkdirTemp(os.TempDir(), "eventstore_test")
	s.Require().NoError(err)
	s.tempDir = tempDir

	s.store = s.openStore()
}

func (s *Suite) openStore() *eventstore.EventStore {
	store, err := eventstore.Open(filepath.Join(s.tempDir, "store.db"), "info")
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

func TestRun(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestGroupingEvents() {
	// Make 345 events of the same type
	for i := range 123 {
		err := s.store.Add(&eventstore.Event{
			Timestamp:   time.Now().Add(time.Hour * time.Duration(i)),
			Description: eventstore.EventDescription{Details: map[string]any{"foo": "abc"}, Type: "type1"},
		})
		s.NoError(err, "error with adding events")
	}
	for i := range 234 {
		err := s.store.Add(&eventstore.Event{
			Timestamp:   time.Now().Add(time.Hour * time.Duration(i)),
			Description: eventstore.EventDescription{Details: map[string]any{"bar": "abc"}, Type: "type1"},
		})
		s.NoError(err, "error with adding events")
	}
	for i := range 6 {
		err := s.store.Add(&eventstore.Event{
			Timestamp:   time.Now().Add(time.Hour * time.Duration(i)),
			Description: eventstore.EventDescription{Details: map[string]any{"foobar": "abc"}, Type: "type1"},
		})
		s.NoError(err, "error with adding events")
	}

	eventKeys, err := s.store.GetKeys()
	s.NoError(err, "error returned when getting all keys")
	s.Equal(123+234+6, len(eventKeys), "error with number of keys returned")

	groupEvents, err := getGroupEvents(s.store, eventKeys)
	s.NoError(err, "error returned when getting group events")

	expectedGroupLens := []int{100, 23, 100, 100, 34, 6}
	sort.Ints(expectedGroupLens)
	groupEventsLengths := []int{}
	for _, group := range groupEvents {
		groupEventsLengths = append(groupEventsLengths, len(group.keys))
	}
	s.Equal(len(expectedGroupLens), len(groupEvents), "error with number of groups")
	sort.Ints(groupEventsLengths)
	for i := range groupEventsLengths {
		s.Equal(expectedGroupLens[i], groupEventsLengths[i], "error with number of events in group")
	}
}
