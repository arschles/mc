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

package s3

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/minio/mc/pkg/client"
	"github.com/minio/mc/pkg/httptracer"
	"github.com/minio/minio-go"
	"github.com/minio/minio-xl/pkg/probe"
)

// S3 client
type s3Client struct {
	mu           *sync.Mutex
	api          minio.CloudStorageAPI
	hostURL      *client.URL
	virtualStyle bool
}

// New returns an initialized s3Client structure. if debug use a internal trace transport.
func New(config *client.Config) (client.Client, *probe.Error) {
	u := client.NewURL(config.HostURL)
	transport := http.DefaultTransport
	if config.Debug == true {
		if config.Signature == "S3v4" {
			transport = httptracer.GetNewTraceTransport(NewTraceV4(), http.DefaultTransport)
		}
		if config.Signature == "S3v2" {
			transport = httptracer.GetNewTraceTransport(NewTraceV2(), http.DefaultTransport)
		}
	}
	s3Conf := minio.Config{
		AccessKeyID:     config.AccessKeyID,
		SecretAccessKey: config.SecretAccessKey,
		Transport:       transport,
		Endpoint:        u.Scheme + u.SchemeSeparator + u.Host,
		Signature: func() minio.SignatureType {
			if config.Signature == "S3v2" {
				return minio.SignatureV2
			}
			return minio.SignatureV4
		}(),
	}
	s3Conf.SetUserAgent(config.AppName, config.AppVersion, config.AppComments...)
	api, err := minio.New(s3Conf)
	if err != nil {
		return nil, probe.NewError(err)
	}
	s3Clnt := &s3Client{
		mu:           new(sync.Mutex),
		api:          api,
		hostURL:      u,
		virtualStyle: isVirtualHostStyle(u.Host),
	}
	return s3Clnt, nil
}

// GetURL get url.
func (c *s3Client) GetURL() client.URL {
	return *c.hostURL
}

// Get - get object.
func (c *s3Client) Get(offset, length int64) (io.ReadSeeker, *probe.Error) {
	bucket, object := c.url2BucketAndObject()
	reader, err := c.api.GetPartialObject(bucket, object, offset, length)
	if err != nil {
		errResponse := minio.ToErrorResponse(err)
		if errResponse != nil {
			if errResponse.Code == "AccessDenied" {
				return nil, probe.NewError(client.PathInsufficientPermission{Path: c.hostURL.String()})
			}
		}
		return nil, probe.NewError(err)
	}
	return reader, nil
}

// Remove - remove object or bucket.
func (c *s3Client) Remove(incomplete bool) *probe.Error {
	bucket, object := c.url2BucketAndObject()
	if incomplete {
		errCh := c.api.RemoveIncompleteUpload(bucket, object)
		return probe.NewError(<-errCh)
	}
	var err error
	if object == "" {
		err = c.api.RemoveBucket(bucket)
	} else {
		err = c.api.RemoveObject(bucket, object)
	}
	return probe.NewError(err)
}

// ShareDownload - get a usable presigned object url to share.
func (c *s3Client) ShareDownload(expires time.Duration) (string, *probe.Error) {
	bucket, object := c.url2BucketAndObject()
	presignedURL, err := c.api.PresignedGetObject(bucket, object, expires)
	if err != nil {
		return "", probe.NewError(err)
	}
	return presignedURL, nil
}

// ShareUpload - get data for presigned post http form upload.
func (c *s3Client) ShareUpload(isRecursive bool, expires time.Duration, contentType string) (map[string]string, *probe.Error) {
	bucket, object := c.url2BucketAndObject()
	p := minio.NewPostPolicy()
	if err := p.SetExpires(time.Now().UTC().Add(expires)); err != nil {
		return nil, probe.NewError(err)
	}
	if strings.TrimSpace(contentType) != "" || contentType != "" {
		// No need to verify for error here, since we have stripped out spaces.
		p.SetContentType(contentType)
	}
	if err := p.SetBucket(bucket); err != nil {
		return nil, probe.NewError(err)
	}
	if isRecursive {
		if err := p.SetKeyStartsWith(object); err != nil {
			return nil, probe.NewError(err)
		}
	} else {
		if err := p.SetKey(object); err != nil {
			return nil, probe.NewError(err)
		}
	}
	m, err := c.api.PresignedPostPolicy(p)
	return m, probe.NewError(err)
}

