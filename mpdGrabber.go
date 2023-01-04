package mpdgrabber

import (
	"fmt"
	"io"
	"log"
	"mime"
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

func templatedSegments(baseURL *url.URL, representation *mpd.Representation) (segmentUrls []string) {
	if representation == nil {
		if Debug {
			fmt.Println("no representation to look for templated segments")
		}
		return
	}
	// I don't think you have 2 templates but the mpd format is bonkers... so maybe?
	var template *mpd.SegmentTemplate
	if representation.AdaptationSet != nil && representation.AdaptationSet.SegmentTemplate != nil {
		template = representation.AdaptationSet.SegmentTemplate
		if Debug {
			fmt.Println("->using AdaptationSet.SegmentTemplate")
		}
	} else {
		template = representation.SegmentTemplate
		if Debug {
			fmt.Println("->using Representation.SegmentTemplate")
		}
	}
	if template == nil {
		if Debug {
			fmt.Println("no SegmentTemplate found")
		}
		return
	}

	segmentUrls = templateSubstitution(template.Initialization, representation)
	if Debug && len(segmentUrls) > 0 {
		fmt.Printf("templated Initialization url: %s\n", segmentUrls[0])
	}
	segmentUrls = append(segmentUrls, templateSubstitution(template.Media, representation)...)
	if Debug {
		fmt.Printf("Found templated segments %d\n", len(segmentUrls))
	}
	for i, segmentURL := range segmentUrls {
		segmentUrls[i] = absBaseURL(baseURL, []string{segmentURL}).String()
	}

	return segmentUrls
}

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

		// segment timeline
		if segTemplate.SegmentTimeline != nil {
			if Debug {
				fmt.Println("\t-> Segment timeline found")
			}

			/* example
					<S t="0" d="96256" r="2" />
					<S d="95232" />
					<S d="96256" r="2" />
			    <S d="95232" />
			*/

			currentT := 0
			duration := 0

			for _, tlSeg := range segTemplate.SegmentTimeline.Segments {
				if tlSeg.StartTime != nil {
					currentT = uint64PtrToI(tlSeg.StartTime)
				}
				duration = int(tlSeg.Duration)
				url := strings.Replace(template, "$Time$", strconv.Itoa(currentT), -1)
				urls = append(urls, url)
				currentT += duration

				// repeat count if present
				if tlSeg.RepeatCount != nil {
					repeat := intPtrToI(tlSeg.RepeatCount)
					for i := 0; i < repeat; i++ {
						url := strings.Replace(template, "$Time$", strconv.Itoa(currentT), -1)
						urls = append(urls, url)
						currentT += duration
					}
				}
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

	// Create the file
	out, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		out.Close()
		os.Remove(path)
		return nil, err
	}

	return out, nil
}

func guessedExtension(r *mpd.Representation) string {
	if r == nil {
		return ""
	}
	// mimetype check first
	mimeType := strPtrtoS(r.MimeType)
	if mimeType == UnknownString && r.AdaptationSet != nil && r.AdaptationSet.MimeType != nil {
		mimeType = strPtrtoS(r.AdaptationSet.MimeType)
	}
	if mimeType != UnknownString {
		fmt.Println("checking mimetype", mimeType)
		ext, err := mime.ExtensionsByType(mimeType)
		if err == nil && len(ext) > 0 {
			return ext[0]
		}
	}

	// codec check next
	codec := strPtrtoS(r.Codecs)
	if codec == "" && r.AdaptationSet != nil && r.AdaptationSet.Codecs != nil {
		codec = strPtrtoS(r.AdaptationSet.Codecs)
	}
	if codec != "" {
		if strings.Contains(codec, "avc") {
			return ".mp4"
		}
		if strings.Contains(codec, "mp4a") {
			return ".mp4"
		}
		if strings.Contains(codec, "mp3") {
			return ".mp3"
		}
		if strings.Contains(codec, "vorbis") {
			return ".ogg"
		}
		if strings.Contains(codec, "opus") {
			return ".opus"
		}
		if strings.Contains(codec, "vp9") {
			return ".webm"
		}
		if strings.Contains(codec, "vp8") {
			return ".webm"
		}
	}

	return ""
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
