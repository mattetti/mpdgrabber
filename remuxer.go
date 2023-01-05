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

	"github.com/mattetti/mpdgrabber/mp4"
	"github.com/mattetti/mpdgrabber/ttml"
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

func Mux(outFilePath string, audioTracks, videoTracks, textTracks []*OutputTrack) error {
	ffmpegPath, err := FfmpegPath()
	if err != nil {
		Logger.Fatalf("ffmpeg wasn't found on your system, it is required to convert video files.\n" +
			"Temp file(s) left on your hardrive\n")
		os.Exit(1)
	}

	// -y overwrites without asking
	args := []string{"-y"}
	mapArgs := []string{}

	trackNbr := 0
	// add the audio files
	for _, track := range audioTracks {
		if fileExists(track.AbsolutePath) {
			args = append(args, "-i", track.AbsolutePath)
			mapArgs = append(mapArgs, "-map", fmt.Sprintf("%d:a", trackNbr))
			trackNbr++
		}
	}

	// add the video files
	for _, track := range videoTracks {
		if fileExists(track.AbsolutePath) {
			args = append(args, "-i", track.AbsolutePath)
			mapArgs = append(mapArgs, "-map", fmt.Sprintf("%d:v", trackNbr))
			trackNbr++
		}
	}

	for _, track := range textTracks {
		if fileExists(track.AbsolutePath) {

			if filepath.Ext(track.AbsolutePath) == ".ttml" {
				fmt.Println("TTML subtitles found, but they aren't supported by FFMpeg")
				// convert the ttml to vtt
				vttPath := track.AbsolutePath + ".vtt"
				doc, err := ttml.Open(track.AbsolutePath)
				if err != nil {
					Logger.Printf("Error parsing %s as ttml: %v\n", track.AbsolutePath, err)
					continue
				}
				if err = doc.SaveAsVTT(vttPath); err != nil {
					Logger.Printf("Error converting %s from ttml to vtt: %v\n", track.AbsolutePath, err)
					continue
				}
				fmt.Println("We converted them to VTT subs and left the .ttml file for you")
				args = append(args, "-i", vttPath)
				mapArgs = append(mapArgs, "-map", fmt.Sprintf("%d:s", trackNbr))
				trackNbr++

				ttmlFilePath := strings.TrimSuffix(outFilePath, filepath.Ext(outFilePath)) + ".ttml"
				if err = os.Rename(track.AbsolutePath, ttmlFilePath); err != nil {
					Logger.Printf("Error renaming %s to %s: %v\n", track.AbsolutePath, ttmlFilePath, err)
				}

				continue

			}

			args = append(args, "-i", track.AbsolutePath)
			mapArgs = append(mapArgs, "-map", fmt.Sprintf("%d:s", trackNbr))
			trackNbr++

		}
	}

	if trackNbr == 0 {
		return fmt.Errorf("No tracks found, nothing to mux")
	}

	// map tags
	args = append(args, mapArgs...)

	// add the rest of the args
	args = append(args,
		"-vcodec", "copy",
		"-acodec", "copy",
		"-scodec", "copy",
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
		tracks := append(audioTracks, videoTracks...)
		// tracks = append(tracks, textTracks...)
		for _, aFile := range tracks {
			err = os.Remove(aFile.AbsolutePath)
			if err != nil {
				Logger.Println("Couldn't delete temp file: " + aFile.AbsolutePath + "\n Please delete manually.\n")
			}
		}
	}

	return err
}

func reassembleFile(tempPath string, suffix string, outPath string, nbrSegments int, cType ContentType) error {
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

	TTMLFlag := -1
	var ttmlDoc *ttml.Document

	for _, fPath := range files {
		in, err := os.Open(fPath)
		if err != nil {
			return fmt.Errorf("failed to open %s - %w", fPath, err)
		}
		defer in.Close()

		// dealing with text files differently
		// we write the data to the file, removing the mp4 encapsulation
		if cType == ContentTypeText {
			atoms, err := mp4.ParseAtoms(in)
			if err != nil {
				return fmt.Errorf("failed to parse atoms in %s - %w", fPath, err)
			}
			for _, atom := range atoms {
				// comment out the following if you don't want to print the styp atom (debugging)
				// if atom.Type() == mp4.STYP {
				// 	// print the styp atom
				// 	data, err := atom.ParseSTYP()
				// 	if err != nil {
				// 		panic(fmt.Sprintf("failed to parse styp atom in %s - %v", fPath, err))
				// 	}
				// 	fmt.Printf("styp: %s\n", data)
				// }

				if atom.Type() == mp4.MDAT {

					if TTMLFlag == -1 {
						// peak atom.Data and check if it starts by "<?xml"
						// if it does, it's a ttml file
						// if it doesn't, it's a webvtt file
						if (len(atom.Data) > 5) && strings.Contains(string(atom.Data[:5]), "<?xml") {
							// if Debug {
							// 	fmt.Println("ttml content found")
							// }
							TTMLFlag = 1
						} else {
							TTMLFlag = 0
							// write the atom to the output file
						}
					}

					if TTMLFlag == 1 {
						if ttmlDoc == nil {
							ttmlDoc, err = ttml.New(atom.Data)
							if err != nil {
								Logger.Println("something wrong happened when parsing the ttml data", err)
							}
						} else {
							ttmlDoc.MergeFromData(atom.Data)
						}
					} else {
						fmt.Println("non TTLM subtitles might not work")
						_, err = atom.Write(out)
						if err != nil {
							return fmt.Errorf("failed to write atom to %s - %w", outPath, err)
						}
					}
				}
			}
		} else {

			// copy the file to the output file as is
			_, err = io.Copy(out, in)
			if err != nil {
				return fmt.Errorf("failed to copy %s to %s - %w", fPath, outPath, err)
			}
		}

		err = os.Remove(fPath)
		if err != nil {
			return fmt.Errorf("failed to remove %s - %w", fPath, err)
		}
	}
	if TTMLFlag == 1 && ttmlDoc != nil {
		if err = ttmlDoc.Write(out); err != nil {
			return fmt.Errorf("failed to write ttmlDoc to %s - %w", outPath, err)
		}
	}

	return nil
}
