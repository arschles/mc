/*
 * Minio Client (C) 2015 Minio, Inc.
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
	"strings"

	"github.com/minio/mc/pkg/client"
	"github.com/minio/mc/pkg/console"
	"github.com/minio/minio-xl/pkg/quick"
)

func migrateConfig() {
	// Migrate config V1 to V101
	migrateConfigV1ToV101()
	// Migrate config V101 to V2
	migrateConfigV101ToV2()
	// Migrate config V2 to V3
	migrateConfigV2ToV3()
	// Migrate config V3 to V4
	migrateConfigV3ToV4()
	// Migrate config V4 to V5
	migrateConfigV4ToV5()
	// Migrate config V5 to V6
	migrateConfigV5ToV6()
}

func fixConfig() {
	// Fix config V3
	fixConfigV3()
	// Fix config V6
	fixConfigV6()
	// Fix config V6 for hosts
	fixConfigV6ForHosts()
}

func fixConfigV6ForHosts() {
	if !isMcConfigExists() {
		return
	}
	config, err := quick.New(newConfigV6())
	fatalIf(err.Trace(), "Unable to initialize config.")

	err = config.Load(mustGetMcConfigPath())
	fatalIf(err.Trace(), "Unable to load config.")

	if config.Data().(*configV6).Version == "6" {
		newConfig := new(configV6)
		newConfig.Aliases = make(map[string]string)
		newConfig.Hosts = make(map[string]hostConfig)
		newConfig.Version = "6"
		newConfig.Aliases = config.Data().(*configV6).Aliases

		url := new(client.URL)
		for host, hostCfg := range config.Data().(*configV6).Hosts {
			// Already fixed - move on.
			if strings.HasPrefix(host, "https") || strings.HasPrefix(host, "http") {
				newConfig.Hosts[host] = hostCfg
				continue
			}
			if host == "s3.amazonaws.com" || host == "storage.googleapis.com" {
				console.Infoln("Found hosts, replacing " + host + " with https://" + host)
				url.Host = host
				url.Scheme = "https"
				url.SchemeSeparator = "://"
				newConfig.Hosts[url.String()] = hostCfg
				delete(newConfig.Hosts, host)
			}
			if host == "localhost:9000" || host == "127.0.0.1:9000" {
				console.Infoln("Found hosts, replacing " + host + " with http://" + host)
				url.Host = host
				url.Scheme = "http"
				url.SchemeSeparator = "://"
				newConfig.Hosts[url.String()] = hostCfg
				delete(newConfig.Hosts, host)
			}
			if host == "play.minio.io:9000" || host == "dl.minio.io:9000" {
				console.Infoln("Found hosts, replacing " + host + " with https://" + host)
				url.Host = host
				url.Scheme = "https"
				url.SchemeSeparator = "://"
				newConfig.Hosts[url.String()] = hostCfg
				delete(newConfig.Hosts, host)
			}
		}
		newConf, err := quick.New(newConfig)
		fatalIf(err.Trace(), "Unable to initialize newly fixed config.")

		err = newConf.Save(mustGetMcConfigPath())
		fatalIf(err.Trace(mustGetMcConfigPath()), "Unable to save newly fixed config path.")
	}
}

// fixConfigV6 - fix all the unnecessary glob URLs present in existing config version 6.
func fixConfigV6() {
	if !isMcConfigExists() {
		return
	}
	config, err := quick.New(newConfigV6())
	fatalIf(err.Trace(), "Unable to initialize config.")

	err = config.Load(mustGetMcConfigPath())
	fatalIf(err.Trace(mustGetMcConfigPath()), "Unable to load config.")

	if config.Data().(*configV6).Version == "6" {
		newConfig := new(configV6)
		newConfig.Aliases = make(map[string]string)
		newConfig.Hosts = make(map[string]hostConfig)
		newConfig.Version = "6"
		newConfig.Aliases = config.Data().(*configV6).Aliases
		for host, hostCfg := range config.Data().(*configV6).Hosts {
			if strings.HasPrefix(host, "https") || strings.HasPrefix(host, "http") {
				newConfig.Hosts[host] = hostCfg
				continue
			}
			if strings.Contains(host, "*s3*") || strings.Contains(host, "*.s3*") {
				console.Infoln("Found glob url, replacing " + host + " with s3.amazonaws.com")
				newConfig.Hosts["s3.amazonaws.com"] = hostCfg
				continue
			}
			if strings.Contains(host, "s3*") {
				console.Infoln("Found glob url, replacing " + host + " with s3.amazonaws.com")
				newConfig.Hosts["s3.amazonaws.com"] = hostCfg
				continue
			}
			if strings.Contains(host, "*amazonaws.com") || strings.Contains(host, "*.amazonaws.com") {
				console.Infoln("Found glob url, replacing " + host + " with s3.amazonaws.com")
				newConfig.Hosts["s3.amazonaws.com"] = hostCfg
				continue
			}
			if strings.Contains(host, "*storage.googleapis.com") {
				console.Infoln("Found glob url, replacing " + host + " with storage.googleapis.com")
				newConfig.Hosts["storage.googleapis.com"] = hostCfg
				continue
			}
			if strings.Contains(host, "localhost:*") {
				console.Infoln("Found glob url, replacing " + host + " with localhost:9000")
				newConfig.Hosts["localhost:9000"] = hostCfg
				continue
			}
			if strings.Contains(host, "127.0.0.1:*") {
				console.Infoln("Found glob url, replacing " + host + " with 127.0.0.1:9000")
				newConfig.Hosts["127.0.0.1:9000"] = hostCfg
				continue
			}
			newConfig.Hosts[host] = hostCfg
		}
		newConf, err := quick.New(newConfig)
		fatalIf(err.Trace(), "Unable to initialize newly fixed config.")

		err = newConf.Save(mustGetMcConfigPath())
		fatalIf(err.Trace(mustGetMcConfigPath()), "Unable to save newly fixed config path.")
	}
}

type configV5 struct {
	Version string                `json:"version"`
	Aliases map[string]string     `json:"alias"`
	Hosts   map[string]hostConfig `json:"hosts"`
}

type configV4 struct {
	Version string            `json:"version"`
	Aliases map[string]string `json:"alias"`
	Hosts   map[string]struct {
		AccessKeyID     string `json:"accessKeyId"`
		SecretAccessKey string `json:"secretAccessKey"`
		Signature       string `json:"signature"`
	} `json:"hosts"`
}

type configV3 struct {
	Version string            `json:"version"`
	Aliases map[string]string `json:"alias"`
	// custom anonymous struct is necessary from version to 3 to version 4
	// since hostConfig{} has changed to camelcase fields for unmarshal
	Hosts map[string]struct {
		AccessKeyID     string `json:"access-key-id"`
		SecretAccessKey string `json:"secret-access-key"`
	} `json:"hosts"`
}

type configV2 struct {
	Version string
	Aliases map[string]string
	// custom anonymous struct is necessary from version to 2 to version 3
	// since hostConfig{} has changed to lower case fields for unmarshal
	Hosts map[string]struct {
		AccessKeyID     string
		SecretAccessKey string
	}
}

// for backward compatibility
type configV101 configV2
type configV1 configV2

// Migrate from config version ‘1.0’ to ‘1.0.1’
func migrateConfigV1ToV101() {
	if !isMcConfigExists() {
		return
	}
	mcConfigV1, err := quick.Load(mustGetMcConfigPath(), newConfigV1())
	fatalIf(err.Trace(), "Unable to load config version ‘1’.")

	// update to newer version
	if mcConfigV1.Version() == "1.0.0" {
		confV101 := mcConfigV1.Data().(*configV1)
		confV101.Version = "1.0.1"

		localHostConfig := struct {
			AccessKeyID     string
			SecretAccessKey string
		}{}
		localHostConfig.AccessKeyID = ""
		localHostConfig.SecretAccessKey = ""

		s3HostConf := struct {
			AccessKeyID     string
			SecretAccessKey string
		}{}
		s3HostConf.AccessKeyID = globalAccessKeyID
		s3HostConf.SecretAccessKey = globalSecretAccessKey

		if _, ok := confV101.Hosts["localhost:*"]; !ok {
			confV101.Hosts["localhost:*"] = localHostConfig
		}
		if _, ok := confV101.Hosts["127.0.0.1:*"]; !ok {
			confV101.Hosts["127.0.0.1:*"] = localHostConfig
		}
		if _, ok := confV101.Hosts["*.s3*.amazonaws.com"]; !ok {
			confV101.Hosts["*.s3*.amazonaws.com"] = s3HostConf
		}

		mcNewConfigV101, err := quick.New(confV101)
		fatalIf(err.Trace(), "Unable to initialize quick config for config version ‘1.0.1’.")

		err = mcNewConfigV101.Save(mustGetMcConfigPath())
		fatalIf(err.Trace(), "Unable to save config version ‘1.0.1’.")

		console.Infof("Successfully migrated %s from version ‘1.0.0’ to version ‘1.0.1’.\n", mustGetMcConfigPath())
	}
}

// Migrate from config ‘1.0.1’ to ‘2’
func migrateConfigV101ToV2() {
	if !isMcConfigExists() {
		return
	}
	mcConfigV101, err := quick.Load(mustGetMcConfigPath(), newConfigV101())
	fatalIf(err.Trace(), "Unable to load config version ‘1.0.1’.")

	// update to newer version
	if mcConfigV101.Version() == "1.0.1" {
		confV2 := mcConfigV101.Data().(*configV101)
		confV2.Version = "2"

		mcNewConfigV2, err := quick.New(confV2)
		fatalIf(err.Trace(), "Unable to initialize quick config for config version ‘2’.")

		err = mcNewConfigV2.Save(mustGetMcConfigPath())
		fatalIf(err.Trace(), "Unable to save config version ‘2’.")

		console.Infof("Successfully migrated %s from version ‘1.0.1’ to version ‘2’.\n", mustGetMcConfigPath())
	}
}

// Migrate from config ‘2’ to ‘3’
func migrateConfigV2ToV3() {
	if !isMcConfigExists() {
		return
	}
	mcConfigV2, err := quick.Load(mustGetMcConfigPath(), newConfigV2())
	fatalIf(err.Trace(), "Unable to load mc config V2.")

	// update to newer version
	if mcConfigV2.Version() == "2" {
		confV3 := new(configV3)
		confV3.Aliases = mcConfigV2.Data().(*configV2).Aliases
		confV3.Hosts = make(map[string]struct {
			AccessKeyID     string `json:"access-key-id"`
			SecretAccessKey string `json:"secret-access-key"`
		})
		for host, hostConf := range mcConfigV2.Data().(*configV2).Hosts {
			newHostConf := struct {
				AccessKeyID     string `json:"access-key-id"`
				SecretAccessKey string `json:"secret-access-key"`
			}{}
			newHostConf.AccessKeyID = hostConf.AccessKeyID
			newHostConf.SecretAccessKey = hostConf.SecretAccessKey
			confV3.Hosts[host] = newHostConf
		}
		confV3.Version = "3"

		mcNewConfigV3, err := quick.New(confV3)
		fatalIf(err.Trace(), "Unable to initialize quick config for config version ‘3’.")

		err = mcNewConfigV3.Save(mustGetMcConfigPath())
		fatalIf(err.Trace(), "Unable to save config version ‘3’.")

		console.Infof("Successfully migrated %s from version ‘2’ to version ‘3’.\n", mustGetMcConfigPath())
	}
}

// Migrate from config version ‘3’ to ‘4’
func migrateConfigV3ToV4() {
	if !isMcConfigExists() {
		return
	}
	mcConfigV3, err := quick.Load(mustGetMcConfigPath(), newConfigV3())
	fatalIf(err.Trace(), "Unable to load mc config V2.")

	// update to newer version
	if mcConfigV3.Version() == "3" {
		confV4 := new(configV4)
		confV4.Aliases = mcConfigV3.Data().(*configV3).Aliases
		confV4.Hosts = make(map[string]struct {
			AccessKeyID     string `json:"accessKeyId"`
			SecretAccessKey string `json:"secretAccessKey"`
			Signature       string `json:"signature"`
		})
		for host, hostConf := range mcConfigV3.Data().(*configV3).Hosts {
			confV4.Hosts[host] = struct {
				AccessKeyID     string `json:"accessKeyId"`
				SecretAccessKey string `json:"secretAccessKey"`
				Signature       string `json:"signature"`
			}{
				AccessKeyID:     hostConf.AccessKeyID,
				SecretAccessKey: hostConf.SecretAccessKey,
				Signature:       "v4",
			}
		}
		confV4.Version = "4"

		mcNewConfigV4, err := quick.New(confV4)
		fatalIf(err.Trace(), "Unable to initialize quick config for config version ‘4’.")

		err = mcNewConfigV4.Save(mustGetMcConfigPath())
		fatalIf(err.Trace(), "Unable to save config version ‘4’.")

		console.Infof("Successfully migrated %s from version ‘3’ to version ‘4’.\n", mustGetMcConfigPath())
	}
}

// Migrate config version ‘4’ to ‘5’
func migrateConfigV4ToV5() {
	if !isMcConfigExists() {
		return
	}
	mcConfigV4, err := quick.Load(mustGetMcConfigPath(), newConfigV4())
	fatalIf(err.Trace(), "Unable to load mc config V4.")

	// update to newer version
	if mcConfigV4.Version() == "4" {
		confV5 := new(configV5)
		confV5.Aliases = mcConfigV4.Data().(*configV4).Aliases
		confV5.Hosts = make(map[string]hostConfig)
		for host, hostConf := range mcConfigV4.Data().(*configV4).Hosts {
			confV5.Hosts[host] = hostConfig{
				AccessKeyID:     hostConf.AccessKeyID,
				SecretAccessKey: hostConf.SecretAccessKey,
				API:             "S3v4",
			}
		}
		confV5.Version = "5"

		mcNewConfigV5, err := quick.New(confV5)
		fatalIf(err.Trace(), "Unable to initialize quick config for config version ‘5’.")

		err = mcNewConfigV5.Save(mustGetMcConfigPath())
		fatalIf(err.Trace(), "Unable to save config version ‘5’.")

		console.Infof("Successfully migrated %s from version ‘4’ to version ‘5’.\n", mustGetMcConfigPath())
	}
}

// Migrate config version ‘4’ to ‘5’
func migrateConfigV5ToV6() {
	if !isMcConfigExists() {
		return
	}
	mcConfigV5, err := quick.Load(mustGetMcConfigPath(), newConfigV5())
	fatalIf(err.Trace(), "Unable to load mc config V5.")

	// update to newer version
	if mcConfigV5.Version() == "5" {
		confV6 := new(configV6)
		confV6.Aliases = mcConfigV5.Data().(*configV5).Aliases
		confV6.Aliases["gcs"] = "https://storage.googleapis.com"
		confV6.Hosts = make(map[string]hostConfig)
		for host, hostConf := range mcConfigV5.Data().(*configV5).Hosts {
			confV6.Hosts[host] = hostConfig{
				AccessKeyID:     hostConf.AccessKeyID,
				SecretAccessKey: hostConf.SecretAccessKey,
				API:             hostConf.API,
			}
		}
		var s3Conf hostConfig
		for host, hostConf := range confV6.Hosts {
			if strings.Contains(host, "s3") {
				if (hostConf.AccessKeyID == globalAccessKeyID) ||
					(hostConf.SecretAccessKey == globalSecretAccessKey) {
					delete(confV6.Hosts, host)
				}
				if hostConf.AccessKeyID == "" || hostConf.SecretAccessKey == "" {
					delete(confV6.Hosts, host)
				}
				s3Conf = hostConfig{
					AccessKeyID:     hostConf.AccessKeyID,
					SecretAccessKey: hostConf.SecretAccessKey,
					API:             hostConf.API,
				}
				break
			}
		}
		confV6.Hosts["*s3*amazonaws.com"] = s3Conf
		confV6.Hosts["*storage.googleapis.com"] = hostConfig{
			AccessKeyID:     globalAccessKeyID,
			SecretAccessKey: globalSecretAccessKey,
			API:             "S3v2",
		}
		confV6.Version = globalMCConfigVersion
		mcNewConfigV6, err := quick.New(confV6)
		fatalIf(err.Trace(), "Unable to initialize quick config for config version ‘6’.")

		err = mcNewConfigV6.Save(mustGetMcConfigPath())
		fatalIf(err.Trace(), "Unable to save config version ‘6’.")

		console.Infof("Successfully migrated %s from version ‘5’ to version ‘6’.\n", mustGetMcConfigPath())
	}
}

// Fix config version ‘3’, by removing broken struct tags.
func fixConfigV3() {
	if !isMcConfigExists() {
		return
	}
	// brokenConfigV3 broken config between version 3.
	type brokenConfigV3 struct {
		Version string
		ACL     string
		Access  string
		Aliases map[string]string
		Hosts   map[string]struct {
			AccessKeyID     string
			SecretAccessKey string
		}
	}
	conf := new(brokenConfigV3)
	conf.Aliases = make(map[string]string)
	conf.Hosts = make(map[string]struct {
		AccessKeyID     string
		SecretAccessKey string
	})

	mcConfigV3, err := quick.Load(mustGetMcConfigPath(), conf)
	fatalIf(err.Trace(), "Unable to load config.")

	// Update to newer version.
	if len(mcConfigV3.Data().(*brokenConfigV3).Aliases) != 0 || mcConfigV3.Data().(*brokenConfigV3).ACL != "" || mcConfigV3.Data().(*brokenConfigV3).Access != "" && mcConfigV3.Version() == "3" {
		confV3 := new(configV3)
		confV3.Aliases = mcConfigV3.Data().(*brokenConfigV3).Aliases
		confV3.Hosts = make(map[string]struct {
			AccessKeyID     string `json:"access-key-id"`
			SecretAccessKey string `json:"secret-access-key"`
		})
		for host, hostConf := range mcConfigV3.Data().(*brokenConfigV3).Hosts {
			newHostConf := struct {
				AccessKeyID     string `json:"access-key-id"`
				SecretAccessKey string `json:"secret-access-key"`
			}{}
			newHostConf.AccessKeyID = hostConf.AccessKeyID
			newHostConf.SecretAccessKey = hostConf.SecretAccessKey
			confV3.Hosts[host] = newHostConf
		}
		confV3.Version = "3"

		mcNewConfigV3, err := quick.New(confV3)
		fatalIf(err.Trace(), "Unable to initialize quick config for config version ‘3’.")

		err = mcNewConfigV3.Save(mustGetMcConfigPath())
		fatalIf(err.Trace(), "Unable to save config version ‘3’.")

		console.Infof("Successfully fixed %s broken config for version ‘3’.\n", mustGetMcConfigPath())
	}
}

// newConfigV1() - get new config version 1.0.0
func newConfigV1() *configV1 {
	conf := new(configV1)
	conf.Version = "1.0.0"
	// make sure to allocate map's otherwise Golang
	// exits silently without providing any errors
	conf.Hosts = make(map[string]struct {
		AccessKeyID     string
		SecretAccessKey string
	})
	conf.Aliases = make(map[string]string)
	return conf
}

// newConfigV101() - get new config version 1.0.1
func newConfigV101() *configV101 {
	conf := new(configV101)
	conf.Version = "1.0.1"
	// make sure to allocate map's otherwise Golang
	// exits silently without providing any errors
	conf.Hosts = make(map[string]struct {
		AccessKeyID     string
		SecretAccessKey string
	})
	conf.Aliases = make(map[string]string)
	return conf
}

// newConfigV2() - get new config version 2
func newConfigV2() *configV2 {
	conf := new(configV2)
	conf.Version = "2"
	// make sure to allocate map's otherwise Golang
	// exits silently without providing any errors
	conf.Hosts = make(map[string]struct {
		AccessKeyID     string
		SecretAccessKey string
	})
	conf.Aliases = make(map[string]string)
	return conf
}

// newConfigV3 - get new config version 3
func newConfigV3() *configV3 {
	conf := new(configV3)
	conf.Version = "3"
	// make sure to allocate map's otherwise Golang
	// exits silently without providing any errors
	conf.Hosts = make(map[string]struct {
		AccessKeyID     string `json:"access-key-id"`
		SecretAccessKey string `json:"secret-access-key"`
	})
	conf.Aliases = make(map[string]string)
	return conf
}

func newConfigV4() *configV4 {
	conf := new(configV4)
	conf.Version = "4"
	// make sure to allocate map's otherwise Golang
	// exits silently without providing any errors
	conf.Hosts = make(map[string]struct {
		AccessKeyID     string `json:"accessKeyId"`
		SecretAccessKey string `json:"secretAccessKey"`
		Signature       string `json:"signature"`
	})
	conf.Aliases = make(map[string]string)
	return conf
}

func newConfigV5() *configV5 {
	conf := new(configV5)
	conf.Version = "5"
	// make sure to allocate map's otherwise Golang
	// exits silently without providing any errors
	conf.Hosts = make(map[string]hostConfig)
	conf.Aliases = make(map[string]string)
	return conf
}
