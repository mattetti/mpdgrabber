package mpdgrabber

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
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
	TotalWorkers    = 4
	TmpFolder, _    = ioutil.TempDir("", "mpdgrabber")
	filenameCleaner = strings.NewReplacer("/", "-", "!", "", "?", "", ",", "")

	DlChan  = make(chan *WJob)
	segChan = make(chan *WJob)
)

type WJobType int

const (
	_ WJobType = iota
	ManifestDL
	ListDL
	VideoDL
	AudioDL
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
		Logger.Println("format not supported")
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
		fmt.Printf("Period ID: %s, duration: %s", period.ID, time.Duration(period.Duration).String())
		if period.BaseURL != "" {
			tmpBaseURL = absBaseURL(tmpBaseURL, period.BaseURL)
			fmt.Printf(" Base URL:%s", tmpBaseURL.String())
		}
		fmt.Println()

		for _, adaptationSet := range period.AdaptationSets {
			fmt.Printf(">> Adaptation set ID: %s/%s - %s, mimeType: %s, lang: %s, codecs: %s \n",
				strPtrtoS(adaptationSet.ID),
				strPtrtoS(adaptationSet.Group),
				strPtrtoS(adaptationSet.ContentType),
				strPtrtoS(adaptationSet.MimeType),
				strPtrtoS(adaptationSet.Lang),
				strPtrtoS(adaptationSet.Codecs),
			)

			// var codecs string
			for _, r := range adaptationSet.Representations {
				switch *adaptationSet.ContentType {
				case "video":
					fmt.Printf("\tRep ID: %s, Bandwidth: %d, width: %d, height: %d, codecs: %s, scanType: %s\n", strPtrtoS(r.ID), int64PtrToI(r.Bandwidth), int64PtrToI(r.Width), int64PtrToI(r.Height), strPtrtoS(r.Codecs), strPtrtoS(r.ScanType))

					//
					if r.BaseURL != nil {
						tmpBaseURL = absBaseURL(tmpBaseURL, *r.BaseURL)
						fmt.Println("\tRepresentation BaseURL:", tmpBaseURL)
					}
					fmt.Println()

				case "audio":
					fmt.Printf("\tRep ID: %s, Bandwidth: %d, SR: %d\n", strPtrtoS(r.ID), int64PtrToI(r.Bandwidth), int64PtrToI(r.AudioSamplingRate))

					if r.BaseURL != nil {
						tmpBaseURL = absBaseURL(tmpBaseURL, *r.BaseURL)
						fmt.Println("\tRepresentation BaseURL:", tmpBaseURL)
					}
					fmt.Println()
				case "text":
					fmt.Printf("\tRep ID: %s, Codecs: %s\n", strPtrtoS(r.ID), strPtrtoS(r.Codecs))
					if r.BaseURL != nil {
						tmpBaseURL = absBaseURL(tmpBaseURL, *r.BaseURL)
						fmt.Println("\tRepresentation BaseURL:", tmpBaseURL)
					}
					if r.SegmentBase != nil {
						fmt.Printf("\tSegmentBase Timescale: %d\n", uit32PtrToI(r.SegmentBase.Timescale))
						fmt.Printf("\tSegmentBase Index Range: %s\n", strPtrtoS(r.SegmentBase.IndexRange))
						if r.SegmentBase.Initialization != nil {
							if r.SegmentBase.Initialization.SourceURL != nil {
								fmt.Printf("\tSegmentBase Initialization src url: %s\n", strPtrtoS(r.SegmentBase.Initialization.SourceURL))
							}
							fmt.Printf("\tSegmentBase Initialization range: %s\n", strPtrtoS(r.SegmentBase.Initialization.Range))
						}
					}

					// job := &WJob{}
				default:
					log.Printf("\tUnknown content type: %s\n", *adaptationSet.ContentType)
				}
			}
			fmt.Println()
		}
	}

	// job := &WJob{
	// 	Type:     ListDL,
	// 	URL:      manifestURL,
	// 	SubsOnly: *subsOnly,
	// 	// SkipConverter: true,
	// 	DestPath: pathToUse,
	// 	Filename: filename}
	// DlChan <- job

	return nil

}

func absBaseURL(manifestBaseURL *url.URL, elBaseURL string) *url.URL {
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

// downloadFile downloads a file from a given url and saves it to a given path
// it returns the file and an error if something goes wrong
// It's the caller's responsibility to close the file.
func downloadFile(url string, path string) (*os.File, error) {
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
	req.Header.Add("Accept", "application/dash+xml,video/vnd.mpeg.dash.mpd")

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

// Close closes the open channels to stop the workers cleanly
func Close() {
	close(DlChan)
}
