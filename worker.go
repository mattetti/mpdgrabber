package mpdgrabber

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mattetti/go-dash/mpd"
)

var (
	TotalWorkers         = 6
	TmpFolder, _         = ioutil.TempDir("", "mpdgrabber")
	filenameCleaner      = strings.NewReplacer("/", "-", "!", "", "?", "", ",", "")
	AudioDownloadEnabled = true
	VideoDownloadEnabled = true
	TextDownloadEnabled  = true
	// inclusive filter, all languages are downloaded by default
	LangFilter = []string{}

	DlChan  = make(chan *WJob)
	segChan = make(chan *WJob)
)

type WJobType int

const (
	_ WJobType = iota
	ManifestDL
	VideoSegmentDL
	VideoPartialSegmentDL
	AudioSegmentDL
	AudioPartialSegmentDL
	TextSegmentDL
	TextPartialSegmentDL
	TextDL
)

func (w WJobType) String() string {
	switch w {
	case ManifestDL:
		return "ManifestDL"
	case VideoSegmentDL:
		return "VideoSegmentDL"
	case VideoPartialSegmentDL:
		return "VideoPartialSegmentDL"
	case AudioSegmentDL:
		return "AudioSegmentDL"
	case AudioPartialSegmentDL:
		return "AudioPartialSegmentDL"
	case TextSegmentDL:
		return "TextSegmentDL"
	case TextPartialSegmentDL:
		return "TextPartialSegmentDL"
	case TextDL:
		return "TextDL"
	default:
		return "Unknown"
	}
}

// LaunchWorkers starts download workers
func LaunchWorkers(wg *sync.WaitGroup, stop <-chan bool) {
	DlChan = make(chan *WJob)
	segChan = make(chan *WJob)

	// the main worker downloads one full manifest at a time but
	// segments are downloaded concurrently
	mainW := &Worker{id: 0, wg: wg, main: true}
	go mainW.Work()

	for i := 1; i < TotalWorkers+1; i++ {
		w := &Worker{id: i, wg: wg, client: &http.Client{}}
		go w.Work()
	}
}

type WJob struct {
	Type          WJobType
	SkipConverter bool
	SubsOnly      bool
	AudioOnly     bool
	URL           string
	AbsolutePath  string
	DestPath      string
	Filename      string
	Pos           int
	Total         int
	Lang          string
	// Err gets populated if something goes wrong while processing the job
	Err error
	wg  *sync.WaitGroup
}

type Worker struct {
	id     int
	wg     *sync.WaitGroup
	main   bool
	client *http.Client
}

func (w *Worker) Work() {
	if Debug {
		Logger.Printf("worker %d is ready for action\n", w.id)
	}
	if w.main {
		w.wg.Add(1)
		for msg := range DlChan {
			w.dispatch(msg)
		}
		close(segChan)
		w.wg.Done()
	} else {
		for msg := range segChan {
			w.dispatch(msg)
		}
	}

	if Debug {
		fmt.Printf("-> Worker %d is out\n", w.id)
	}
}

func (w *Worker) dispatch(job *WJob) {
	switch job.Type {
	case ManifestDL:
		w.downloadManifest(job)
	case VideoSegmentDL, VideoPartialSegmentDL, AudioSegmentDL, AudioPartialSegmentDL:
		job.wg.Add(1)
		if Debug {
			fmt.Printf("-> [W%d] start downloading %s segment: [%d/%d]\n", w.id, job.Type, job.Pos, job.Total)
		}
		w.downloadSegment(job)
	case TextSegmentDL, TextPartialSegmentDL:
		job.wg.Add(1)
		if Debug {
			fmt.Printf("-> [W%d] start downloading %s segment: [%d/%d]\n", w.id, job.Type, job.Pos, job.Total)
		}
		w.downloadSegment(job)
	default:
		Logger.Printf("format: %s not supported by workers\n", job.Type)
		return
	}

}

