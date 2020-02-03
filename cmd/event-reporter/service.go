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

package main

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/godbus/dbus"
	"github.com/godbus/dbus/introspect"

	"github.com/TheCacophonyProject/event-reporter/eventstore"
)

const dbusName = "org.cacophony.Events"
const dbusPath = "/org/cacophony/Events"

// StartService exposes an instance of `service` (see below) on the
// system DBUS. This allows other processes to queue events for
// sending.
func StartService(store *eventstore.EventStore) error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return err
	}
	reply, err := conn.RequestName(dbusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return errors.New("name already taken")
	}

	svc := &service{
		store: store,
	}
	conn.Export(svc, dbusPath, dbusName)
	conn.Export(genIntrospectable(svc), dbusPath, "org.freedesktop.DBus.Introspectable")
	return nil
}

func genIntrospectable(v interface{}) introspect.Introspectable {
	node := &introspect.Node{
		Interfaces: []introspect.Interface{{
			Name:    dbusName,
			Methods: introspect.Methods(v),
		}},
	}
	return introspect.NewIntrospectable(node)
}

type service struct {
	store *eventstore.EventStore
}

// Queue is deprecated. Use Add from now on
// Queue adds an event to the event store. It is exposed over DBUS.
// The event details must be supplied as JSON encoded bytes and the
// timestamp as the number of nanoseconds since 1970-01-01 UTC.
func (svc *service) Queue(details []byte, nanos int64) *dbus.Error {
	err := svc.store.Queue(details, time.Unix(0, nanos))
	if err != nil {
		return &dbus.Error{
			Name: dbusName + ".Errors.QueueFailed",
			Body: []interface{}{err.Error()},
		}
	}
	return nil
}

func (svc *service) Add(detailsRaw string, eventType string, unixNsec int64) *dbus.Error {
	details := map[string]interface{}{}
	if err := json.Unmarshal([]byte(detailsRaw), &details); err != nil {
		return dbusErr("", err)
	}

	event := &eventstore.Event{
		Timestamp: time.Unix(0, unixNsec),
		Description: eventstore.EventDescription{
			Details: details,
			Type:    eventType,
		},
	}

	return dbusErr(".Errors.AddFailed", svc.store.Add(event))
}

func dbusErr(name string, err error) *dbus.Error {
	if err == nil {
		return nil
	}
	return &dbus.Error{
		Name: dbusName + name,
		Body: []interface{}{err.Error()},
	}
}

func (svc *service) Get(key uint64) (string, *dbus.Error) {
	data, err := svc.store.Get(key)
	if err != nil {
		return "", dbusErr(".Errors.GetFailed", err)
	}
	return string(data), nil
}

func (svc *service) GetKeys() ([]uint64, *dbus.Error) {
	keys, err := svc.store.GetKeys()
	if err != nil {
		return nil, dbusErr(".Errors.GetKeysFailed", err)
	}
	return keys, nil
}

func (svc *service) Delete(key uint64) *dbus.Error {
	return dbusErr(".Errors.DeleteFailed", svc.store.Delete(key))
}
