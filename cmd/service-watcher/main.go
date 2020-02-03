/*
event-reporter - report events to the Cacophony Project API.
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
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"encoding/json"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	systemdbus "github.com/coreos/go-systemd/dbus"

	"github.com/TheCacophonyProject/event-reporter/eventstore"
)

const (
	minTimeBetweenReports = time.Minute
)

type LogReport struct {
	Logs        *[]string
	Time        int64
	UnitName    string
	ActiveState string
}

type LogsRaw struct {
	Unit              string `json:"UNIT"`
	SystemdUnit       string `json:"_SYSTEMD_UNIT"`
	RealtimeTimestamp string `json:"__REALTIME_TIMESTAMP"`
	Message           string `json:"MESSAGE"`
}

func main() {
	err := runMain()
	if err != nil {
		log.Fatal(err.Error())
	}
}

func runMain() error {
	log.SetFlags(0) // Removes default timestamp flag

	conn, err := systemdbus.New()
	if err != nil {
		log.Println("failed to connect to dbus")
		return err
	}

	defer conn.Close()

	if err := conn.Subscribe(); err != nil {
		log.Println("failed to subscribe to the dbus")
		return err
	}

	updateCh := make(chan *systemdbus.PropertiesUpdate, 256)
	errCh := make(chan error, 256)
	conn.SetPropertiesSubscriber(updateCh, errCh)

	lastUnitReportTimes := map[string]time.Time{}

	for {
		select {
		case update := <-updateCh:
			ts := time.Now()
			activeState := strings.Trim(update.Changed["ActiveState"].String(), "\"")
			unitName := update.UnitName
			// Only process states we are interested in
			if !inActiveStates(activeState) {
				break
			}
			if t, ok := lastUnitReportTimes[unitName]; ok && time.Now().Sub(t) < minTimeBetweenReports {
				log.Println("reporting too often")
				break
			}

			log.Println("unitname:", unitName)
			log.Println("activeState:", activeState)

			rawLogs, failed, err := getLogs(update.UnitName, 20)
			if err != nil {
				return err
			}
			if !failed {
				break // Can just be a service activating
			}

			event := eventstore.Event{
				Timestamp: ts,
				Description: eventstore.EventDescription{
					Type: "systemError",
					Details: map[string]interface{}{
						"versoin":     1,
						"unitName":    unitName,
						"logs":        rawLogs,
						"activeState": activeState,
					},
				},
			}
			if err != nil {
				return err
			}
			if err := eventstore.AddEvent(event); err != nil {
				return err
			}
			lastUnitReportTimes[unitName] = time.Now()

		case err := <-errCh:
			log.Println("err:", err)
		}
	}
}

func getLogs(unitName string, number int) (*[]string, bool, error) {
	failed := false
	cmd := exec.Command(
		"journalctl",
		"-u", unitName,
		"--output=json",
		"-n", strconv.Itoa(number))
	out, err := cmd.Output()
	if err != nil {
		return nil, false, err
	}
	strLogs := strings.Split(string(out), "\n")
	logs := []string{}

	for _, strlog := range strLogs {
		var rawLog LogsRaw
		json.Unmarshal([]byte(strlog), &rawLog)
		// Only get logs from this session
		if rawLog.SystemdUnit == "init.scope" {
			if strings.Contains(rawLog.Message, "Started") {
				logs = []string{}
				failed = false
			}
			if strings.Contains(rawLog.Message, "Unit entered failed state.") {
				failed = true
			}
		}
		logs = append(logs, rawLog.Message)
	}
	for _, l := range logs {
		log.Println(l)
	}
	return &logs, failed, nil
}

func inActiveStates(state string) bool {
	switch state {
	case
		//"active",			// triggered when service exec is started
		"activating", // If service is set to restart this is called after the service exec exits
		"failed":     // If service is set to restart the service won't 'fail'.
		return true
	}
	return false
}