func DownloadFromMPDFile(manifestURL, pathToUse, outFilename string) error {
	wg := &sync.WaitGroup{}
	job := &WJob{
		Type:     ManifestDL,
		URL:      manifestURL,
		DestPath: pathToUse,
		Filename: outFilename,
		wg:       wg,
	}
	wg.Add(1)
	DlChan <- job
	wg.Wait()

	return job.Err
}

func (w *Worker) downloadManifest(job *WJob) {
	// if Debug {
	Logger.Println("Downloading manifest file:", job.URL)
	// }
	manifestPath := filepath.Join(TmpFolder, "manifest.mpd")
	mpdF, err := downloadFile(job.URL, manifestPath)
	if err != nil {
		Logger.Println("failed to download the manifest file")
		Logger.Println(err)
		job.Err = err
		return
	}

	defer func() {
		mpdF.Close()
		os.Remove(manifestPath)
		if job.wg != nil {
			job.wg.Done()
			if Debug {
				fmt.Println("-> done with the download manifest job")
			}
		}
	}()

	if Debug {
		fmt.Println("-> Parsing the manifest file", manifestPath)
	}

	// rewind the file
	mpdF.Seek(0, io.SeekStart)

	// parse the manifest
	mpdData, err := mpd.Read(mpdF)
	if err != nil {
		job.Err = fmt.Errorf("Failed to read the mpd file - %s\n", err)
		return
	}

	if mpdData.Type != nil && (*mpdData.Type == "dynamic") {
		job.Err = fmt.Errorf("dynamic mpd not supported")
		return
	}
	// if Debug {
	Logger.Println("MPD file parsed")
	// }

	audioTracks := []*OutputTrack{}
	videoTracks := []*OutputTrack{}
	textTracks := []*OutputTrack{}

	maniURL, _ := url.Parse(job.URL)
	var baseURL *url.URL

	if len(mpdData.BaseURL) == 0 {
		baseURL = maniURL
	} else {
		baseURL = absBaseURL(maniURL, mpdData.BaseURL)
		if Debug {
			fmt.Println("-> Base URL", baseURL.String())
		}
	}

	tmpBaseURL := baseURL
	for _, period := range mpdData.Periods {
		if Debug {
			fmt.Printf("-> Period ID: %s, duration: %s\n", period.ID, time.Duration(period.Duration).String())
		}

		if len(period.BaseURL) > 0 {
			tmpBaseURL = absBaseURL(tmpBaseURL, period.BaseURL)
			if Debug {
				fmt.Printf("-> Base URL:%s", tmpBaseURL.String())
				fmt.Println()
			}
		}

		for _, adaptationSet := range period.AdaptationSets {
			contentType := extractContentType(adaptationSet.ContentType, adaptationSet.MimeType)
			if contentType == UnknownString {
				availableTypes := representationTypes(adaptationSet.Representations)
				if len(availableTypes) == 1 {
					contentType = availableTypes[0]
				}
			}
			setBaseURL := absBaseURL(tmpBaseURL, adaptationSet.BaseURL)
			// populate the adaptation set in the representation
			for i := range adaptationSet.Representations {
				adaptationSet.Representations[i].AdaptationSet = adaptationSet
			}

			if shouldSkipLang(strPtrtoS(adaptationSet.Lang)) {
				if Debug {
					fmt.Printf("-> Skipping adaptation %s, [%s] because Lang: %s {allowed: %s}\n",
						strPtrtoS(adaptationSet.ID),
						contentType,
						strPtrtoS(adaptationSet.Lang),
						strings.Join(LangFilter, ","),
					)
				}
				continue
			}

			if shouldSkipContentType(contentType) {
				if Debug {
					fmt.Printf("-> Skipping adaptation %s, [%s] because content type filtering {allowed: %s}\n",
						strPtrtoS(adaptationSet.ID),
						contentType,
						strings.Join(allowedContentTypes(), ","),
					)
				}
				continue
			}

			if Debug {
				debugPrintAdaptationSet(setBaseURL, contentType, adaptationSet)
			}

			r := highestRepresentation(contentType, adaptationSet.Representations)
			if Debug {
				fmt.Println("\tBest representation:")
				debugPrintRepresentation(setBaseURL, contentType, r)
				fmt.Println()
			}

			if r == nil {
				Logger.Println("no representation found for adaptation set:", strPtrtoS(adaptationSet.ID))
				continue
			}

			rBaseURL := absBaseURL(setBaseURL, r.BaseURL)

			switch contentType {
			case "video":
				Logger.Printf("Downloading Video Track: %s", strPtrtoS(r.ID))
				downloadVideoRepresentation(job, rBaseURL, r, &videoTracks)
			case "audio":
				Logger.Printf("Downloading Audio Track: %s", strPtrtoS(r.ID))
				downloadAudioRepresentation(job, rBaseURL, r, &audioTracks)
			case "text":
				Logger.Printf("Downloading Text Track: %s", strPtrtoS(r.ID))
				downloadTextRepresentation(job, rBaseURL, r, &textTracks)
			default:
				Logger.Println("unknown content type:", contentType)
			}

		}
	}

	outputPath := filepath.Join(job.DestPath, job.Filename) + ".mkv"
	err = Mux(outputPath, audioTracks, videoTracks, textTracks)
	if err != nil {
		Logger.Println("Failed to mux audio tracks:", err)
		os.Exit(1)
	}
	Logger.Printf("Created %s\n", outputPath)

}

