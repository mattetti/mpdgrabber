package mpdgrabber

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mattetti/go-dash/mpd"
)

var (
	TotalWorkers         = 4
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
	ListDL
	VideoDL
	AudioSegmentDL
	TextDL
)

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
	// case VideoDL:
	// 	w.downloadVideo(job)
	// case TextDL:
	// 	w.downloadText(job)
	case AudioSegmentDL:
		job.wg.Add(1)
		if Debug {
			fmt.Println("-> Worker", w.id, "downloading audio segment", job.URL, "to", job.AbsolutePath)
		}
		w.downloadAudioSegment(job)
	default:
		Logger.Printf("format: %v not supported by workers\n", job.Type)
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
	if Debug {
		fmt.Println("-> Downloading the manifest file", job.URL)
	}
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
	if Debug {
		fmt.Println("-> MPD file parsed")
		// FIXME: max segment duration is missing
	}

	audioTracks := []*AudioTrack{}
	// videoFiles := []string{}
	// textFiles := []string{}

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

			case "audio":
				if isSegmentBase(r) {
					audioFilename := filepath.Base(rBaseURL.Path)
					path := filepath.Join(job.DestPath, audioFilename)
					job := &WJob{
						Type:         AudioSegmentDL,
						URL:          rBaseURL.String(),
						AbsolutePath: path,
						Filename:     audioFilename,
						wg:           job.wg,
					}
					segChan <- job

					at := &AudioTrack{
						RepresentationID: strPtrtoS(r.ID),
						BaseURL:          rBaseURL.String(),
						Language:         strPtrtoS(adaptationSet.Lang),
						AbsolutePath:     path,
						Codec:            strPtrtoS(r.Codecs),
						SampleRate:       int64PtrToI(r.AudioSamplingRate),
					}
					audioTracks = append(audioTracks, at)
				} else {
					// TODO: support segment list and template
					Logger.Printf("audio is not segment base, AS ID: %s, Rep ID: %s", strPtrtoS(adaptationSet.ID), strPtrtoS(r.ID))
				}

			case "text":
			default:
				Logger.Println("unknown content type:", contentType)
			}

		}
	}

	outputPath := filepath.Join(job.DestPath, job.Filename) + ".mkv"
	err = Mux(outputPath, audioTracks)
	if err != nil {
		Logger.Println("Failed to mux audio tracks:", err)
		os.Exit(1)
	}

}

func (w *Worker) downloadAudioSegment(job *WJob) {
	if Debug {
		fmt.Println("-> Downloading audio segment:", job.URL, "to", job.AbsolutePath)
	}
	defer func() {
		if job.wg != nil {
			job.wg.Done()
		}
	}()

	audioF, err := downloadFile(job.URL, job.AbsolutePath)
	if err != nil {
		Logger.Println("Failed to download the audio segment file")
		Logger.Println(err)
	}
	if Debug {
		fmt.Println("-> done with the download audio segment job", job.AbsolutePath)
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

	return true
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

type AudioTrack struct {
	RepresentationID string
	BaseURL          string // optional
	Language         string
	Codec            string
	SampleRate       int
	AbsolutePath     string
}
