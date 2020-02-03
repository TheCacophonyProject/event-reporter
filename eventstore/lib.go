/*
management-interface - Web based management of Raspberry Pis over WiFi
Copyright (C) 2020, The Cacophony Project

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/

package eventstore

import (
	"encoding/json"
	"errors"

	"github.com/godbus/dbus"
)

func AddEvent(event Event) error {
	detailsBytes, err := json.Marshal(event.Description.Details)
	if err != nil {
		return err
	}
	_, err = eventsDbusCall(
		"org.cacophony.Events.Add",
		string(detailsBytes),
		event.Description.Type,
		event.Timestamp.UnixNano())
	return err
}

func GetEventKeys() ([]uint64, error) {
	data, err := eventsDbusCall("org.cacophony.Events.GetKeys")
	if err != nil {
		return nil, err
	}
	if len(data) != 1 {
		return nil, errors.New("error getting event keys")
	}
	eventKeys, ok := data[0].([]uint64)
	if !ok {
		return nil, errors.New("error reading event keys")
	}
	return eventKeys, nil
}

func GetEvent(key uint64) (*Event, error) {
	data, err := eventsDbusCall("org.cacophony.Events.Get", key)
	if err != nil {
		return nil, err
	}
	if len(data) != 1 {
		return nil, errors.New("error getting event data")
	}
	eventString, ok := data[0].(string)
	if !ok {
		return nil, errors.New("error reading event data")
	}
	var event Event
	if err := json.Unmarshal([]byte(eventString), &event); err != nil {
		return nil, err
	}
	return &event, nil
}

func DeleteEvent(key uint64) error {
	_, err := eventsDbusCall("org.cacophony.Events.Delete", key)
	return err
}

func eventsDbusCall(method string, params ...interface{}) ([]interface{}, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, err
	}
	obj := conn.Object("org.cacophony.Events", "/org/cacophony/Events")
	call := obj.Call(method, 0, params...)
	return call.Body, call.Err
}