func downloadVideoRepresentation(job *WJob, baseURL *url.URL, r *mpd.Representation, videoTracks *[]*OutputTrack) {
	downloadRepresentation(job, baseURL, r, ContentTypeVideo, videoTracks)
}

func downloadAudioRepresentation(job *WJob, baseURL *url.URL, r *mpd.Representation, audioTracks *[]*OutputTrack) {
	downloadRepresentation(job, baseURL, r, ContentTypeAudio, audioTracks)
}

func downloadTextRepresentation(job *WJob, baseURL *url.URL, r *mpd.Representation, textTracks *[]*OutputTrack) {
	downloadRepresentation(job, baseURL, r, ContentTypeText, textTracks)
}

func downloadRepresentation(job *WJob, baseURL *url.URL, r *mpd.Representation, cType ContentType, outputTracks *[]*OutputTrack) {

	var outPath string
	if isSegmentBase(r) {
		// 1 big file for the entire representation, no need to assemble segments
		outFilename := filepath.Base(baseURL.Path)
		outPath := filepath.Join(job.DestPath, outFilename)
		var jobType WJobType
		switch cType {
		case ContentTypeAudio:
			jobType = AudioSegmentDL
		case ContentTypeVideo:
			jobType = VideoSegmentDL
		case ContentTypeText:
			jobType = TextSegmentDL
		default:
			Logger.Println("unknown content type:", cType)
			return
		}

		Logger.Printf("(1 %s segment)\n", cType)
		job := &WJob{
			Type:         jobType,
			URL:          baseURL.String(),
			AbsolutePath: outPath,
			Filename:     outFilename,
			wg:           job.wg,
			Total:        1,
		}
		segChan <- job

	} else {
		suffix := "_seg_"
		// must be set by the segment list or segment template code sections
		nbrSegments := 0
		tmpFilenamePattern := ""
		var jobType WJobType
		switch cType {
		case ContentTypeAudio:
			jobType = AudioPartialSegmentDL
		case ContentTypeVideo:
			jobType = VideoPartialSegmentDL
		case ContentTypeText:
			jobType = TextPartialSegmentDL
		default:
			Logger.Println("unknown content type:", cType)
			return
		}

		// Segment list
		if r.SegmentList != nil && r.SegmentList.SegmentURLs != nil && len(r.SegmentList.SegmentURLs) > 0 {
			// raw segment list

			// we use a dedicated to wait for the entire segment list to be downloaded
			segWG := &sync.WaitGroup{}
			nbrSegments = len(r.SegmentList.SegmentURLs)
			tmpFilenamePattern = filepath.Base(strPtrtoS(r.SegmentList.SegmentURLs[0].Media)) + suffix
			var outFilename string
			var path string

			Logger.Printf("(%d %s segments)\n", nbrSegments, cType)
			for i, segURL := range r.SegmentList.SegmentURLs {
				outFilename = tmpFilenamePattern + strconv.Itoa(i)
				path = filepath.Join(job.DestPath, outFilename)

				job := &WJob{
					Type:         jobType,
					Pos:          i,
					Total:        nbrSegments,
					URL:          strPtrtoS(segURL.Media),
					AbsolutePath: path,
					Filename:     outFilename,
					wg:           segWG,
				}
				segChan <- job
			}
			segWG.Wait()

		} else if isTemplated(r) {
			// templated segment list
			segURLs := templatedSegments(baseURL, r)
			nbrSegments = len(segURLs)
			if len(segURLs) > 0 {
				tmpFilenamePattern = filepath.Base(segURLs[0]) + suffix
				segWG := &sync.WaitGroup{}
				Logger.Printf("(%d %s segments)\n", nbrSegments, cType)

				for i, segurl := range segURLs {
					outFilename := tmpFilenamePattern + strconv.Itoa(i)
					path := filepath.Join(job.DestPath, outFilename)

					job := &WJob{
						Type:         jobType,
						Pos:          i,
						Total:        nbrSegments,
						URL:          segurl,
						AbsolutePath: path,
						Filename:     outFilename,
						wg:           segWG,
					}
					segChan <- job

				}
				segWG.Wait()
			}

			if nbrSegments > 0 {
				outFilename := tmpFilenamePattern[:len(tmpFilenamePattern)-5]
				// the track to reassemble
				tempPathPattern := filepath.Join(job.DestPath, outFilename)
				outPath = tempPathPattern + guessedExtension(r)
				Logger.Printf("Reconstructing sub %s file: %s\n", cType, filepath.Base(outPath))
				err := reassembleFile(tempPathPattern, suffix, outPath, len(segURLs), cType)
				if err != nil {
					job.Err = fmt.Errorf("error reassembling file: %s - %v", outPath, err)
					Logger.Println(job.Err)
					return
				}
			}

		} else {
			Logger.Printf("track is not in a supported format, AS ID: %s, Rep ID: %s", strPtrtoS(r.AdaptationSet.ID), strPtrtoS(r.ID))
			return
		}

		if outPath != "" {
			at := &OutputTrack{
				RepresentationID: strPtrtoS(r.ID),
				BaseURL:          baseURL.String(),
				Language:         strPtrtoS(r.AdaptationSet.Lang),
				AbsolutePath:     outPath,
				Codec:            strPtrtoS(r.Codecs),
				SampleRate:       int64PtrToI(r.AudioSamplingRate),
				MediaType:        ContentTypeAudio,
			}
			*outputTracks = append(*outputTracks, at)
		}
	}
}