// Put - put object.
func (c *s3Client) Put(data io.ReadSeeker, size int64) *probe.Error {
	// md5 is purposefully ignored since AmazonS3 does not return proper md5sum
	// for a multipart upload and there is no need to cross verify,
	// invidual parts are properly verified fully in transit and also upon completion
	// of the multipart request.
	bucket, object := c.url2BucketAndObject()
	err := c.api.PutObject(bucket, object, data, size, "application/octet-stream")
	if err != nil {
		errResponse := minio.ToErrorResponse(err)
		if errResponse != nil {
			if errResponse.Code == "AccessDenied" {
				return probe.NewError(client.PathInsufficientPermission{
					Path: c.hostURL.String(),
				})
			}
			if errResponse.Code == "MethodNotAllowed" {
				return probe.NewError(client.ObjectAlreadyExists{
					Object: object,
				})
			}
			if errResponse.Code == "InvalidArgument" {
				return probe.NewError(client.ObjectMissing{})
			}
		}
		return probe.NewError(err)
	}
	return nil
}

// MakeBucket - make a new bucket.
func (c *s3Client) MakeBucket() *probe.Error {
	bucket, object := c.url2BucketAndObject()
	if object != "" {
		return probe.NewError(client.BucketNameTopLevel{})
	}
	if len(bucket) < 3 || len(bucket) > 63 {
		return probe.NewError(errors.New("Bucket name should be more than 3 characters and less than 64 characters"))
	}
	match, _ := regexp.MatchString("^[a-zA-Z][a-zA-Z0-9\\-]+[a-zA-Z0-9]$", bucket)
	if !match {
		return probe.NewError(errors.New("Bucket name can contain alphabet, '-' and numbers, but first character should be an alphabet"))
	}

	err := c.api.MakeBucket(bucket, minio.BucketACL("private"))
	if err != nil {
		return probe.NewError(err)
	}
	return nil
}

// GetBucketAccess get acl on a bucket.
func (c *s3Client) GetBucketAccess() (acl string, error *probe.Error) {
	bucket, object := c.url2BucketAndObject()
	if object != "" {
		return "", probe.NewError(client.InvalidBucketName{Bucket: filepath.Join(bucket, object)})
	}
	if bucket == "" {
		return "", probe.NewError(client.BucketNameEmpty{})
	}
	bucketACL, err := c.api.GetBucketACL(bucket)
	if err != nil {
		return "", probe.NewError(err)
	}
	return bucketACL.String(), nil
}

// SetBucketAccess set acl on a bucket
func (c *s3Client) SetBucketAccess(acl string) *probe.Error {
	bucket, object := c.url2BucketAndObject()
	if object != "" {
		return probe.NewError(client.InvalidBucketName{Bucket: filepath.Join(bucket, object)})
	}
	if bucket == "" {
		return probe.NewError(client.BucketNameEmpty{})
	}
	err := c.api.SetBucketACL(bucket, minio.BucketACL(acl))
	if err != nil {
		return probe.NewError(err)
	}
	return nil
}

