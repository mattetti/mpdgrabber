package mpdgrabber

import (
	"fmt"
	"log"
	"math"
	"net/url"
	"strings"

	"github.com/mattetti/go-dash/mpd"
)

func debugPrintAdaptationSet(baseURL *url.URL, contentType string, as *mpd.AdaptationSet) {
	fmt.Printf(">> AdaptationSet ID: %s ContentType: %s", strPtrtoS(as.ID), contentType)
	if as.MimeType != nil {
		fmt.Printf(" MimeType: %s", strPtrtoS(as.MimeType))
	}
	if as.Codecs != nil {
		fmt.Printf(" Codecs: %s", strPtrtoS(as.Codecs))
	}
	fmt.Println()
	if baseURL != nil {
		fmt.Println("  BaseURL:", baseURL.String())
	}
	// print Group
	if as.Group != nil {
		fmt.Printf("  Group ID: %s\n", strPtrtoS(as.Group))
	}
	if as.SegmentAlignment != nil {
		fmt.Printf("  SegmentAlignment: %t\n", boolPtrToB(as.SegmentAlignment))
	}
	if as.MaxWidth != nil {
		fmt.Printf("  MaxWidth: %s\n", strPtrtoS(as.MaxWidth))
	}
	if as.MaxHeight != nil {
		fmt.Printf("  MaxHeight: %s\n", strPtrtoS(as.MaxHeight))
	}
	if as.PAR != nil {
		fmt.Printf("  Par: %s\n", strPtrtoS(as.PAR))
	}
	if as.Lang != nil {
		fmt.Printf("  Lang: %s\n", strPtrtoS(as.Lang))
	}
	for _, role := range as.Roles {
		fmt.Printf("  Role: %s\n", strPtrtoS(role.Value))
	}
	fmt.Printf("  # of representations: %d\n", len(as.Representations))
}

