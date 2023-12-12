/*
version-reporter - report event containing the version of all cacophony software at boot.
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
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/TheCacophonyProject/event-reporter/v3/eventclient"
)

func main() {
	err := runMain()
	if err != nil {
		log.Fatal(err.Error())
	}
}

func runMain() error {
	log.SetFlags(0) // Removes default timestamp flag

	packageMpedData, err := getInstalledPackages()
	if err != nil {
		return err
	}

	event := eventclient.Event{
		Timestamp: time.Now(),
		Type:      "versionData",
		Details:   packageMpedData,
	}

	for i := 3; i > 0; i-- {
		err := eventclient.AddEvent(event)
		if err == nil {
			log.Println("added verionData event")
			if err := eventclient.UploadEvents(); err != nil {
				return err
			}
			break
		}
		if i == 1 {
			log.Println(err)
			break
		}
		log.Println("failed to log event. Will retry in 5 seconds")
		time.Sleep(5 * time.Second)
	}

	return nil
}

// Return info on the packages that are currently installed on the device.
func getInstalledPackages() (map[string]interface{}, error) {

	if runtime.GOOS == "windows" {
		return nil, nil
	}

	out, err := exec.Command(
		"/usr/bin/dpkg-query",
		"--show",
		"--showformat",
		"${Package}|${Version}|${Maintainer}\n").Output()
	if err != nil {
		return nil, err
	}
	packagesData := string(out)
	// Want to separate this into separate fields so that can display in a table in HTML
	data := map[string]interface{}{}
	rows := strings.Split(packagesData, "\n")
	for _, row := range rows {
		// We only want packages related to cacophony.
		if !strings.Contains(strings.ToUpper(row), "CACOPHONY") {
			continue
		}
		words := strings.Split(strings.TrimSpace(row), "|")
		data[words[0]] = words[1]
	}

	classifier_path := "/home/pi/.venv/classifier/bin/python"
	if _, err := os.Stat(classifier_path); err == nil {
		out, err := exec.Command(classifier_path, "-m", "pip", "show", "classifier-pipeline").Output()
		if err != nil {
			return data, nil
		}

		pipInfo := string(out)
		version_index := strings.Index(pipInfo, "Version:")
		if version_index != -1 {
			pipInfo = pipInfo[version_index+9:]
			end_line := strings.Index(pipInfo, "\n")
			version_info := strings.Trim(pipInfo[:end_line], " ")
			data["classifier-pipeline"] = version_info
		}

	}

	return data, nil
}