func (w *Worker) downloadSegment(job *WJob) {
	// if Debug {
	// 	fmt.Println("-> Downloading segment:", job.URL, "to", job.AbsolutePath)
	// }
	defer func() {
		if job.wg != nil {
			job.wg.Done()
		}
	}()

	if fileExists(job.AbsolutePath) {
		Logger.Println("File already present", job.AbsolutePath)
	}

	audioF, err := downloadFile(job.URL, job.AbsolutePath)
	if err != nil {
		Logger.Printf("Failed to download the %s segment file\n", job.Type)
		Logger.Println(err)
	}
	if Debug {
		fmt.Printf("-> [W%d] done downloading %s segment [%d/%d]\n", w.id, job.Type, job.Pos, job.Total)
	}
	audioF.Close()
	job.Err = err
}

func representationTypes(representations []*mpd.Representation) []string {
	typesMap := map[string]bool{}
	for _, r := range representations {
		typesMap[extractContentType(nil, r.MimeType)] = true
	}
	t := make([]string, 0, len(typesMap))
	for k := range typesMap {
		t = append(t, k)
	}
	return t
}

func highestRepresentation(contentType string, representations []*mpd.Representation) *mpd.Representation {
	var highestBandwidth int64
	var highestWidth int64
	var highestRep *mpd.Representation

	if contentType == UnknownString {
		availableTypes := representationTypes(representations)
		if len(availableTypes) == 1 {
			contentType = availableTypes[0]
		} else {
			Logger.Printf("multiple content types found: %s", strings.Join(availableTypes, ", "))
			return nil
		}
	}

	// Video
	if strings.ToLower(contentType) == "video" {
		for _, r := range representations {
			// try the width first (since the bandwidth is codec dependent)
			if r.Width != nil {
				if *r.Width > highestWidth {
					highestWidth = *r.Width
					highestRep = r
				}
			} else if r.Bandwidth != nil {
				if *r.Bandwidth > highestBandwidth {
					highestBandwidth = *r.Bandwidth
					highestRep = r
				}
			}
		}
	} else

	// Audio
	if strings.ToLower(contentType) == "audio" {
		for _, r := range representations {
			// try the bandwidth first
			if r.Bandwidth != nil && *r.Bandwidth > highestBandwidth {
				highestBandwidth = *r.Bandwidth
				highestRep = r
			}
			// TODO: consider filtering/sorting codecs since bigger isn't always better
		}
	} else

	// Text
	if strings.ToLower(contentType) == "text" {
		for _, r := range representations {
			// try the bandwidth first
			if r.Bandwidth != nil && *r.Bandwidth > highestBandwidth {
				highestBandwidth = *r.Bandwidth
				highestRep = r
			}
		}
	}

	if highestRep == nil {
		Logger.Println("No highest representation found for content type", contentType, "picking the last one")
		// pick the last one, hoping it's the highest quality
		highestRep = representations[len(representations)-1]
	}

	return highestRep
}