func debugPrintRepresentation(baseURL *url.URL, contentType string, r *mpd.Representation) {
	fmt.Printf("\tRepresentation ID: %s\n", strPtrtoS(r.ID))
	if r.MimeType != nil {
		fmt.Printf("\tMimeType: %s\n", strPtrtoS(r.MimeType))
	}
	rURL := absBaseURL(baseURL, nil)
	if len(r.BaseURL) > 0 {
		rURL = absBaseURL(rURL, r.BaseURL)
		fmt.Println("\tBaseURL:", rURL.String())
		if r.SegmentBase != nil {
			fmt.Printf("\tSegmentBase Timescale: %d\n", uint32PtrToI(r.SegmentBase.Timescale))
			// the Random Access Points (RAP) and other initialization information is contained in the index range.
			fmt.Printf("\tSegmentBase Index Range: %s\n", strPtrtoS(r.SegmentBase.IndexRange))
			if r.SegmentBase.Initialization != nil {
				if r.SegmentBase.Initialization.SourceURL != nil {
					fmt.Printf("\tSegmentBase Initialization src url: %s\n", strPtrtoS(r.SegmentBase.Initialization.SourceURL))
				}
				fmt.Printf("\tSegmentBase Initialization range: %s\n", strPtrtoS(r.SegmentBase.Initialization.Range))
			}
		}
	}

	if r.AdaptationSet != nil && r.AdaptationSet.SegmentTemplate != nil {
		if Debug {
			fmt.Println("\t-> AdaptationSet SegmentTemplate")
		}
		segmentUrls := templateSubstitution(r.AdaptationSet.SegmentTemplate.Initialization, r)
		segmentUrls = append(segmentUrls, templateSubstitution(r.AdaptationSet.SegmentTemplate.Media, r)...)
		fmt.Println("\t\t# of Segment URLs:", len(segmentUrls))
		for i, segmentURL := range segmentUrls {
			segmentUrls[i] = absBaseURL(rURL, []string{segmentURL}).String()
			fmt.Println("\t\tSegment URL:", segmentUrls[i])
		}
	}

	if r.SegmentTemplate != nil {
		// <SegmentTemplate timescale="48000" media="2second/tears_of_steel_1080p_audio_32k_dash_track1_$Number$.mp4" startNumber="1" duration="95232" initialization="2second/tears_of_steel_1080p_audio_32k_dash_track1_init.mp4"/>
		if r.SegmentTemplate.Timescale != nil {
			fmt.Printf("\tSegmentTemplate Timescale: %d\n", int64PtrToI(r.SegmentTemplate.Timescale))
		}
		if r.SegmentTemplate.Media != nil {
			if strings.Contains(strPtrtoS(r.SegmentTemplate.Media), "$Number$") {
				mediaStr := strPtrtoS(r.SegmentTemplate.Media)
				fmt.Printf("\tSegmentTemplate number Media: %s\n", mediaStr)
				duration := int64PtrToI(r.SegmentTemplate.Duration)
				start := int64PtrToI(r.SegmentTemplate.StartNumber)
				timescale := int64PtrToI(r.SegmentTemplate.Timescale)
				// calculate the number of segments
				// floor of duration / timescale
				numberOfSegments := int(math.Ceil(float64(duration) / float64(timescale)))
				// TODO: should be ~370 in the above example
				fmt.Println("\t\tNumber of segments:", numberOfSegments)
				numberOfSegments += start
				for i := start; i < numberOfSegments; i++ {
					// replace $Number$ with i in mediaStr
					mediaURLStr := strings.Replace(mediaStr, "$Number$", fmt.Sprintf("%d", i), -1)
					mediaURL := absBaseURL(rURL, []string{mediaURLStr})
					fmt.Printf("\t\tSegmentTemplate number Media [%d]: %s\n", i, mediaURL.String())
				}
				// ceil value of numbnerOfSegments

			} else {
				fmt.Printf("\tSegmentTemplate Media: %s\n", strPtrtoS(r.SegmentTemplate.Media))
			}
		}
		if r.SegmentTemplate.StartNumber != nil {
			fmt.Printf("\tSegmentTemplate StartNumber: %d\n", int64PtrToI(r.SegmentTemplate.StartNumber))
		}
		if r.SegmentTemplate.Duration != nil {
			fmt.Printf("\tSegmentTemplate Duration: %d\n", int64PtrToI(r.SegmentTemplate.Duration))
		}
		if r.SegmentTemplate.Timescale != nil {
			fmt.Printf("\tSegmentTemplate Timescale: %d\n", int64PtrToI(r.SegmentTemplate.Timescale))
		}
		if r.SegmentTemplate.Initialization != nil {
			fmt.Printf("\tSegmentTemplate Initialization: %s\n", strPtrtoS(r.SegmentTemplate.Initialization))
		}
		if r.SegmentTemplate.PresentationTimeOffset != nil {
			fmt.Printf("\tSegmentTemplate PresentationTimeOffset: %d\n", uint64PtrToI(r.SegmentTemplate.PresentationTimeOffset))
		}
		if r.SegmentTemplate.SegmentTimeline != nil {
			fmt.Printf("\tSegmentTemplate SegmentTimeline: %d\n", len(r.SegmentTemplate.SegmentTimeline.Segments))
		}

	}

	if contentType == UnknownString {
		contentType = extractContentType(nil, r.MimeType)
		fmt.Printf("\tContentType: %s\n", contentType)
	}

	switch contentType {
	case "video":
		fmt.Printf("\tBandwidth: %d, width: %d, height: %d, codecs: %s, scanType: %s\n", int64PtrToI(r.Bandwidth), int64PtrToI(r.Width), int64PtrToI(r.Height), strPtrtoS(r.Codecs), strPtrtoS(r.ScanType))
		fmt.Println()

	case "audio":
		fmt.Printf("\tBandwidth: %d, SR: %d", int64PtrToI(r.Bandwidth), int64PtrToI(r.AudioSamplingRate))
		if r.Codecs != nil {
			fmt.Printf(", Codecs: %s", strPtrtoS(r.Codecs))
		}
		if (r.AudioChannelConfiguration != nil) && (r.AudioChannelConfiguration.Value != nil) {
			fmt.Printf(", AudioChannelConfiguration: %s\n", strPtrtoS(r.AudioChannelConfiguration.Value))
		}
		fmt.Println()

	case "text":
		fmt.Printf("\t Codecs: %s\n", strPtrtoS(r.Codecs))
		fmt.Println()

	default:
		log.Printf("\tUnknown content type: %s\n", contentType)
	}
}
