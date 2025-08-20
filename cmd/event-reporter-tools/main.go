package main

import (
	"fmt"
	"os"

	eventreporter "github.com/TheCacophonyProject/event-reporter/v3/internal/event-reporter"
	servicewatcher "github.com/TheCacophonyProject/event-reporter/v3/internal/service-watcher"
	versionreporter "github.com/TheCacophonyProject/event-reporter/v3/internal/version-reporter"
	"github.com/TheCacophonyProject/go-utils/logging"
)

var log *logging.Logger

func main() {
	err := runMain()
	if err != nil {
		log.Fatal(err)
	}
}

var version = "<not set>"

func runMain() error {
	log = logging.NewLogger("info")
	if len(os.Args) < 2 {
		log.Info("Usage: tool <subcommand> [args]")
		return fmt.Errorf("no subcommand given")
	}

	subcommand := os.Args[1]
	args := os.Args[2:]

	var err error
	switch subcommand {
	case "event-reporter":
		err = eventreporter.Run(args, version)
	case "service-watcher":
		err = servicewatcher.Run(args, version)
	case "version-reporter":
		err = versionreporter.Run(args, version)
	default:
		err = fmt.Errorf("unknown subcommand: %s", subcommand)
	}

	return err
}
