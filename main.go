// Copyright 2017 HootSuite Media Inc.
//
// Licensed under the Apache License, Version 2.0 (the License);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an AS IS BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// Modified hereafter by contributors to runatlantis/atlantis.
//
// Package main is the entrypoint for the CLI.
package main

import (
	"fmt"

	"github.com/runatlantis/atlantis/cmd"
	"github.com/runatlantis/atlantis/server/logging"
	"github.com/spf13/viper"
)

const atlantisVersion = "0.17.0"

func main() {

	v := viper.New()

	logger, err := logging.NewStructuredLogger()

	if err != nil {
		panic(fmt.Sprintf("unable to initialize logger. %s", err.Error()))
	}

	// We're creating commands manually here rather than using init() functions
	// (as recommended by cobra) because it makes testing easier.
	server := &cmd.ServerCmd{
		ServerCreator:   &cmd.DefaultServerCreator{},
		Viper:           v,
		AtlantisVersion: atlantisVersion,
		Logger:          logger,
	}
	version := &cmd.VersionCmd{AtlantisVersion: atlantisVersion}
	testdrive := &cmd.TestdriveCmd{}
	cmd.RootCmd.AddCommand(server.Init())
	cmd.RootCmd.AddCommand(version.Init())
	cmd.RootCmd.AddCommand(testdrive.Init())
	cmd.Execute()
}