// Stat - send a 'HEAD' on a bucket or object to fetch its metadata.
func (c *s3Client) Stat() (*client.Content, *probe.Error) {
	c.mu.Lock()
	objectMetadata := new(client.Content)
	bucket, object := c.url2BucketAndObject()
	switch {
	// valid case for '-r s3/'
	case bucket == "" && object == "":
		for bucket := range c.api.ListBuckets() {
			if bucket.Err != nil {
				c.mu.Unlock()
				return nil, probe.NewError(bucket.Err)
			}
		}
		c.mu.Unlock()
		return &client.Content{URL: *c.hostURL, Type: os.ModeDir}, nil
	}
	if object != "" {
		metadata, err := c.api.StatObject(bucket, object)
		if err != nil {
			c.mu.Unlock()
			errResponse := minio.ToErrorResponse(err)
			if errResponse != nil {
				if errResponse.Code == "NoSuchKey" {
					// Append "/" to the object name proactively and see if the Listing
					// produces an output. If yes, then we treat it as a directory.
					prefixName := object
					// Trim any trailing separators and add it.
					prefixName = strings.TrimSuffix(prefixName, string(c.hostURL.Separator)) + string(c.hostURL.Separator)
					for objectStat := range c.api.ListObjects(bucket, prefixName, false) {
						if objectStat.Err != nil {
							return nil, probe.NewError(objectStat.Err)
						}
						content := client.Content{}
						content.URL = *c.hostURL
						content.Type = os.ModeDir
						return &content, nil
					}
					return nil, probe.NewError(client.PathNotFound{Path: c.hostURL.Path})
				}
			}
			return nil, probe.NewError(err)
		}
		objectMetadata.URL = *c.hostURL
		objectMetadata.Time = metadata.LastModified
		objectMetadata.Size = metadata.Size
		objectMetadata.Type = os.FileMode(0664)
		c.mu.Unlock()
		return objectMetadata, nil
	}
	err := c.api.BucketExists(bucket)
	if err != nil {
		c.mu.Unlock()
		return nil, probe.NewError(err)
	}
	bucketMetadata := new(client.Content)
	bucketMetadata.URL = *c.hostURL
	bucketMetadata.Type = os.ModeDir
	c.mu.Unlock()
	return bucketMetadata, nil
}

// Figure out if the URL is of 'virtual host' style.
// Currently only supported hosts with virtual style are Amazon S3 and Google Cloud Storage.
func isVirtualHostStyle(hostURL string) bool {
	matchS3, _ := filepath.Match("*.s3*.amazonaws.com", hostURL)
	matchGoogle, _ := filepath.Match("*.storage.googleapis.com", hostURL)
	return matchS3 || matchGoogle
}

// url2BucketAndObject gives bucketName and objectName from URL path.
func (c *s3Client) url2BucketAndObject() (bucketName, objectName string) {
	path := c.hostURL.Path
	// Convert any virtual host styled requests.
	//
	// For the time being this check is introduced for S3,
	// If you have custom virtual styled hosts please.
	// List them below.
	if c.virtualStyle {
		var bucket string
		hostIndex := strings.Index(c.hostURL.Host, "s3")
		if hostIndex == -1 {
			hostIndex = strings.Index(c.hostURL.Host, "storage.googleapis")
		}
		if hostIndex > 0 {
			bucket = c.hostURL.Host[:hostIndex-1]
			path = string(c.hostURL.Separator) + bucket + c.hostURL.Path
		}
	}
	splits := strings.SplitN(path, string(c.hostURL.Separator), 3)
	switch len(splits) {
	case 0, 1:
		bucketName = ""
		objectName = ""
	case 2:
		bucketName = splits[1]
		objectName = ""
	case 3:
		bucketName = splits[1]
		objectName = splits[2]
	}
	return bucketName, objectName
}

/// Bucket API operations.

// List - list at delimited path, if not recursive.
func (c *s3Client) List(recursive, incomplete bool) <-chan *client.Content {
	c.mu.Lock()
	defer c.mu.Unlock()

	contentCh := make(chan *client.Content)
	if incomplete {
		if recursive {
			go c.listIncompleteRecursiveInRoutine(contentCh)
		} else {
			go c.listIncompleteInRoutine(contentCh)
		}
	} else {
		if recursive {
			go c.listRecursiveInRoutine(contentCh)
		} else {
			go c.listInRoutine(contentCh)
		}
	}
	return contentCh
}