func isSegmentBase(r *mpd.Representation) bool {

	/*
		  See https://bitmovin.com/dynamic-adaptive-streaming-http-mpeg-dash/#:~:text=4%3A%20Segment%20Referencing%20Schemes

			A representation should only contain one of the following options:
			* one or more SegmentList elements
			* one SegmentTemplate
			* one or more BaseURL elements, at most one SegmentBase element and no SegmentTemplate or SegmentList element.
	*/

	if r.SegmentBase != nil {
		return true
	}

	if r.SegmentList != nil {
		return false
	}

	if r.SegmentTemplate != nil {
		return false
	}

	// check the parent segmentTemplate
	if r.AdaptationSet != nil && r.AdaptationSet.SegmentTemplate != nil {
		return false
	}

	if len(r.BaseURL) == 0 {
		return false
	}

	return true
}

func isTemplated(r *mpd.Representation) bool {
	if r.SegmentTemplate != nil {
		return true
	}

	if r.AdaptationSet != nil && r.AdaptationSet.SegmentTemplate != nil {
		return true
	}

	return false
}

func shouldSkipLang(lang string) bool {
	if lang == "" || lang == "und" || lang == UnknownString {
		return false
	}

	if len(LangFilter) == 0 {
		return false
	}

	for _, l := range LangFilter {
		if l == lang {
			return false
		}
	}

	return true
}

func shouldSkipContentType(contentType string) bool {
	// filter content types based on download flags
	switch contentType {
	case "video":
		if !VideoDownloadEnabled {
			return true
		}
	case "audio":
		if !AudioDownloadEnabled {
			return true
		}
	case "text":
		if !TextDownloadEnabled {
			return true
		}
	}
	return false
}

func allowedContentTypes() []string {
	var allowed []string
	if VideoDownloadEnabled {
		allowed = append(allowed, "video")
	}
	if AudioDownloadEnabled {
		allowed = append(allowed, "audio")
	}
	if TextDownloadEnabled {
		allowed = append(allowed, "text")
	}
	return allowed
}

// Close closes the open channels to stop the workers cleanly
func Close() {
	close(DlChan)
}

type OutputTrack struct {
	RepresentationID string
	BaseURL          string // optional
	Language         string
	Codec            string
	SampleRate       int
	AbsolutePath     string
	MediaType        ContentType
}
