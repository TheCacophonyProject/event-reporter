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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TheCacophonyProject/event-reporter/v3/eventclient"
	"github.com/TheCacophonyProject/event-reporter/v3/eventstore"
	"github.com/TheCacophonyProject/go-utils/logging"
	"github.com/TheCacophonyProject/go-utils/saltutil"
	"github.com/TheCacophonyProject/modemd/connrequester"
	"github.com/TheCacophonyProject/modemd/modemlistener"
	arg "github.com/alexflint/go-arg"

	"github.com/TheCacophonyProject/go-api"
)

const (
	connTimeout           = time.Minute * 2
	connRetryInterval     = time.Minute * 10
	connMaxRetries        = 3
	poweredOffTimeFile    = "/etc/cacophony/powered-off-time"
	severityErrorTimeFile = "/etc/cacophony/severity-error-time"
	uploadBatchSize       = 100
)

var version = "No version provided"
var log *logging.Logger
var mu sync.Mutex
var severityErrorTime = time.Time{}

func getSeverityErrorTime() time.Time {
	mu.Lock()
	defer mu.Unlock()
	return severityErrorTime
}

func setSeverityErrorTime(t time.Time) {
	log.Infof("Setting severity Error Time to %s", t.Format(time.DateTime))
	mu.Lock()
	severityErrorTime = t
	mu.Unlock()
	if err := os.WriteFile(severityErrorTimeFile, []byte(t.Format(time.DateTime)), 0644); err != nil {
		log.Errorf("Error writing severity error time to file: %v", err)
	}
}

func readSeverityErrorTimeFromFile() {
	data, err := os.ReadFile(severityErrorTimeFile)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Errorf("error reading severity error time: %v", err)
	}

	t, err := time.Parse(time.DateTime, string(data))
	if err != nil {
		log.Errorf("error parsing '%s' to DateTime: %v", string(data), err)
	}
	log.Info("Reading last severity Error Time as ", t.Format(time.DateTime))
	mu.Lock()
	severityErrorTime = t
	mu.Unlock()
}

func clearSeverityErrorTime() {
	mu.Lock()
	severityErrorTime = time.Time{}
	mu.Unlock()
	if err := os.Remove(severityErrorTimeFile); err != nil {
		log.Errorf("Error removing severity error time file: %v", err)
	}
}

type Args struct {
	DBPath   string        `arg:"-d,--db" help:"path to state database"`
	Interval time.Duration `arg:"--interval" help:"time between event reports"`
	logging.LogArgs
}

func (Args) Version() string {
	return version
}

var defaultArgs = Args{
	DBPath:   "/var/lib/event-reporter.db",
	Interval: 30 * time.Minute,
}

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

	readSeverityErrorTimeFromFile()

	store, err := eventstore.Open(args.DBPath, args.LogLevel)
	if err != nil {
		return err
	}
	defer store.Close()

	cr := connrequester.NewConnectionRequester()

	uploadEventsChan := make(chan bool, 2)

	err = StartService(store, uploadEventsChan)
	if err != nil {
		return err
	}

	//If powered off time was saved make a powered off event
	makePowerOffEvent()
	modemConnectSignal, err := modemlistener.GetModemConnectedSignalListener()
	if err != nil {
		log.Println("Failed to get modem connected signal listener")
	}

	for {
		eventKeys, err := store.GetKeys()
		if err != nil {
			return err
		}

		sendCount := len(eventKeys)
		if sendCount > 0 {
			log.Printf("%d event%s to send", sendCount, plural(sendCount))
			sendEvents(store, eventKeys, cr)

			// Check if the devices logs should be uploaded also through salt.
			uploadDevicesLogs()
		}

		// Empty modemConnectSignal channel so as to not trigger from old signals
		emptyChannel(modemConnectSignal)
		select {
		case <-uploadEventsChan:
			log.Println("events upload requested")
		case <-modemConnectSignal:
			log.Println("Modem connected.")
		case <-time.After(args.Interval):
		}
	}
}

