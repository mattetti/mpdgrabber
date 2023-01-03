package mpdgrabber

import (
	"fmt"
	"log"
	"net/url"

	"github.com/zencoder/go-dash/mpd"
)

func debugPrintAdaptationSet(contentType string, as *mpd.AdaptationSet) {
	fmt.Printf(">> AdaptationSet ID: %s ContentType: %s", strPtrtoS(as.ID), contentType)
	if as.MimeType != nil {
		fmt.Printf(" MimeType: %s", strPtrtoS(as.MimeType))
	}
	if as.Codecs != nil {
		fmt.Printf(" Codecs: %s", strPtrtoS(as.Codecs))
	}
	fmt.Println()
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
	if r.BaseURL != nil {
		tmpBaseURL := absBaseURL(baseURL, *r.BaseURL)
		fmt.Println("\tBaseURL:", tmpBaseURL)

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
