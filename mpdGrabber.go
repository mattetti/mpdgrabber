package mpdgrabber

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/mattetti/go-dash/mpd"
)

var Debug = false
var Logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

const UnknownString = "unknown"

// FIXME: we need to return a slice of structs to report the duration of each segment
func templateSubstitution(templateStr *string, representation *mpd.Representation) (urls []string) {
	if templateStr == nil {
		return urls
	}
	template := *templateStr

	/*
		$identifier$	Substitution parameter
		$$	An escape sequence, i.e., “$$” is replaced with a single “$”.
		$RepresentationID$	The player substitutes this identifier with the value of the attribute Representation@id of the containing Representation.
		$Number$	The player substitutes this identifier with the number of the corresponding Segment.
		$Bandwidth$	The player substitutes this identifier with the value of Representation@bandwidth attribute value.
		$Time$	The player substitutes this identifier with the value of the SegmentTimeline@t attribute for the Segment. You can use either $Number$ or $Time$, but not both at the same time.
	*/

	if strings.Contains(template, "$RepresentationID$") {
		template = strings.Replace(template, "$RepresentationID$", strPtrtoS(representation.ID), -1)
	}

	// Time-Based SegmentTemplate
	// $Time$ identifier, which will be substituted with the value of the t attribute from the SegmentTimeline.
	if strings.Contains(template, "$Time$") {
		fmt.Println("\t-> Time-based SegmentTemplate")
		var segTemplate *mpd.SegmentTemplate
		if representation.AdaptationSet != nil && representation.AdaptationSet.SegmentTemplate != nil {
			segTemplate = representation.AdaptationSet.SegmentTemplate
		} else {
			segTemplate = representation.SegmentTemplate
		}
		fmt.Printf("**%+v\n", segTemplate)

		// segment timeline
		if segTemplate.SegmentTimeline != nil {
			if Debug {
				fmt.Println("\t-> Segment timeline found")
			}

			segIndex := 0 // 0 by default, TODO: check specs to see if index 0 or 1

			for _, tlSeg := range segTemplate.SegmentTimeline.Segments {
				// start time is the index of the first segment
				if tlSeg.StartTime != nil {
					segIndex = uint64PtrToI(tlSeg.StartTime)
				}

				// handle repeat count if set
				repeat := intPtrToI(tlSeg.RepeatCount)
				if repeat == 0 {
					repeat = 1
				}
				for i := 0; i < repeat; i++ {
					url := strings.Replace(template, "$Time$", strconv.Itoa(segIndex+i), -1)
					urls = append(urls, url)
				}
				segIndex += repeat
			}

		}

	} else if strings.Contains(template, "$Number$") {

	} else {
		urls = append(urls, template)
	}

	return
}

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

func intPtrToI(d *int) int {
	if d == nil {
		return 0
	}
	return int(*d)
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
