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

	"github.com/zencoder/go-dash/mpd"
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

	Logger.Printf("worker %d is out", w.id)
}

func (w *Worker) dispatch(job *WJob) {
	switch job.Type {
	// case ListDL:
	// 	// w. TODO (job)
	// case VideoDL:
	// 	w.downloadVideo(job)
	// case TextDL:
	// 	w.downloadText(job)
	// case AudioDL:
	// 	w.downloadAudio(job)
	default:
		Logger.Printf("format: %v not supported by workers\n", job.Type)
		return
	}

}

func DownloadFromMPDFile(manifestURL, destPath string) error {

	manifestPath := filepath.Join(TmpFolder, "manifest.mpd")
	mpdF, err := downloadFile(manifestURL, manifestPath)
	if err != nil {
		Logger.Println("failed to download the manifest file")
		Logger.Println(err)
		return err
	}

	defer func() {
		mpdF.Close()
		os.Remove(manifestPath)
	}()

	// rewind the file
	mpdF.Seek(0, io.SeekStart)

	// parse the manifest
	mpdData, err := mpd.Read(mpdF)
	if err != nil {
		return fmt.Errorf("Failed to read the mpd file - %s\n", err)
	}

	if mpdData.Type != nil && (*mpdData.Type == "dynamic") {
		return fmt.Errorf("dynamic mpd not supported")
	}

	maniURL, _ := url.Parse(manifestURL)

	var baseURL *url.URL
	if mpdData.BaseURL == "" {
		baseURL = maniURL
	} else {
		baseURL = absBaseURL(maniURL, mpdData.BaseURL)
		fmt.Println("Base URL", baseURL.String())
	}

	tmpBaseURL := baseURL
	for _, period := range mpdData.Periods {
		if Debug {
			fmt.Printf("Period ID: %s, duration: %s\n", period.ID, time.Duration(period.Duration).String())
		}

		if period.BaseURL != "" {
			tmpBaseURL = absBaseURL(tmpBaseURL, period.BaseURL)
			if Debug {
				fmt.Printf(" Base URL:%s", tmpBaseURL.String())
				fmt.Println()
			}
		}

		for _, adaptationSet := range period.AdaptationSets {
			contentType := strPtrtoS(adaptationSet.ContentType)

			if shouldSkipLang(strPtrtoS(adaptationSet.Lang)) {
				continue
			}

			// filter content types based on download flags
			switch contentType {
			case "video":
				if !VideoDownloadEnabled {
					continue
				}
			case "audio":
				if !AudioDownloadEnabled {
					continue
				}
			case "text":
				if !TextDownloadEnabled {
					continue
				}
			}

			if Debug {
				debugPrintAdaptationSet(adaptationSet)
			}

			r := highestRepresentation(contentType, adaptationSet.Representations)
			if Debug {
				fmt.Println("\tBest representation:")
				debugPrintRepresentation(tmpBaseURL, contentType, r)
				fmt.Println()
			}

			if r == nil {
				Logger.Println("no representation found for adaptation set:", strPtrtoS(adaptationSet.ID))
				continue
			}

			switch contentType {
			case "video":

			case "audio":
				if isSegmentBase(r) {
					job := &WJob{
						Type:     AudioSegmentDL,
						URL:      tmpBaseURL.String(),
						DestPath: "",
						Filename: "",
					}
					DlChan <- job
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

	return nil

}

func highestRepresentation(contentType string, representations []*mpd.Representation) *mpd.Representation {
	var highestBandwidth int64
	var highestWidth int64
	var highestRep *mpd.Representation

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

// Close closes the open channels to stop the workers cleanly
func Close() {
	close(DlChan)
}
