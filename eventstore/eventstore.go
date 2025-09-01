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
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TheCacophonyProject/go-utils/logging"
	"github.com/boltdb/bolt"
)

const (
	openTimeout = 5 * time.Second
)

var oldBucketName = []byte("events")
var idDataBucketName = []byte("id-data-events") // Bucket with the key being a uint64 and the value being a json
var log = logging.NewLogger("info")

// EventStore perists details for events which are to be sent to the
// Cacophony Events API.
type EventStore struct {
	db *bolt.DB
}

// Open opens the event store. It should be closed later with the
// Close() method.
func Open(fileName string) (*EventStore, error) {
	db, err := bolt.Open(fileName, 0600, &bolt.Options{Timeout: openTimeout})
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(oldBucketName)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(idDataBucketName)
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("creating bucket: %v", err)
	}

	return &EventStore{db: db}, nil
}

// Use Add for adding new events now. This is keept for testing migrations
// Queue recordings an event in the event store. The details provided
// uniquely identify the event, but the contents are opaque to the
// event store.
func (s *EventStore) Queue(details []byte, timestamp time.Time) error {
	log.Printf("adding new event: '%s'", string(details))
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(oldBucketName)
		if bucket == nil {
			return noBucketErr(oldBucketName)
		}
		rec := bucket.Get(details)

		var writer *bytes.Buffer
		if rec == nil {
			// First addition of this event type.
			writer = new(bytes.Buffer)
			writer.Write([]byte{0}) // version number
		} else {
			writer = bytes.NewBuffer(rec)
		}

		binary.Write(writer, binary.LittleEndian, timestamp.UnixNano())

		return bucket.Put(details, writer.Bytes())
	})
}

type Event struct {
	Timestamp   time.Time
	Description EventDescription `json:"description"`
}

type EventDescription struct {
	Type    string                 `json:"type"`
	Details map[string]interface{} `json:"details"`
}

func (s *EventStore) Add(event *Event) error {
	log.Printf("Adding new '%s' event\n", event.Description.Type)
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(idDataBucketName)
		if bucket == nil {
			return noBucketErr(idDataBucketName)
		}
		nextSeq, err := bucket.NextSequence()
		if err != nil {
			return err
		}
		return bucket.Put(uint64ToBytes(nextSeq), data)
	})
}

func uint64ToBytes(i uint64) []byte {
	key := make([]byte, 8)
	binary.LittleEndian.PutUint64(key, i)
	return key
}

func bytesToUint64(b []byte) uint64 {
	return binary.LittleEndian.Uint64(b)
}

func (s *EventStore) Get(key uint64) ([]byte, error) {
	var val []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(idDataBucketName)
		if bucket == nil {
			return noBucketErr(idDataBucketName)
		}
		val = bucket.Get(uint64ToBytes(key))
		if val == nil {
			return fmt.Errorf("no key %v found", key)
		}
		return nil
	})
	return val, err
}

func (s *EventStore) GetKeys() ([]uint64, error) {
	keys := []uint64{}
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(idDataBucketName)
		if bucket == nil {
			return noBucketErr(idDataBucketName)
		}
		return bucket.ForEach(func(k, v []byte) error {
			keys = append(keys, bytesToUint64(k))
			return nil
		})
	})
	return keys, err
}

func noBucketErr(name []byte) error {
	return fmt.Errorf("no event bucket found called '%s'", name)
}

func (s *EventStore) Delete(key uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(idDataBucketName)
		if bucket == nil {
			return noBucketErr(idDataBucketName)
		}
		return bucket.Delete(uint64ToBytes(key))
	})
}

func (s *EventStore) DeleteKeys(keys []uint64) error {
	for _, key := range keys {
		if err := s.Delete(key); err != nil {
			return err
		}
	}
	return nil
}

// All returns all the events stored in the event store as EventTimes
// instances. Events with identical details are grouped together into
// a single EventTimes instance.
func (s *EventStore) All() ([]EventTimes, error) {
	var out []EventTimes
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(oldBucketName)
		cursor := bucket.Cursor()

		for key, rec := cursor.First(); key != nil; key, rec = cursor.Next() {
			var version byte
			reader := bytes.NewReader(rec)
			err := binary.Read(reader, binary.LittleEndian, &version)
			if err != nil {
				return fmt.Errorf("failed to read version: %v", err)
			}
			if version != 0 {
				return fmt.Errorf("unsupported version: %v", version)
			}

			var timestamps []time.Time
			for {
				var nanos int64
				err := binary.Read(reader, binary.LittleEndian, &nanos)
				if err != nil {
					break
				}
				timestamps = append(timestamps, time.Unix(0, nanos))
			}

			out = append(out, newEventTimes(key, timestamps...))
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Discard removes an event from from the store.
func (s *EventStore) Discard(ev EventTimes) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(oldBucketName)
		return bucket.Delete(ev.Details)
	})
}

// Close releases resources used by the EventStore. It should be
// called once the EventStore is no longer required.
func (s *EventStore) Close() {
	s.db.Close()
}

func newEventTimes(details []byte, times ...time.Time) EventTimes {
	ev := EventTimes{
		Details:    make([]byte, len(details)),
		Timestamps: times,
	}
	copy(ev.Details, details)
	return ev
}

// EventTimes holds
type EventTimes struct {
	Details    []byte
	Timestamps []time.Time
}
