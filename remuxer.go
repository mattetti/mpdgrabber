package mpdgrabber

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func FfmpegPath() (string, error) {
	// Look for ffmpeg
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("where", "ffmpeg")
	} else {
		cmd = exec.Command("which", "ffmpeg")
	}
	buf, err := cmd.Output()
	return strings.Trim(strings.Trim(string(buf), "\r\n"), "\n"), err
}

func Mux(outFilePath string, audioTracks []*AudioTrack) error {
	ffmpegPath, err := FfmpegPath()
	if err != nil {
		Logger.Fatalf("ffmpeg wasn't found on your system, it is required to convert video files.\n" +
			"Temp file(s) left on your hardrive\n")
		os.Exit(1)
	}

	// -y overwrites without asking
	args := []string{"-y"}

	trackNbr := 0
	// add the audio files
	for _, track := range audioTracks {
		if fileExists(track.AbsolutePath) {
			args = append(args, "-i", track.AbsolutePath)
			trackNbr++
		}
	}

	if trackNbr == 0 {
		return fmt.Errorf("No tracks found, nothing to mux")
	}

	for i := 0; i < trackNbr; i++ {
		args = append(args, "-map", fmt.Sprintf("%d", i))
	}

	// add the rest of the args
	args = append(args,
		"-vcodec", "copy",
		"-acodec", "copy",
		// "-bsf:a", "aac_adtstoasc"
	)

	args = append(args, outFilePath)
	cmd := exec.Command(ffmpegPath, args...)

	// Pipe out the cmd output in debug mode
	if Debug {
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return err
		}
		go io.Copy(os.Stdout, stdout)
		go io.Copy(os.Stderr, stderr)
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		Logger.Printf("ffmpeg Error: %v\n", err)
		Logger.Println("args", cmd.Args)
		return err
	}

	state := cmd.ProcessState
	if !state.Success() {
		Logger.Println("Error: something went wrong when trying to use ffmpeg")
	} else {
		for _, aFile := range audioTracks {
			err = os.Remove(aFile.AbsolutePath)
			if err != nil {
				Logger.Println("Couldn't delete temp file: " + aFile.AbsolutePath + "\n Please delete manually.\n")
			}
		}
	}

	return err
}
