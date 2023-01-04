package mpdgrabber

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
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

func Mux(outFilePath string, audioTracks []*OutputTrack) error {
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

func reassembleFile(tempPath string, suffix string, outPath string, nbrSegments int) error {

	// look for all files in path that start by the baseFilename and suffix
	// for each file, open it and write it to the output file
	files, err := filepath.Glob(tempPath + suffix + "*")
	if err != nil {
		return fmt.Errorf("failed to list files in %s - %w", tempPath, err)
	}
	if len(files) != nbrSegments {
		Logger.Printf("expected %d files, got %d\n", nbrSegments, len(files))
		Logger.Fatal("not enough files")
	}

	out, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed to create %s - %w", outPath, err)
	}
	defer out.Close()

	// sort the files using the suffix number
	sort.Slice(files, func(i, j int) bool {
		// find the last instance of suffix and extract the end of the the string
		a, _ := strconv.Atoi(files[i][strings.LastIndex(files[i], suffix)+len(suffix):])
		b, _ := strconv.Atoi(files[j][strings.LastIndex(files[j], suffix)+len(suffix):])
		return a < b
	})

	for _, fPath := range files {
		in, err := os.Open(fPath)
		if err != nil {
			return fmt.Errorf("failed to open %s - %w", fPath, err)
		}
		defer in.Close()
		_, err = io.Copy(out, in)
		if err != nil {
			return fmt.Errorf("failed to copy %s to %s - %w", fPath, outPath, err)
		}
		err = os.Remove(fPath)
		if err != nil {
			return fmt.Errorf("failed to remove %s - %w", fPath, err)
		}
	}

	return nil
}
