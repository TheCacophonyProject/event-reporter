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
	"encoding/json"
	"log"
	"os/exec"
	"runtime"
	"strings"
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
	packageJSONData, err := json.Marshal(packageMpedData)
	if err != nil {
		return err
	}
	log.Println(string(packageJSONData))

	return nil
}

// Return info on the packages that are currently installed on the device.
func getInstalledPackages() (map[string]string, error) {

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
	data := map[string]string{}
	rows := strings.Split(packagesData, "\n")
	for _, row := range rows {
		// We only want packages related to cacophony.
		if !strings.Contains(strings.ToUpper(row), "CACOPHONY") {
			continue
		}
		words := strings.Split(strings.TrimSpace(row), "|")
		data[words[0]] = words[1]
	}

	return data, nil
}
