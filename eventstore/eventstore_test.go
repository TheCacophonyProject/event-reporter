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
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/boltdb/bolt"
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

// TestMigrate will add data to the old bucket using the old method then try to
// migrate the data to the new bucket. Will compare results.
func (s *Suite) TestMigrate() {
	err := s.store.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(oldBucketName)
		return err
	})
	s.NoError(err)

	t := Now()
	e := map[int64]map[string]interface{}{
		t.Unix(): {
			"details": map[string]interface{}{
				"fileId": "cat1",
				"volume": "1",
			},
			"type": "audiobait1",
		},
		t.Add(time.Second).Unix(): {
			"details": map[string]interface{}{
				"fileId": "bird2",
				"volume": "2",
			},
			"type": "audiobait2",
		},
		t.Add(time.Second).Unix(): {
			"type": "audiobait2",
		},
	}
	log.Printf("events to migrate %+v", e)
	// Adding some event using the old method
	for t, i := range e {
		eventDetails := map[string]interface{}{
			"description": map[string]interface{}{
				"type":    i["type"],
				"details": i["details"],
			},
		}
		detailsJSON1, err := json.Marshal(&eventDetails)
		s.NoError(err)
		s.NoError(s.store.Queue(detailsJSON1, time.Unix(t, 0)))
	}

	// Close and reopen store, the migration happens when the store is opened so
	// that is why it is closed and opened again
	s.store.Close()
	store2 := s.openStore()
	keys, err := store2.GetKeys()
	s.NoError(err)
	// Check that each event did a proper migration
	for _, key := range keys {
		detailsBytes, err := store2.Get(key)
		s.NoError(err)
		event := &Event{}
		s.NoError(json.Unmarshal(detailsBytes, event))
		i := e[event.Timestamp.Unix()]
		log.Printf("event time %d", event.Timestamp.Unix())
		s.Equal(i["type"], event.Description.Type) // Checkign that type was properly migrated
		if _, ok := i["details"]; ok {             // Only compare details if origional data had details
			s.Equal(i["details"], event.Description.Details) // Checkign that details was properly migrated
		}
	}

	// Check that migrated events are deleted
	eventTimes, err := store2.All() // Old way of getting events
	s.NoError(err)
	s.Equal(len(eventTimes), 0)
}

func (s *Suite) TestAddAndGet() {
	time1 := Now()
	time2 := Now().Add(time.Second)
	time3 := Now().Add(2 * time.Second)
	events := map[int64]Event{
		time1.Unix(): Event{
			Description: EventDescription{Details: map[string]interface{}{"file": "abc"}, Type: "type1"},
			Timestamp:   time1,
		},
		time2.Unix(): Event{
			Timestamp:   time2,
			Description: EventDescription{Details: map[string]interface{}{"file": "abc"}, Type: "type1"},
		},
		time3.Unix(): Event{
			Timestamp:   time3,
			Description: EventDescription{Details: map[string]interface{}{"file": "abc"}, Type: "type1"},
		},
	}
	// Test addign data
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

func TestRun(t *testing.T) {
	suite.Run(t, new(Suite))
}

func Now() time.Time {
	// Truncate necessary to get rid of monotonic clock reading.
	return time.Now().Truncate(time.Second)
}
