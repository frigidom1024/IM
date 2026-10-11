package main

import (
	"bufio"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// This test double exercises the actual MinIO HTTP client and object bytes.
// It intentionally does not verify S3 signatures or implement multipart storage.
func registerS3Stub(mux *http.ServeMux) {
	directory, err := os.MkdirTemp("", "gochat-s3-e2e-")
	if err != nil {
		panic(err)
	}
	var mutex sync.Mutex
	type objectMetadata struct {
		MIME     string
		ETag     string
		Modified time.Time
		Size     int64
	}
	metadata := map[string]objectMetadata{}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "service": "s3-test-double"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		w.Header().Set("Server", "gochat-s3-test-double")
		w.Header().Set("x-amz-request-id", "test-request")
		if _, ok := r.URL.Query()["location"]; ok {
			w.Header().Set("Content-Type", "application/xml")
			io.WriteString(w, `<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`)
			return
		}
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
		if len(parts) < 2 || parts[1] == "" {
			if r.Method == http.MethodGet {
				prefix := r.URL.Query().Get("prefix")
				startAfter := r.URL.Query().Get("start-after")
				token := r.URL.Query().Get("continuation-token")
				if token != "" {
					decoded, err := base64.RawURLEncoding.DecodeString(token)
					if err != nil {
						http.Error(w, "invalid continuation token", http.StatusBadRequest)
						return
					}
					startAfter = string(decoded)
				}
				maxKeys := 1000
				if value := r.URL.Query().Get("max-keys"); value != "" {
					parsed, err := strconv.Atoi(value)
					if err != nil || parsed < 0 {
						http.Error(w, "invalid max-keys", http.StatusBadRequest)
						return
					}
					if parsed < maxKeys {
						maxKeys = parsed
					}
				}
				keys := make([]string, 0)
				bucketPrefix := parts[0] + "/"
				for storedKey := range metadata {
					if !strings.HasPrefix(storedKey, bucketPrefix) {
						continue
					}
					objectKey := strings.TrimPrefix(storedKey, bucketPrefix)
					if strings.HasPrefix(objectKey, prefix) && objectKey > startAfter {
						keys = append(keys, objectKey)
					}
				}
				sort.Strings(keys)
				truncated := maxKeys > 0 && len(keys) > maxKeys
				if len(keys) > maxKeys {
					keys = keys[:maxKeys]
				}
				type content struct {
					Key          string `xml:"Key"`
					LastModified string `xml:"LastModified"`
					ETag         string `xml:"ETag"`
					Size         int64  `xml:"Size"`
					StorageClass string `xml:"StorageClass"`
				}
				result := struct {
					XMLName               xml.Name  `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListBucketResult"`
					Name                  string    `xml:"Name"`
					Prefix                string    `xml:"Prefix"`
					KeyCount              int       `xml:"KeyCount"`
					MaxKeys               int       `xml:"MaxKeys"`
					IsTruncated           bool      `xml:"IsTruncated"`
					EncodingType          string    `xml:"EncodingType,omitempty"`
					ContinuationToken     string    `xml:"ContinuationToken,omitempty"`
					NextContinuationToken string    `xml:"NextContinuationToken,omitempty"`
					StartAfter            string    `xml:"StartAfter,omitempty"`
					Contents              []content `xml:"Contents"`
				}{Name: parts[0], Prefix: prefix, KeyCount: len(keys), MaxKeys: maxKeys, IsTruncated: truncated, ContinuationToken: token, StartAfter: r.URL.Query().Get("start-after")}
				encodeKey := func(key string) string { return key }
				if r.URL.Query().Get("encoding-type") == "url" {
					result.EncodingType = "url"
					encodeKey = func(key string) string { return strings.ReplaceAll(url.QueryEscape(key), "+", "%20") }
					result.Prefix = encodeKey(prefix)
					result.StartAfter = encodeKey(result.StartAfter)
				}
				for _, key := range keys {
					object := metadata[bucketPrefix+key]
					result.Contents = append(result.Contents, content{Key: encodeKey(key), LastModified: object.Modified.UTC().Format(time.RFC3339Nano), ETag: `"` + object.ETag + `"`, Size: object.Size, StorageClass: "STANDARD"})
				}
				if truncated {
					result.NextContinuationToken = base64.RawURLEncoding.EncodeToString([]byte(keys[len(keys)-1]))
				}
				w.Header().Set("Content-Type", "application/xml")
				_ = xml.NewEncoder(w).Encode(result)
				return
			}
			w.WriteHeader(200)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/")
		sum := md5.Sum([]byte(key))
		filename := filepath.Join(directory, hex.EncodeToString(sum[:]))
		switch r.Method {
		case http.MethodPut:
			// This header exists only in the E2E test double, allowing orphan
			// maintenance fixtures without waiting for the production grace period.
			var fixtureModified time.Time
			if value := r.Header.Get("X-GoChat-Test-Modified"); value != "" {
				parsed, parseErr := time.Parse(time.RFC3339Nano, value)
				if parseErr != nil {
					http.Error(w, "invalid test modification time", 400)
					return
				}
				fixtureModified = parsed
			}
			file, err := os.Create(filename)
			if err != nil {
				http.Error(w, "disk error", 500)
				return
			}
			hasher := md5.New()
			if strings.Contains(r.Header.Get("Content-Encoding"), "aws-chunked") || strings.HasPrefix(r.Header.Get("X-Amz-Content-Sha256"), "STREAMING-") {
				err = decodeAWSChunks(io.MultiWriter(file, hasher), r.Body)
			} else {
				_, err = io.Copy(io.MultiWriter(file, hasher), io.LimitReader(r.Body, 16<<20))
			}
			file.Close()
			if err != nil {
				http.Error(w, "write error", 500)
				return
			}
			stat, err := os.Stat(filename)
			if err != nil {
				http.Error(w, "stat error", 500)
				return
			}
			etag := hex.EncodeToString(hasher.Sum(nil))
			modified := stat.ModTime()
			if !fixtureModified.IsZero() {
				modified = fixtureModified
			}
			metadata[key] = objectMetadata{MIME: r.Header.Get("Content-Type"), ETag: etag, Modified: modified, Size: stat.Size()}
			w.Header().Set("ETag", `"`+etag+`"`)
			w.WriteHeader(200)
		case http.MethodGet, http.MethodHead:
			file, err := os.Open(filename)
			if err != nil {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(404)
				xml.NewEncoder(w).Encode(struct {
					XMLName xml.Name `xml:"Error"`
					Code    string   `xml:"Code"`
				}{Code: "NoSuchKey"})
				return
			}
			defer file.Close()
			hasher := md5.New()
			io.Copy(hasher, file)
			file.Seek(0, 0)
			w.Header().Set("ETag", `"`+hex.EncodeToString(hasher.Sum(nil))+`"`)
			w.Header().Set("Content-Type", metadata[key].MIME)
			modified := metadata[key].Modified
			w.Header().Set("Last-Modified", modified.UTC().Format(http.TimeFormat))
			http.ServeContent(w, r, parts[1], modified, file)
		case http.MethodDelete:
			os.Remove(filename)
			delete(metadata, key)
			w.WriteHeader(204)
		default:
			w.WriteHeader(405)
		}
	})
}

func decodeAWSChunks(writer io.Writer, reader io.Reader) error {
	buffered := bufio.NewReader(reader)
	total := int64(0)
	for {
		header, err := buffered.ReadString('\n')
		if err != nil {
			return err
		}
		length, err := strconv.ParseInt(strings.Split(strings.TrimSpace(header), ";")[0], 16, 64)
		if err != nil || length < 0 || total+length > 16<<20 {
			return io.ErrUnexpectedEOF
		}
		if length == 0 {
			return nil
		}
		if _, err = io.CopyN(writer, buffered, length); err != nil {
			return err
		}
		if _, err = buffered.ReadString('\n'); err != nil {
			return err
		}
		total += length
	}
}
