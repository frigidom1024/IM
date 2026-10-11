package main

import (
	"context"
	"encoding/xml"
	"github.com/minio/minio-go/v7"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestS3ListManagedObjects(t *testing.T) {
	mux := http.NewServeMux()
	registerS3Stub(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	client, err := minio.New(strings.TrimPrefix(server.URL, "http://"), &minio.Options{})
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"attachments/a& b", "attachments/z", "thumbnails/other"} {
		request := httptest.NewRequest(http.MethodPut, server.URL+"/gochat/"+url.PathEscape(key), strings.NewReader("payload"))
		request.RequestURI = ""
		request.Header.Set("Content-Type", "text/plain")
		request.Header.Set("X-GoChat-Test-Modified", "2010-01-01T00:00:00Z")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("put status %d", response.StatusCode)
		}
	}
	response, err := http.Get(server.URL + "/gochat?list-type=2&prefix=attachments%2F&encoding-type=url&max-keys=1")
	if err != nil {
		t.Fatal(err)
	}
	var listing struct {
		XMLName               xml.Name
		Prefix                string
		KeyCount              int
		IsTruncated           bool
		NextContinuationToken string
		Contents              []struct {
			Key, LastModified, ETag string
			Size                    int64
		}
	}
	err = xml.NewDecoder(response.Body).Decode(&listing)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || listing.XMLName.Local != "ListBucketResult" || listing.XMLName.Space != "http://s3.amazonaws.com/doc/2006-03-01/" || listing.Prefix != "attachments%2F" || listing.KeyCount != 1 || !listing.IsTruncated || listing.NextContinuationToken == "" || len(listing.Contents) != 1 {
		t.Fatalf("invalid listing: %#v, err %v", listing, err)
	}
	if listing.Contents[0].Key != "attachments%2Fa%26%20b" || listing.Contents[0].LastModified != "2010-01-01T00:00:00Z" || listing.Contents[0].ETag != `"321c3cf486ed509164edec1e1981fec8"` || listing.Contents[0].Size != 7 {
		t.Fatalf("invalid XML object: %#v", listing.Contents[0])
	}

	count := 0
	for object := range client.ListObjects(context.Background(), "gochat", minio.ListObjectsOptions{Prefix: "attachments/", Recursive: true, MaxKeys: 1}) {
		if object.Err != nil {
			t.Fatal(object.Err)
		}
		if !strings.HasPrefix(object.Key, "attachments/") || object.Size != 7 || object.ETag != "321c3cf486ed509164edec1e1981fec8" || object.LastModified.IsZero() || !object.LastModified.Before(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("incorrect object: %#v", object)
		}
		count++
	}
	if count != 2 {
		t.Fatalf("listed %d objects", count)
	}
	count = 0
	for object := range client.ListObjects(context.Background(), "gochat", minio.ListObjectsOptions{Prefix: "absent/", Recursive: true}) {
		if object.Err != nil {
			t.Fatal(object.Err)
		}
		count++
	}
	if count != 0 {
		t.Fatal("empty prefix returned objects")
	}
}
