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
	"log"
	"time"

	arg "github.com/alexflint/go-arg"

	"github.com/TheCacophonyProject/event-reporter/eventstore"
	"github.com/TheCacophonyProject/go-api"
)

var version = "No version provided"

type argSpec struct {
	ConfigFile string        `arg:"-c,--config" help:"path to thermal-uploader configuration file"`
	DBPath     string        `arg:"-d,--db" help:"path to state database"`
	Interval   time.Duration `arg:"--interval" help:"time between event reports"`
}

func (argSpec) Version() string {
	return version
}

func procArgs() argSpec {
	// Set argument default values.
	args := argSpec{
		ConfigFile: "/etc/thermal-uploader.yaml",
		DBPath:     "/var/run/event-reporter.db",
		Interval:   30 * time.Minute,
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

	apiClient, err := api.NewAPIFromConfig(args.ConfigFile)
	if err != nil {
		return err
	}

	err = StartService(store)
	if err != nil {
		return err
	}

	for {
		events, err := store.All()
		if err != nil {
			return err
		}
		log.Printf("%d events to send", len(events))
		sendEvents(apiClient, store, events)
		time.Sleep(args.Interval)
	}
}

func sendEvents(
	apiClient *api.CacophonyAPI,
	store *eventstore.EventStore,
	events []eventstore.EventTimes,
) {
	var tempErrs []error
	var permErrs []error
	for _, event := range events {
		err := apiClient.ReportEvent(event.Details, event.Timestamps)
		if err != nil {
			if api.IsPermanentError(err) {
				permErrs = append(permErrs, err)
				store.Discard(event)
			} else {
				tempErrs = append(tempErrs, err)
			}
		} else {
			store.Discard(event)
		}
	}
	logLastErrors("temporary", tempErrs)
	logLastErrors("permanent", permErrs)
}

func logLastErrors(label string, errs []error) {
	if len(errs) < 1 {
		return
	}
	log.Printf("%d %s error%s occurred during reporting. Most recent:", len(errs), label, plural(len(errs)))
	for _, err := range last5Errs(errs) {
		log.Printf("  %v", err)
	}
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
