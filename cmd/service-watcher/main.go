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

	"github.com/TheCacophonyProject/event-reporter/v3/eventclient"
	systemdbus "github.com/coreos/go-systemd/dbus"
)

const (
	minTimeBetweenReports = 20 * time.Minute //TODO add into cacophony-config
	numLogLines           = 20               //TODO add into cacophony-config
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
		log.Printf("failed to connect to dbus: %v", err)
		return err
	}
	log.Println("connected to systemdbus")

	defer conn.Close()

	if err := conn.Subscribe(); err != nil {
		log.Printf("failed to subscribe to the dbus: %v", err)
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
			if !isInterestingState(activeState) {
				break
			}
			if t, ok := lastUnitReportTimes[unitName]; ok && time.Now().Sub(t) < minTimeBetweenReports {
				log.Println("reporting too often")
				break
			}

			rawLogs, failed, err := getLogs(unitName, numLogLines)
			if err != nil {
				return err
			}
			if !failed {
				break // Can just be a service activating
			}

			log.Printf("service failed. unitname: %s, activeState: %s", unitName, activeState)
			for _, l := range rawLogs {
				log.Println(l)
			}

			event := eventclient.Event{
				Timestamp: ts,
				Type:      "systemError",
				Details: map[string]interface{}{
					"version":     1,
					"unitName":    unitName,
					"logs":        rawLogs,
					"activeState": activeState,
				},
			}
			if err != nil {
				return err
			}
			if err := eventclient.AddEvent(event); err != nil {
				return err
			}
			lastUnitReportTimes[unitName] = time.Now()

		case err := <-errCh:
			log.Printf("error reading systemd property change: %v", err)
			return err
		}
	}
}

func getLogs(unitName string, numLines int) ([]string, bool, error) {
	failed := false
	cmd := exec.Command(
		"journalctl",
		"-u", unitName,
		"--output=json",
		"-n", strconv.Itoa(numLines))
	out, err := cmd.Output()
	if err != nil {
		return nil, false, err
	}
	strLogs := strings.Split(strings.Trim(string(out), "\n"), "\n")
	var logs []string

	for _, strlog := range strLogs {
		var rawLog LogsRaw
		if err := json.Unmarshal([]byte(strlog), &rawLog); err != nil {
			return nil, false, err
		}
		// Only get logs from this session
		if rawLog.SystemdUnit == "init.scope" {
			if strings.Contains(rawLog.Message, "Started") {
				logs = nil
				failed = false
			}
			if strings.Contains(rawLog.Message, "Unit entered failed state.") {
				failed = true
			}
		}
		logs = append(logs, rawLog.Message)
	}
	return logs, failed, nil
}

func isInterestingState(state string) bool {
	switch state {
	case
		//"active",			// triggered when service exec is started
		"activating", // If service is set to restart this is called after the service exec exits
		"failed":     // If service is set to restart the service won't 'fail'.
		return true
	}
	return false
}
