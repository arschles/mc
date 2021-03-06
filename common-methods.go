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
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/minio/mc/pkg/client"
	"github.com/minio/mc/pkg/client/fs"
	"github.com/minio/mc/pkg/client/s3"
	"github.com/minio/minio-xl/pkg/probe"
)

// Check if the target URL represents folder. It may or may not exist yet.
func isTargetURLDir(targetURL string) bool {
	targetURLParse := client.NewURL(targetURL)
	_, targetContent, err := url2Stat(targetURL)
	if err != nil {
		if targetURLParse.Path == string(targetURLParse.Separator) && targetURLParse.Scheme != "" {
			return false
		}
		if strings.HasSuffix(targetURLParse.Path, string(targetURLParse.Separator)) {
			return true
		}
		return false
	}
	if !targetContent.Type.IsDir() { // Target is a dir.
		return false
	}
	return true
}

// getSource gets a reader from URL
func getSource(sourceURL string) (reader io.ReadSeeker, err *probe.Error) {
	sourceClnt, err := url2Client(sourceURL)
	if err != nil {
		return nil, err.Trace(sourceURL)
	}
	return sourceClnt.Get(0, 0)
}

// putTarget writes to URL from reader. If length=-1, read until EOF.
func putTarget(targetURL string, reader io.ReadSeeker, size int64) *probe.Error {
	targetClnt, err := url2Client(targetURL)
	if err != nil {
		return err.Trace(targetURL)
	}
	err = targetClnt.Put(reader, size)
	if err != nil {
		return err.Trace(targetURL)
	}
	return nil
}

// getNewClient gives a new client interface
func getNewClient(urlStr string, auth hostConfig) (client.Client, *probe.Error) {
	url := client.NewURL(urlStr)
	switch url.Type {
	case client.Object: // Minio and S3 compatible cloud storage
		s3Config := new(client.Config)
		s3Config.AccessKeyID = func() string {
			if auth.AccessKeyID == globalAccessKeyID {
				return ""
			}
			return auth.AccessKeyID
		}()
		s3Config.SecretAccessKey = func() string {
			if auth.SecretAccessKey == globalSecretAccessKey {
				return ""
			}
			return auth.SecretAccessKey
		}()
		s3Config.Signature = auth.API
		s3Config.AppName = "Minio"
		s3Config.AppVersion = mcVersion
		s3Config.AppComments = []string{os.Args[0], runtime.GOOS, runtime.GOARCH}
		s3Config.HostURL = urlStr
		s3Config.Debug = globalDebug

		s3Client, err := s3.New(s3Config)
		if err != nil {
			return nil, err.Trace(urlStr)
		}
		return s3Client, nil
	case client.Filesystem:
		fsClient, err := fs.New(urlStr)
		if err != nil {
			return nil, err.Trace(urlStr)
		}
		return fsClient, nil
	}
	return nil, errInitClient(urlStr).Trace(urlStr)
}