func (c *s3Client) listIncompleteInRoutine(contentCh chan *client.Content) {
	defer close(contentCh)
	// get bucket and object from URL.
	b, o := c.url2BucketAndObject()
	switch {
	case b == "" && o == "":
		for bucket := range c.api.ListBuckets() {
			if bucket.Err != nil {
				contentCh <- &client.Content{
					Err: probe.NewError(bucket.Err),
				}
				return
			}
			for object := range c.api.ListIncompleteUploads(bucket.Name, o, false) {
				if object.Err != nil {
					contentCh <- &client.Content{
						Err: probe.NewError(object.Err),
					}
					return
				}
				content := new(client.Content)
				url := *c.hostURL
				// Join bucket with - incoming object key.
				url.Path = filepath.Join(string(url.Separator), bucket.Name, object.Key)
				if c.virtualStyle {
					url.Path = filepath.Join(string(url.Separator), object.Key)
				}
				switch {
				case strings.HasSuffix(object.Key, string(c.hostURL.Separator)):
					// We need to keep the trailing Separator, do not use filepath.Join().
					content.URL = url
					content.Time = time.Now()
					content.Type = os.ModeDir
				default:
					content.URL = url
					content.Size = object.Size
					content.Time = object.Initiated
					content.Type = os.ModeTemporary
				}
				contentCh <- content
			}
		}
	default:
		for object := range c.api.ListIncompleteUploads(b, o, false) {
			if object.Err != nil {
				contentCh <- &client.Content{
					Err: probe.NewError(object.Err),
				}
				return
			}
			content := new(client.Content)
			url := *c.hostURL
			// Join bucket with - incoming object key.
			url.Path = filepath.Join(string(url.Separator), b, object.Key)
			if c.virtualStyle {
				url.Path = filepath.Join(string(url.Separator), object.Key)
			}
			switch {
			case strings.HasSuffix(object.Key, string(c.hostURL.Separator)):
				// We need to keep the trailing Separator, do not use filepath.Join().
				content.URL = url
				content.Time = time.Now()
				content.Type = os.ModeDir
			default:
				content.URL = url
				content.Size = object.Size
				content.Time = object.Initiated
				content.Type = os.ModeTemporary
			}
			contentCh <- content
		}
	}
}

func (c *s3Client) listIncompleteRecursiveInRoutine(contentCh chan *client.Content) {
	defer close(contentCh)
	// get bucket and object from URL.
	b, o := c.url2BucketAndObject()
	switch {
	case b == "" && o == "":
		for bucket := range c.api.ListBuckets() {
			if bucket.Err != nil {
				contentCh <- &client.Content{
					Err: probe.NewError(bucket.Err),
				}
				return
			}
			for object := range c.api.ListIncompleteUploads(bucket.Name, o, true) {
				if object.Err != nil {
					contentCh <- &client.Content{
						Err: probe.NewError(object.Err),
					}
					return
				}
				content := new(client.Content)
				url := *c.hostURL
				url.Path = filepath.Join(url.Path, bucket.Name, object.Key)
				content.URL = url
				content.Size = object.Size
				content.Time = object.Initiated
				content.Type = os.ModeTemporary
				contentCh <- content
			}
		}
	default:
		for object := range c.api.ListIncompleteUploads(b, o, true) {
			if object.Err != nil {
				contentCh <- &client.Content{
					Err: probe.NewError(object.Err),
				}
				return
			}
			url := *c.hostURL
			// Join bucket and incoming object key.
			url.Path = filepath.Join(string(url.Separator), b, object.Key)
			if c.virtualStyle {
				url.Path = filepath.Join(string(url.Separator), object.Key)
			}
			content := new(client.Content)
			content.URL = url
			content.Size = object.Size
			content.Time = object.Initiated
			content.Type = os.ModeTemporary
			contentCh <- content
		}
	}
}

