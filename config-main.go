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
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/minio/cli"
	"github.com/minio/mc/pkg/console"
	"github.com/minio/minio/pkg/probe"
	"github.com/minio/minio/pkg/quick"
)

//   Configure minio client
//
//   ----
//   NOTE: that the configure command only writes values to the config file.
//   It does not use any configuration values from the environment variables.
//
//   One needs to edit configuration file manually, this is purposefully done
//   so to avoid taking credentials over cli arguments. It is a security precaution
//   ----
//
var configCmd = cli.Command{
	Name:   "config",
	Usage:  "Modify, add alias, oauth into default configuration file [~/.mc/config.json].",
	Action: mainConfig,
	CustomHelpTemplate: `NAME:
   mc {{.Name}} - {{.Usage}}

USAGE:
   mc {{.Name}} add alias ALIASNAME URL
   mc {{.Name}} list alias

EXAMPLES:
   1. Add aliases for a URL
      $ mc {{.Name}} add alias zek https://s3.amazonaws.com/

   2. List all aliased URLs.
      $ mc {{.Name}} list alias

`,
}

// AliasMessage container for content message structure
type AliasMessage struct {
	Alias string `json:"alias"`
	URL   string `json:"url"`
}

// String string printer for Content metadata
func (a AliasMessage) String() string {
	if !globalJSONFlag {
		message := console.Colorize("Alias", fmt.Sprintf("[%s] <- ", a.Alias))
		message += console.Colorize("URL", fmt.Sprintf("%s", a.URL))
		return message
	}
	jsonMessageBytes, e := json.Marshal(a)
	fatalIf(probe.NewError(e), "Unable to marshal into JSON.")

	return string(jsonMessageBytes)
}

func checkConfigSyntax(ctx *cli.Context) {
	// show help if nothing is set
	if !ctx.Args().Present() || ctx.Args().First() == "help" {
		cli.ShowCommandHelpAndExit(ctx, "config", 1) // last argument is exit code
	}
	if strings.TrimSpace(ctx.Args().First()) == "" {
		cli.ShowCommandHelpAndExit(ctx, "config", 1) // last argument is exit code
	}
	if len(ctx.Args().Tail()) > 3 {
		fatalIf(errDummy().Trace(), "Incorrect number of arguments to config command")
	}
	switch strings.TrimSpace(ctx.Args().First()) {
	case "add":
		if strings.TrimSpace(ctx.Args().Tail().First()) != "alias" {
			cli.ShowCommandHelpAndExit(ctx, "config", 1) // last argument is exit code
		}
		if strings.TrimSpace(ctx.Args().Tail().First()) == "alias" {
			if len(ctx.Args().Tail().Tail()) != 2 {
				fatalIf(errInvalidArgument().Trace(), "Incorrect number of arguments for add alias command.")
			}
		}
	case "list":
		if strings.TrimSpace(ctx.Args().Tail().First()) != "alias" {
			cli.ShowCommandHelpAndExit(ctx, "config", 1) // last argument is exit code
		}
	default:
		cli.ShowCommandHelpAndExit(ctx, "config", 1) // last argument is exit code
	}
}

// mainConfig is the handle for "mc config" sub-command. writes configuration data in json format to config file.
func mainConfig(ctx *cli.Context) {
	checkConfigSyntax(ctx)

	// set new custom coloring
	console.SetCustomTheme(map[string]*color.Color{
		"Alias": color.New(color.FgCyan, color.Bold),
		"URL":   color.New(color.FgWhite),
	})

	arg := ctx.Args().First()
	tailArgs := ctx.Args().Tail()

	switch strings.TrimSpace(arg) {
	case "add":
		if strings.TrimSpace(tailArgs.First()) == "alias" {
			addAlias(tailArgs.Get(1), tailArgs.Get(2))
		}
	case "list":
		if strings.TrimSpace(tailArgs.First()) == "alias" {
			conf := newConfigV2()
			config, err := quick.New(conf)
			fatalIf(err.Trace(conf.Version), "Failed to initialize ‘quick’ configuration data structure.")

			configPath := mustGetMcConfigPath()
			err = config.Load(configPath)
			fatalIf(err.Trace(configPath), "Unable to load config path")

			// convert interface{} back to its original struct
			newConf := config.Data().(*configV2)
			for k, v := range newConf.Aliases {
				console.Println(AliasMessage{k, v})
			}
		}
	}
}

// addAlias - add new aliases
func addAlias(alias, url string) {
	if alias == "" || url == "" {
		fatalIf(errDummy().Trace(), "Alias or URL cannot be empty.")
	}
	conf := newConfigV2()
	config, err := quick.New(conf)
	fatalIf(err.Trace(conf.Version), "Failed to initialize ‘quick’ configuration data structure.")

	err = config.Load(mustGetMcConfigPath())
	fatalIf(err.Trace(), "Unable to load config path")

	url = strings.TrimSuffix(url, "/")
	if !strings.HasPrefix(url, "http") {
		fatalIf(errDummy().Trace(), fmt.Sprintf("Invalid alias URL ‘%s’. Valid examples are: http://s3.amazonaws.com, https://yourbucket.example.com.", url))
	}
	if isAliasReserved(alias) {
		fatalIf(errDummy().Trace(), fmt.Sprintf("Cannot use a reserved name ‘%s’ as an alias. Following are reserved names: [help, private, readonly, public, authenticated].", alias))
	}
	if !isValidAliasName(alias) {
		fatalIf(errDummy().Trace(), fmt.Sprintf("Alias name ‘%s’ is invalid, valid examples are: mybucket, Area51, Grand-Nagus", alias))
	}
	// convert interface{} back to its original struct
	newConf := config.Data().(*configV2)
	if oldURL, ok := newConf.Aliases[alias]; ok {
		fatalIf(errDummy().Trace(), fmt.Sprintf("Alias ‘%s’ already exists for ‘%s’.", alias, oldURL))
	}
	newConf.Aliases[alias] = url
	newConfig, err := quick.New(newConf)
	fatalIf(err.Trace(conf.Version), "Failed to initialize ‘quick’ configuration data structure.")

	err = writeConfig(newConfig)
	fatalIf(err.Trace(alias, url), "Unable to save alias ‘"+alias+"’.")
}
