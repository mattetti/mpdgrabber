package mpdgrabber

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var Debug = false
var Logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

const UnknownString = "unknown"

// always returns a copy
func absBaseURL(manifestBaseURL *url.URL, elBaseURLs []string) *url.URL {
	if len(elBaseURLs) == 0 {
		u := *manifestBaseURL
		return &u
	}
	elBaseURL := elBaseURLs[0]
	u, err := url.Parse(elBaseURL)
	if err != nil {
		if Debug {
			fmt.Printf("failed to parse the base url %s - %s\n", elBaseURL, err)
		}
		return manifestBaseURL
	}
	if u.IsAbs() {
		return u
	}
	return manifestBaseURL.ResolveReference(u)
}

func extractContentType(contentType, mimeType *string) string {
	if contentType != nil {
		return strings.ToLower(*contentType)
	}
	if mimeType != nil {
		mType := strings.ToLower(*mimeType)
		if strings.Contains(mType, "video") {
			return "video"
		}
		if strings.Contains(mType, "audio") {
			return "audio"
		}
		if strings.Contains(mType, "text") {
			return "text"
		}
	}
	return UnknownString
}

// downloadFile downloads a file from a given url and saves it to a given path
// it returns the file and an error if something goes wrong
// It's the caller's responsibility to close the file.
func downloadFile(url string, path string) (*os.File, error) {

	// check if there is a valid file at `path`
	if fileExists(path) {
		if Debug {
			fmt.Println("-> File already exists at", path)
		}
		return os.Open(path)
	}

	// Create the file
	out, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	// build the request with the proper headers
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	// req.Header.Add("Accept", "application/dash+xml,video/vnd.mpeg.dash.mpd")

	// call the request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func strPtrtoS(s *string) string {
	if s == nil {
		return UnknownString
	}
	return *s
}

func int64PtrToI(d *int64) int {
	if d == nil {
		return 0
	}
	return int(*d)
}

func uint64PtrToI(d *uint64) int {
	if d == nil {
		return 0
	}
	return int(*d)
}

func uint32PtrToI(d *uint32) int {
	if d == nil {
		return 0
	}
	return int(*d)
}

func boolPtrToB(d *bool) bool {
	if d == nil {
		return false
	}
	return *d
}
