/*
 * Minio Client (C) 2014, 2015 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"encoding/json"

	"github.com/minio/cli"
	"github.com/minio/mc/pkg/console"
	"github.com/minio/minio-xl/pkg/probe"
)

var (
	configVersionFlags = []cli.Flag{
		cli.BoolFlag{
			Name:  "help, h",
			Usage: "Help of config version",
		},
	}
)

// Print config version.
var configVersionCmd = cli.Command{
	Name:   "version",
	Usage:  "Print config version.",
	Action: mainConfigVersion,
	Flags:  append(configVersionFlags, globalFlags...),
	CustomHelpTemplate: `NAME:
   mc config {{.Name}} - {{.Usage}}

USAGE:
   mc config {{.Name}}
`,
}

func mainConfigVersion(ctx *cli.Context) {
	// Set global flags from context.
	setGlobalsFromContext(ctx)

	config, err := loadMcConfig()
	fatalIf(err.Trace(), "Unable to load config version ‘"+globalMCConfigVersion+"’.")

	type Version string
	if globalJSON {
		tB, e := json.Marshal(
			struct {
				Version Version `json:"version"`
			}{Version: Version(config.Version)},
		)
		fatalIf(probe.NewError(e), "Unable to construct version string.")
		console.Println(string(tB))
		return
	}
	console.Println(config.Version)
}
