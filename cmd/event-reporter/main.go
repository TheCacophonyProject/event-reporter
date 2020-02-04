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

	err = StartService(store)
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

		time.Sleep(args.Interval)
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

	var errs []error
	success := 0
	for _, eventKey := range eventKeys {
		if err := sendEvent(store, eventKey, apiClient); err != nil {
			errs = append(errs, err)
		} else {
			store.Delete(eventKey)
			success++
		}
	}
	if len(errs) > 0 {
		log.Printf("%d error%s occurred during reporting. Most recent:", len(errs), plural(len(errs)))
		for _, err := range last5Errs(errs) {
			log.Printf("  %v", err)
		}
	}
	if success > 0 {
		log.Printf("%d event%s sent", success, plural(success))
	}
}

func sendEvent(store *eventstore.EventStore, eventKey uint64, apiClient *api.CacophonyAPI) error {
	eventBytes, err := store.Get(eventKey)
	if err != nil {
		return err
	}
	event := &eventstore.Event{}
	if err := json.Unmarshal(eventBytes, event); err != nil {
		return err
	}
	log.Printf("sending event %v", event)
	return apiClient.ReportEvent(eventBytes, []time.Time{event.Timestamp})
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
