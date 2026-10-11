package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"strconv"
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
	metadata := map[string]string{}
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
			w.WriteHeader(200)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/")
		sum := md5.Sum([]byte(key))
		filename := filepath.Join(directory, hex.EncodeToString(sum[:]))
		switch r.Method {
		case http.MethodPut:
			file, err := os.Create(filename)
			if err != nil {
				http.Error(w, "disk error", 500)
				return
			}
			hasher := md5.New()
			if strings.Contains(r.Header.Get("Content-Encoding"),"aws-chunked") {
				err = decodeAWSChunks(io.MultiWriter(file,hasher),r.Body)
			} else {
				_, err = io.Copy(io.MultiWriter(file, hasher), io.LimitReader(r.Body, 16<<20))
			}
			file.Close()
			if err != nil {
				http.Error(w, "write error", 500)
				return
			}
			metadata[key] = r.Header.Get("Content-Type")
			w.Header().Set("ETag", `"`+hex.EncodeToString(hasher.Sum(nil))+`"`)
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
			w.Header().Set("Content-Type", metadata[key])
			w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
			http.ServeContent(w, r, parts[1], time.Now(), file)
		case http.MethodDelete:
			os.Remove(filename)
			delete(metadata, key)
			w.WriteHeader(204)
		default:
			w.WriteHeader(405)
		}
	})
}

func decodeAWSChunks(writer io.Writer,reader io.Reader) error {
 buffered:=bufio.NewReader(reader)
 total:=int64(0)
 for {
  header,err:=buffered.ReadString('\n');if err!=nil { return err }
  length,err:=strconv.ParseInt(strings.Split(strings.TrimSpace(header),";")[0],16,64);if err!=nil || length<0 || total+length>16<<20 { return io.ErrUnexpectedEOF }
  if length==0 { return nil }
  if _,err=io.CopyN(writer,buffered,length);err!=nil { return err }
  if _,err=buffered.ReadString('\n');err!=nil { return err };total+=length
 }
}
