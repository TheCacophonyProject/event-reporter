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
	"io/ioutil"
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

func (suite *Suite) SetupTest() {
	tempDir, err := ioutil.TempDir(os.TempDir(), "eventstore_test")
	suite.Require().NoError(err)
	suite.tempDir = tempDir

	suite.store = suite.openStore()
}

func (suite *Suite) openStore() *EventStore {
	store, err := Open(filepath.Join(suite.tempDir, "store.db"))
	suite.Require().NoError(err)
	return store
}

func (suite *Suite) TearDownTest() {
	if suite.store != nil {
		suite.store.Close()
		suite.store = nil
	}
	if suite.tempDir != "" {
		os.RemoveAll(suite.tempDir)
		suite.tempDir = ""
	}
}

func (suite *Suite) TestBasics() {
	now := Now()
	details := []byte("foo")
	err := suite.store.Queue(details, now)
	suite.NoError(err)

	events, err := suite.store.All()
	suite.NoError(err)
	suite.Len(events, 1)
	suite.Equal(events[0], newEventTimes(details, now))
}

func (suite *Suite) TestPersists() {
	now := Now()
	details := []byte("foo")
	err := suite.store.Queue(details, now)
	suite.NoError(err)
	suite.store.Close()

	store2 := suite.openStore()
	events, err := store2.All()
	suite.NoError(err)
	suite.Len(events, 1)
	suite.Equal(events[0], newEventTimes(details, now))
}

func (suite *Suite) TestOneEventMultipleTimes() {
	details := []byte("foo")

	now0 := Now()
	err := suite.store.Queue(details, now0)
	suite.NoError(err)

	now1 := now0.Add(time.Second)
	err = suite.store.Queue(details, now1)
	suite.NoError(err)

	events, err := suite.store.All()
	suite.NoError(err)
	suite.Len(events, 1)

	suite.Equal(events[0], newEventTimes(details, now0, now1))
}

func (suite *Suite) TestMultipleEvents() {
	// Queue event 0
	details0 := []byte("foo")
	now0 := Now()
	err := suite.store.Queue(details0, now0)
	suite.NoError(err)

	// Queue event 1
	details1 := []byte("bar")
	now1 := now0.Add(time.Second)
	err = suite.store.Queue(details1, now1)
	suite.NoError(err)

	// See that they're both stored.
	events, err := suite.store.All()
	suite.NoError(err)
	suite.Len(events, 2)

	expected := []EventTimes{
		newEventTimes(details0, now0),
		newEventTimes(details1, now1),
	}
	suite.ElementsMatch(events, expected)
}

func (suite *Suite) TestDiscard() {
	// Queue event 0
	details0 := []byte("foo")
	now0 := Now()
	err := suite.store.Queue(details0, now0)
	suite.NoError(err)

	// Queue event 1
	details1 := []byte("bar")
	now1 := now0.Add(time.Second)
	err = suite.store.Queue(details1, now1)
	suite.NoError(err)

	// See that they're both stored.
	events, err := suite.store.All()
	suite.NoError(err)
	suite.Len(events, 2)

	// Remove one
	err = suite.store.Discard(events[0])
	suite.NoError(err)

	eventsNow, err := suite.store.All()
	suite.NoError(err)
	suite.Len(eventsNow, 1)

	// Remove other
	err = suite.store.Discard(events[1])
	suite.NoError(err)

	eventsNow, err = suite.store.All()
	suite.NoError(err)
	suite.Len(eventsNow, 0)

	// Removing an already removed item is OK.
	err = suite.store.Discard(events[1])
	suite.NoError(err)
}

func TestRun(t *testing.T) {
	suite.Run(t, new(Suite))
}

func Now() time.Time {
	// Truncate necessary to get rid of monotonic clock reading.
	return time.Now().Truncate(time.Nanosecond)
}