func emptyChannel(ch chan time.Time) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func sendEvents(
	store *eventstore.EventStore,
	eventKeys []uint64,
	cr *connrequester.ConnectionRequester,
) {
	cr.Start()
	defer cr.Stop()
	if err := cr.WaitUntilUpLoop(connTimeout, connRetryInterval, connMaxRetries); err != nil {
		log.Println("unable to get an internet connection. Not reporting events")
		return
	}

	apiClient, err := api.New()
	if err != nil {
		log.Warnf("API connection failed: %v", err)
		return
	}

	groupedEvents, err := getGroupEvents(store, eventKeys)
	if err != nil {
		log.Errorf("error grouping events: %v", err)
	}
	log.Printf("%d event%s to send in %d group%s",
		len(eventKeys), plural(len(eventKeys)),
		len(groupedEvents), plural(len(groupedEvents)))

	var errs []error
	successEvents := 0
	successGroup := 0
	for _, groupedEvent := range groupedEvents {
		if err := apiClient.ReportEvent([]byte(groupedEvent.description), groupedEvent.times); err != nil {
			errs = append(errs, err)
		} else {
			if err := store.DeleteKeys(groupedEvent.keys); err != nil {
				log.Errorf("failed to delete recordings from store: %v", err)
				return
			}
			successEvents += len(groupedEvent.keys)
			successGroup++
		}
	}

	if len(errs) > 0 {
		log.Printf("%d error%s occurred during reporting. Most recent:", len(errs), plural(len(errs)))
		for _, err := range last5Errs(errs) {
			log.Errorf("%v", err)
		}
	}
	if successEvents > 0 {
		log.Printf("%d event%s sent in %d group%s",
			successEvents, plural(successEvents),
			successGroup, plural(successGroup))
	}
}

type eventGroup struct {
	times       []time.Time
	keys        []uint64
	description string
}

func getGroupEvents(store *eventstore.EventStore, eventKeys []uint64) ([]eventGroup, error) {
	eventGroups := map[string]eventGroup{}

	// First batch all events by description
	for _, eventKey := range eventKeys {
		eventBytes, err := store.Get(eventKey)
		if err != nil {
			return nil, err
		}
		event := &eventstore.Event{}
		if err := json.Unmarshal(eventBytes, event); err != nil {
			return nil, err
		}

		description, err := json.Marshal(&eventstore.Event{
			Description: event.Description,
		})
		if err != nil {
			return nil, err
		}

		eventGroup := eventGroups[string(description)]
		eventGroup.times = append(eventGroup.times, event.Timestamp)
		eventGroup.keys = append(eventGroup.keys, eventKey)
		eventGroups[string(description)] = eventGroup
	}

	// Then make a list of grouped events. Making sure that each group has at most uploadBatchSize events
	eventGroupsList := []eventGroup{}
	for description, groups := range eventGroups {
		times := groups.times
		for i := 0; i < len(times); i += uploadBatchSize {
			end := i + uploadBatchSize
			if end > len(times) {
				end = len(times)
			}
			eventGroupsList = append(eventGroupsList, eventGroup{
				times:       times[i:end],
				keys:        groups.keys[i:end],
				description: description,
			})
		}
	}

	return eventGroupsList, nil
}

func last5Errs(errs []error) []error {
	i := len(errs) - 5
	if i < 0 {
		i = 0
	}
	return errs[i:]
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func makePowerOffEvent() {
	outBytes, err := os.ReadFile(poweredOffTimeFile)
	if err != nil {
		return
	}
	nanoTime, err := strconv.ParseInt(strings.TrimSpace(string(outBytes)), 10, 64)
	if err != nil {
		log.Printf("failed to read power off time: %v", err)
		return
	}
	eventclient.AddEvent(eventclient.Event{
		Timestamp: time.Unix(0, nanoTime),
		Type:      "rpiPoweredOff",
	})
	os.Remove(poweredOffTimeFile)
}

func uploadDevicesLogs() {
	errTime := getSeverityErrorTime()
	if !errTime.IsZero() {
		log.Info("Uploading device logs")

		logSince := errTime.Add(-time.Hour * 12) // Get logs from 12 hours before the first error
		oneMonth := 31 * 24 * time.Hour
		if time.Since(logSince) > oneMonth {
			log.Info("Limiting logs to one month.")
			logSince = time.Now().Add(-oneMonth)
		}

		log.Infof("Uploading device logs since %s", logSince.Format(time.DateTime))

		journalctlCmd := exec.Command("journalctl", "--since", logSince.Format(time.DateTime))

		logFileName := "/tmp/journalctl-logs-severity-error.log"
		logFile, err := os.Create(logFileName)
		if err != nil {
			log.Printf("Failed to create log file: %v", err)

			return
		}
		defer logFile.Close()

		journalctlCmd.Stdout = logFile

		if err := journalctlCmd.Run(); err != nil {
			log.Printf("Failed to run journalctl command: %v", err)
			return
		}

		if err := exec.Command("gzip", "-f", logFileName).Run(); err != nil {
			log.Printf("Failed to compress log file: %v", err)
			return
		}

		if !saltutil.IsSaltIdSet() {
			log.Error("Salt is not yet ready to upload logs")
			return
		}
		if err := exec.Command("salt-call", "cp.push", logFileName+".gz").Run(); err != nil {
			log.Printf("Error pushing log file with salt: %v", err)
			return
		}
		log.Info("Device logs uploaded")
		clearSeverityErrorTime()
	}
}
