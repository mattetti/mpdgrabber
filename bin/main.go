package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/mattetti/mpdgrabber"
)

const (
	// baseURL simple audio only example
	audioOnlyManifest     = "https://dash.akamaized.net/dash264/TestCases/3a/fraunhofer/heaac_stereo_without_video/ElephantsDream/elephants_dream_audio_only_heaac_stereo_sidx.mpd"
	audioAndVideoManifest = "https://storage.googleapis.com/shaka-demo-assets/angel-one/dash.mpd"
	templatedManifest     = "https://dash.akamaized.net/dash264/TestCasesIOP33/adapatationSetSwitching/5/manifest.mpd"
)

var (
	debugFlag      = flag.Bool("debug", true, "Set debug mode")
	URLFlag        = flag.String("url", templatedManifest, "URL of the mpeg-dash manifest to backup.")
	outputFileName = flag.String("output", "downloaded_video", "The name of the output file without the extension.")
	audioOnlyFlag  = flag.Bool("audio-only", false, "Download only the audio tracks.")
	videoOnlyFlag  = flag.Bool("video-only", false, "Download only the video tracks.")
	textOnlyFlag   = flag.Bool("text-only", false, "Download only the text tracks.")
	langsOnlyFlag  = flag.String("langs-only", "", "Download only the text tracks for the specified languages (comma separated).")
)

func main() {

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s \n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()
	mpdArgCheck()

	mpdgrabber.Debug = *debugFlag
	if mpdgrabber.Debug {
		mpdgrabber.Logger.Println("Downloading", *URLFlag)
		fmt.Println()
	}

	if *audioOnlyFlag {
		mpdgrabber.AudioDownloadEnabled = true
		mpdgrabber.VideoDownloadEnabled = false
		mpdgrabber.TextDownloadEnabled = false
	} else if *videoOnlyFlag {
		mpdgrabber.AudioDownloadEnabled = false
		mpdgrabber.VideoDownloadEnabled = true
		mpdgrabber.TextDownloadEnabled = false
	} else if *textOnlyFlag {
		mpdgrabber.AudioDownloadEnabled = false
		mpdgrabber.VideoDownloadEnabled = false
		mpdgrabber.TextDownloadEnabled = true
	}

	if *langsOnlyFlag != "" {
		mpdgrabber.LangFilter = strings.Split(*langsOnlyFlag, ",")
		for i, lang := range mpdgrabber.LangFilter {
			mpdgrabber.LangFilter[i] = strings.TrimSpace(lang)
		}
	}

	wg := &sync.WaitGroup{}
	stopChan := make(chan bool)
	mpdgrabber.LaunchWorkers(wg, stopChan)

	pathToUse, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	if err := mpdgrabber.DownloadFromMPDFile(*URLFlag, pathToUse, *outputFileName); err != nil {
		mpdgrabber.Logger.Printf("Failed to download the mpd file: %s", err)
		os.Exit(1)
	}

	mpdgrabber.Close()
	fmt.Println("Waiting for workers to finish!")
	wg.Wait()
}

func mpdArgCheck() {
	if *URLFlag == "" {
		if len(os.Args) < 2 {
			fmt.Fprint(os.Stderr, "You have to pass the URL of a mpd manifest.\n")
			os.Exit(2)
			return
		} else {
			// backup in case the user didn't use flags but passed params instead
			*URLFlag = os.Args[1]
			if *outputFileName == "download" && len(os.Args) > 2 {
				*outputFileName = os.Args[2]
			}
		}
	}
}