func (c *s3Client) listInRoutine(contentCh chan *client.Content) {
	defer close(contentCh)
	// get bucket and object from URL.
	b, o := c.url2BucketAndObject()
	switch {
	case b == "" && o == "":
		for bucket := range c.api.ListBuckets() {
			if bucket.Err != nil {
				contentCh <- &client.Content{
					Err: probe.NewError(bucket.Err),
				}
				return
			}
			url := *c.hostURL
			url.Path = filepath.Join(url.Path, bucket.Name)
			content := new(client.Content)
			content.URL = url
			content.Size = 0
			content.Time = bucket.CreationDate
			content.Type = os.ModeDir
			contentCh <- content
		}
	case b != "" && !strings.HasSuffix(c.hostURL.Path, string(c.hostURL.Separator)) && o == "":
		err := c.api.BucketExists(b)
		if err != nil {
			contentCh <- &client.Content{
				Err: probe.NewError(err),
			}
		}
		content := new(client.Content)
		content.URL = *c.hostURL
		content.Type = os.ModeDir
		contentCh <- content
	default:
		metadata, err := c.api.StatObject(b, o)
		switch err.(type) {
		case nil:
			content := new(client.Content)
			content.URL = *c.hostURL
			content.Time = metadata.LastModified
			content.Size = metadata.Size
			content.Type = os.FileMode(0664)
			contentCh <- content
		default:
			for object := range c.api.ListObjects(b, o, false) {
				if object.Err != nil {
					contentCh <- &client.Content{
						Err: probe.NewError(object.Err),
					}
					return
				}
				content := new(client.Content)
				url := *c.hostURL
				// Join bucket and incoming object key.
				url.Path = filepath.Join(string(url.Separator), b, object.Key)
				if c.virtualStyle {
					url.Path = filepath.Join(string(url.Separator), object.Key)
				}
				switch {
				case strings.HasSuffix(object.Key, string(c.hostURL.Separator)):
					// We need to keep the trailing Separator, do not use filepath.Join().
					content.URL = url
					content.Time = time.Now()
					content.Type = os.ModeDir
				default:
					content.URL = url
					content.Size = object.Size
					content.Time = object.LastModified
					content.Type = os.FileMode(0664)
				}
				contentCh <- content
			}
		}
	}
}

func (c *s3Client) listRecursiveInRoutine(contentCh chan *client.Content) {
	defer close(contentCh)
	// get bucket and object from URL.
	b, o := c.url2BucketAndObject()
	switch {
	case b == "" && o == "":
		for bucket := range c.api.ListBuckets() {
			if bucket.Err != nil {
				contentCh <- &client.Content{
					Err: probe.NewError(bucket.Err),
				}
				return
			}
			bucketURL := *c.hostURL
			bucketURL.Path = filepath.Join(bucketURL.Path, bucket.Name)
			contentCh <- &client.Content{
				URL:  bucketURL,
				Type: os.ModeDir,
				Time: bucket.CreationDate,
			}
			for object := range c.api.ListObjects(bucket.Name, o, true) {
				if object.Err != nil {
					contentCh <- &client.Content{
						Err: probe.NewError(object.Err),
					}
					continue
				}
				content := new(client.Content)
				objectURL := *c.hostURL
				objectURL.Path = filepath.Join(objectURL.Path, bucket.Name, object.Key)
				content.URL = objectURL
				content.Size = object.Size
				content.Time = object.LastModified
				content.Type = os.FileMode(0664)
				contentCh <- content
			}
		}
	default:
		for object := range c.api.ListObjects(b, o, true) {
			if object.Err != nil {
				contentCh <- &client.Content{
					Err: probe.NewError(object.Err),
				}
				continue
			}
			content := new(client.Content)
			url := *c.hostURL
			// Join bucket and incoming object key.
			url.Path = filepath.Join(string(url.Separator), b, object.Key)
			// If virtualStyle replace the url.Path back.
			if c.virtualStyle {
				url.Path = filepath.Join(string(url.Separator), object.Key)
			}
			content.URL = url
			content.Size = object.Size
			content.Time = object.LastModified
			content.Type = os.FileMode(0664)
			contentCh <- content
		}
	}
}
