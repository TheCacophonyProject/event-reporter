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

package servicewatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/TheCacophonyProject/event-reporter/v3/eventclient"
	"github.com/TheCacophonyProject/go-utils/logging"
	"github.com/alexflint/go-arg"
	systemdbus "github.com/coreos/go-systemd/v22/dbus"
)

const (
	minTimeBetweenReports = 20 * time.Minute //TODO add into cacophony-config
	numLogLines           = 20               //TODO add into cacophony-config
)

var log = logging.NewLogger("info")
var version = "<not set>"

type Args struct {
	logging.LogArgs
}

func (Args) Version() string {
	return version
}

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

var packageToServiceMap = map[string][]string{
	"tc2-agent":            {"tc2-agent"},
	"cacophony-config":     {"cacophony-config-sync"},
	"device-register":      {"device-register"},
	"event-reporter":       {"event-reporter", "version-reporter", "rpi-power-off", "rpi-power-on", "service-watcher"},
	"management-interface": {"managementd"},
	"modemd":               {"modemd"},
	"rpi-net-manager":      {"rpi-net-manager"},
	"salt-updater":         {"salt-updater"},
	"trap-controller":      {"trap-controller"},
	"thermal-uploader":     {"thermal-uploader"},
	"tc2-hat-controller":   {"tc2-hat-comms", "tc2-hat-i2c", "tc2-hat-rtc", "tc2-hat-temp", "tc2-hat-attiny", "rpi-reboot"},
}

var defaultArgs = Args{}

func procArgs(input []string) (Args, error) {
	args := defaultArgs

	parser, err := arg.NewParser(arg.Config{}, &args)
	if err != nil {
		return Args{}, err
	}
	err = parser.Parse(input)
	if errors.Is(err, arg.ErrHelp) {
		parser.WriteHelp(os.Stdout)
		os.Exit(0)
	}
	if errors.Is(err, arg.ErrVersion) {
		fmt.Println(version)
		os.Exit(0)
	}
	return args, err
}

func Run(inputArgs []string, ver string) error {
	version = ver
	args, err := procArgs(inputArgs)
	if err != nil {
		return fmt.Errorf("failed to parse args: %v", err)
	}
	log = logging.NewLogger(args.LogLevel)

	log.Infof("Running version: %s", version)

	serviceToPackageMap := map[string]string{}
	for pkg, services := range packageToServiceMap {
		for _, service := range services {
			serviceToPackageMap[service] = pkg
		}
	}

	/*
		// Test code for checking that all the versions can be found
		for service, pkg := range serviceToPackageMap {
			version, err := getPackageVersion(pkg)
			if err != nil {
				log.Printf("failed to get version for package %s: %v", pkg, err)
			} else {
				log.Printf("Service %s is version %s", service, version)
			}
		}
	*/

	conn, err := systemdbus.NewWithContext(context.Background())
	if err != nil {
		log.Printf("failed to connect to dbus: %v", err)
		return err
	}
	log.Println("Connected to system dbus")

	defer conn.Close()

	if err := conn.Subscribe(); err != nil {
		log.Printf("Failed to subscribe to the dbus: %v", err)
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
			unitName := strings.TrimSuffix(update.UnitName, ".service")
			// Only process states we are interested in
			if !isInterestingState(activeState) {
				break
			}
			if t, ok := lastUnitReportTimes[unitName]; ok && time.Since(t) < minTimeBetweenReports {
				log.Info("Reporting too often for ", unitName)
				break
			}

			rawLogs, failed, err := getLogs(unitName, numLogLines)
			if err != nil {
				return err
			}
			if !failed {
				break // Can just be a service activating
			}

			log.Printf("Service failed. unitName: %s, activeState: %s", unitName, activeState)
			for _, l := range rawLogs {
				log.Debug(l)
			}

			version := "unknown"
			packageName, ok := serviceToPackageMap[unitName]
			if ok {
				version, err = getPackageVersion(packageName)
				if err != nil {
					log.Printf("failed to get version for package %s: %v", packageName, err)
				}
			} else {
				log.Infof("Unknown unitName: %s", unitName)
			}
			log.Debug("Version: ", version)

			// If it is a snapshot then we don't need to be making service errors.
			if strings.Contains(version, "SNAPSHOT") {
				log.Infof("Skipping making service error for SNAPSHOT. Unit '%s', version '%s'", unitName, version)
				break
			}

			event := eventclient.Event{
				Timestamp: ts,
				Type:      "systemError",
				Details: map[string]interface{}{
					"version":               version,
					"unitName":              unitName,
					"logs":                  rawLogs,
					"activeState":           activeState,
					eventclient.SeverityKey: eventclient.SeverityError,
				},
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
			if strings.Contains(rawLog.Message, "Unit entered failed state.") { // Needed for older cameras
				failed = true
			}
			if strings.Contains(rawLog.Message, "Failed with result") { // Needed for TC2 cameras
				failed = true
			}
		}
		logs = append(logs, rawLog.Message)
	}
	return logs, failed, nil
}

func getPackageVersion(packageName string) (string, error) {
	output, err := exec.Command("dpkg-query", "--show", "--showformat=${Version}", packageName).Output()
	if err != nil {
		return "", fmt.Errorf("failed to get version for package %s: %v", packageName, err)
	}

	version := strings.TrimSpace(string(output))
	if version == "" {
		return "", fmt.Errorf("package %s is not installed or version could not be determined", packageName)
	}

	return version, nil
}

func isInterestingState(state string) bool {
	switch state {
	case
		"activating", // If service is set to restart this is called after the service exec exits
		"failed":     // If service is set to restart the service won't 'fail'.
		return true
	}
	return false
}
