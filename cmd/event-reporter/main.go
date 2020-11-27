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
	"log"
	"time"

	"github.com/TheCacophonyProject/event-reporter/eventstore"
	"github.com/TheCacophonyProject/modemd/connrequester"
	arg "github.com/alexflint/go-arg"

	"github.com/TheCacophonyProject/go-api"
)

const (
	connTimeout       = time.Minute * 2
	connRetryInterval = time.Minute * 10
	connMaxRetries    = 3
)

var version = "No version provided"

type argSpec struct {
	DBPath   string        `arg:"-d,--db" help:"path to state database"`
	Interval time.Duration `arg:"--interval" help:"time between event reports"`
}

func (argSpec) Version() string {
	return version
}

func procArgs() argSpec {
	// Set argument default values.
	args := argSpec{
		DBPath:   "/var/lib/event-reporter.db",
		Interval: 30 * time.Minute,
	}
	arg.MustParse(&args)
	return args
}

func main() {
	err := runMain()
	if err != nil {
		log.Fatal(err.Error())
	}
}

func runMain() error {
	args := procArgs()
	log.SetFlags(0) // Removes default timestamp flag
	log.Printf("running version: %s", version)

	store, err := eventstore.Open(args.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	cr := connrequester.NewConnectionRequester()

	uploadEventsChan := make(chan bool)

	err = StartService(store, uploadEventsChan)
	if err != nil {
		return err
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
		}

		select {
		case <-uploadEventsChan:
			log.Println("events upload requested")
		case <-time.After(args.Interval):
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
		log.Printf("API connection failed: %v", err)
		return
	}

	groupedEvents, err := getGroupEvents(store, eventKeys)
	if err != nil {
		log.Printf("error grouping events: %v", err)
	}
	log.Printf("%d event%s to send in %d group%s",
		len(eventKeys), plural(len(eventKeys)),
		len(groupedEvents), plural(len(groupedEvents)))

	var errs []error
	successEvents := 0
	successGroup := 0
	for description, groupedEvent := range groupedEvents {
		if err := apiClient.ReportEvent([]byte(description), groupedEvent.times); err != nil {
			errs = append(errs, err)
		} else {
			if err := store.DeleteKeys(groupedEvent.keys); err != nil {
				log.Printf("failed to delete recordings from store: %v", err)
				return
			}
			successEvents += len(groupedEvent.keys)
			successGroup++
		}
	}

	if len(errs) > 0 {
		log.Printf("%d error%s occurred during reporting. Most recent:", len(errs), plural(len(errs)))
		for _, err := range last5Errs(errs) {
			log.Printf("  %v", err)
		}
	}
	if successEvents > 0 {
		log.Printf("%d event%s sent in %d group%s",
			successEvents, plural(successEvents),
			successGroup, plural(successGroup))
	}
}

type eventGroup struct {
	times []time.Time
	keys  []uint64
}

func getGroupEvents(store *eventstore.EventStore, eventKeys []uint64) (map[string]eventGroup, error) {
	eventGroups := map[string]eventGroup{}
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
	return eventGroups, nil
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
