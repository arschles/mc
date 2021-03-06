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

package client

import (
	"bytes"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// URL client url structure
type URL struct {
	Type            URLType
	Scheme          string
	Host            string
	Path            string
	SchemeSeparator string
	Separator       rune
}

// URLType - enum of different url types
type URLType int

// enum types
const (
	Object     = iota // Minio and S3 compatible cloud storage
	Filesystem        // POSIX compatible file systems
)

// Maybe rawurl is of the form scheme:path. (Scheme must be [a-zA-Z][a-zA-Z0-9+-.]*)
// If so, return scheme, path; else return "", rawurl.
func getScheme(rawurl string) (scheme, path string) {
	urlSplits := strings.Split(rawurl, "://")
	if len(urlSplits) == 2 {
		scheme, uri := urlSplits[0], "//"+urlSplits[1]
		// ignore numbers in scheme
		validScheme := regexp.MustCompile("^[a-zA-Z]+$")
		if uri != "" {
			if validScheme.MatchString(scheme) {
				return scheme, uri
			}
		}
	}
	return "", rawurl
}

// Assuming s is of the form [s delimiter s].
// If so, return s, [delimiter]s or return s, s if cutdelimiter == true
// If no delimiter found return s, "".
func splitSpecial(s string, delimiter string, cutdelimiter bool) (string, string) {
	i := strings.Index(s, delimiter)
	if i < 0 {
		// if delimiter not found return as is.
		return s, ""
	}
	// if delimiter should be removed, remove it.
	if cutdelimiter {
		return s[0:i], s[i+len(delimiter):]
	}
	// return split strings with delimiter
	return s[0:i], s[i:]
}

// getHost - extract host from authority string, we do not support ftp style username@ yet.
func getHost(authority string) (host string) {
	i := strings.LastIndex(authority, "@")
	if i >= 0 {
		// TODO support, username@password style userinfo, useful for ftp support.
		return
	}
	return authority
}

// NewURL returns an abstracted URL for filesystems and object storage.
func NewURL(urlStr string) *URL {
	scheme, rest := getScheme(urlStr)
	rest, _ = splitSpecial(rest, "?", true)
	if strings.HasPrefix(rest, "//") {
		// if rest has '//' prefix, skip them
		var authority string
		authority, rest = splitSpecial(rest[2:], "/", false)
		if rest == "" {
			rest = "/"
		}
		host := getHost(authority)
		if host != "" && (scheme == "http" || scheme == "https") {
			return &URL{
				Scheme:          scheme,
				Type:            Object,
				Host:            host,
				Path:            rest,
				SchemeSeparator: "://",
				Separator:       '/',
			}
		}
	}
	return &URL{
		Type:      Filesystem,
		Path:      rest,
		Separator: filepath.Separator,
	}
}

// JoinURLs join two input urls and returns a url
func JoinURLs(url1, url2 *URL) *URL {
	var url1Path, url2Path string
	url1Path = url1.Path
	url2Path = url2.Path
	if runtime.GOOS == "windows" {
		url1Path = strings.Replace(url1.Path, "\\", "/", -1)
		url2Path = strings.Replace(url2.Path, "\\", "/", -1)
	}
	if url1.Type == Object {
		if strings.HasSuffix(url1Path, "/") {
			url1.Path = url1Path + strings.TrimPrefix(url2Path, "/")
		} else {
			url1.Path = url1Path + "/" + strings.TrimPrefix(url2Path, "/")
		}
	}
	if url1.Type == Filesystem {
		if strings.HasSuffix(url1Path, "/") {
			url1.Path = url1Path + strings.TrimPrefix(url2Path, "/")
		} else {
			url1.Path = url1Path + "/" + strings.TrimPrefix(url2Path, "/")
		}
		if runtime.GOOS == "windows" {
			url1.Path = strings.Replace(url1.Path, "/", "\\", -1)
		}
	}
	return url1
}

// String convert URL into its canonical form.
func (u URL) String() string {
	var buf bytes.Buffer
	// if fileystem no translation needed, return as is.
	if u.Type == Filesystem {
		return u.Path
	}
	// if Object convert from any non standard paths to a supported URL path style.
	if u.Type == Object {
		buf.WriteString(u.Scheme)
		buf.WriteByte(':')
		buf.WriteString("//")
		if h := u.Host; h != "" {
			buf.WriteString(h)
		}
		switch runtime.GOOS {
		case "windows":
			if u.Path != "" && u.Path[0] != '\\' && u.Host != "" && u.Path[0] != '/' {
				buf.WriteByte('/')
			}
			buf.WriteString(strings.Replace(u.Path, "\\", "/", -1))
		default:
			if u.Path != "" && u.Path[0] != '/' && u.Host != "" {
				buf.WriteByte('/')
			}
			buf.WriteString(u.Path)
		}
	}
	return buf.String()
}
