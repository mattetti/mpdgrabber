package mpdgrabber

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/abema/go-mp4"
	"github.com/mattetti/mpdgrabber/subs"
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
			outfileNameNoExt := strings.TrimSuffix(outFilePath, filepath.Ext(outFilePath))

			if filepath.Ext(track.AbsolutePath) == ".ttml" {
				fmt.Println("TTML subtitles found, but they aren't supported by FFMpeg")
				// convert the ttml to vtt
				vttPath := outfileNameNoExt + ".vtt"
				doc, err := subs.OpenTtml(track.AbsolutePath)
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

				ttmlFilePath := outfileNameNoExt + ".ttml"
				if err = os.Rename(track.AbsolutePath, ttmlFilePath); err != nil {
					Logger.Printf("Error renaming %s to %s: %v\n", track.AbsolutePath, ttmlFilePath, err)
				}

				continue
			}

			// provide a copy of the file even if it's embedded in the container
			subFilePath := outfileNameNoExt + filepath.Ext(track.AbsolutePath)
			if err = os.Rename(track.AbsolutePath, subFilePath); err != nil {
				Logger.Printf("Error renaming %s to %s: %v\n", track.AbsolutePath, subFilePath, err)
			}

			args = append(args, "-i", subFilePath)
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
	// webVTT := -1
	var ttmlDoc *subs.TtmlDocument
	var sawVTT bool
	var language string
	var trackID uint32
	var trackCues []string
	var timescale uint32
	var currentTime int

	for _, fPath := range files {

		in, err := os.Open(fPath)
		if err != nil {
			return fmt.Errorf("failed to open %s - %w", fPath, err)
		}
		defer in.Close()

		// dealing with text files differently
		// we write the data to the file, removing the mp4 encapsulation
		if cType == ContentTypeText {
			if Debug {
				fmt.Println("--", fPath)
			}
			var baseTime int
			var defaultSampleDuration uint32
			var trun *mp4.Trun

			// fmt.Println(fPath)
			_, err = mp4.ReadBoxStructure(in, func(h *mp4.ReadHandle) (interface{}, error) {
				switch h.Path[0] {
				case mp4.BoxTypeMoov():
					tkhds, err := mp4.ExtractBoxWithPayload(in, &h.BoxInfo, mp4.BoxPath{mp4.BoxTypeTrak(), mp4.BoxTypeTkhd()})
					if err != nil {
						return nil, err
					}
					if len(tkhds) == 0 {
						return nil, errors.New("tkhd box not found")
					}
					tkhd := tkhds[0].Payload.(*mp4.Tkhd)
					trackID = tkhd.TrackID

					mdhds, err := mp4.ExtractBoxWithPayload(in, &h.BoxInfo,
						mp4.BoxPath{mp4.BoxTypeTrak(), mp4.BoxTypeMdia(), mp4.BoxTypeMdhd()})
					if err != nil {
						return nil, err
					}
					if len(mdhds) == 0 {
						return nil, errors.New("mdhd box not found")
					}
					mdhd := mdhds[0].Payload.(*mp4.Mdhd)
					if mdhd.Timescale != 0 {
						timescale = mdhd.Timescale
					}

					for i, _ := range mdhd.Language {
						mdhd.Language[i] += 0x60
					}
					l := string(mdhd.Language[:])
					if l != "" {
						language = string(mdhd.Language[:])
					}

					if Debug {
						fmt.Println(">> Track", trackID, "language:", language, "timescale", timescale)
					}

					stsds, err := mp4.ExtractBoxWithPayload(in, &h.BoxInfo, mp4.BoxPath{
						mp4.BoxTypeTrak(),
						mp4.BoxTypeMdia(),
						mp4.BoxTypeMinf(),
						mp4.BoxTypeStbl(),
						mp4.BoxTypeStsd(),
					})
					if err != nil {
						fmt.Println(err)
						return nil, err
					}
					if len(stsds) == 0 {
						fmt.Println("no stsd box")
						return nil, errors.New("stsd box not found")
					}
					wvtts, err := mp4.ExtractBox(in, &stsds[0].Info, mp4.BoxPath{mp4.StrToBoxType("wvtt")})
					if len(wvtts) > 0 {
						sawVTT = true
					}

				case mp4.BoxTypeMoof():
					trun = nil

					// extract tfdt box
					tfdts, err := mp4.ExtractBoxWithPayload(in, &h.BoxInfo, mp4.BoxPath{mp4.BoxTypeTraf(), mp4.BoxTypeTfdt()})
					if err != nil {
						return nil, err
					}
					if len(tfdts) == 0 {
						return nil, errors.New("tfdt box not found")
					}
					tfdt := tfdts[0].Payload.(*mp4.Tfdt)
					if tfdt.Version < 0 || tfdt.Version > 1 {
						return nil, errors.New("TFDT version can only be 0 or 1")
					}
					baseTime = int(tfdt.GetBaseMediaDecodeTime())

					// Extract tfhd box
					tfhds, err := mp4.ExtractBoxWithPayload(in, &h.BoxInfo, mp4.BoxPath{mp4.BoxTypeTraf(), mp4.BoxTypeTfhd()})
					if err != nil {
						return nil, err
					}
					if len(tfhds) == 0 {
						return nil, errors.New("tfdt box not found")
					}
					tfhd := tfhds[0].Payload.(*mp4.Tfhd)
					defaultSampleDuration = tfhd.DefaultSampleDuration

					truns, err := mp4.ExtractBoxWithPayload(in, &h.BoxInfo, mp4.BoxPath{mp4.BoxTypeTraf(), mp4.BoxTypeTrun()})
					if err != nil {
						return nil, err
					}
					if len(truns) > 0 {
						trun = truns[0].Payload.(*mp4.Trun)
					}

				case mp4.BoxTypeMdat():
					if sawVTT {
						currentTime = baseTime

						var sampleIDX int
						var payloadSize uint32
						payloadType := make([]byte, 4)
						const boxHeaderSize = 8
						for i, presentation := range trun.Entries {
							// Note: a presentation/sample can have multiple cues.
							// That's what the presentation Sample Size represents
							duration := presentation.SampleDuration
							if duration == 0 {
								if Debug {
									fmt.Println("0 duration, backup:", defaultSampleDuration)
								}
								duration = defaultSampleDuration
							}

							// presentation time applies to all cues in the presentation
							var startTime int
							tOffset := trun.GetSampleCompositionTimeOffset(i)
							if tOffset > 0 {
								startTime = int(baseTime) + int(tOffset)
							} else {
								startTime = currentTime
							}
							currentTime = startTime + int(duration)
							cueStart := startTime
							cueEnd := currentTime
							if timescale > 0 {
								cueStart /= int(timescale)
								cueEnd /= int(timescale)
							}

							totalSize := 0
							sampleSize := int(presentation.SampleSize)
							var n int
							for sampleSize > 8 && totalSize <= sampleSize && sampleIDX < len(trun.Entries) {

								// read the payload size
								err := binary.Read(in, binary.BigEndian, &payloadSize)
								_, err = in.Read(payloadType)
								if err != nil {
									fmt.Printf("[%d of %d| sample:%d] failed to read box size/type %v - sampleSize: %d, totalSize: %d\n", sampleIDX, len(trun.Entries), n, err, sampleSize, totalSize)
									return nil, err
								}

								sampleIDX++
								n++

								totalSize += int(payloadSize)

								// VTTC
								if bytes.Equal(payloadType, []byte("vttc")) {
									// payload = reader.readBytes(payloadSize - 8);
									payload := make([]byte, int(payloadSize)-boxHeaderSize)
									err := binary.Read(in, binary.BigEndian, &payload)
									if err != nil {
										fmt.Println("failed to read payload", err)
										break
									}
									cue, err := subs.ParseVTTCPayload(payload, cueStart, cueEnd)
									if Debug {
										truncatedCue := cue
										if len(cue) > 50 {
											truncatedCue = truncatedCue[:45]
										}
										fmt.Printf("[%d of %d] sample: %d, %s\n", sampleIDX, len(trun.Entries), n, truncatedCue)
									}
									if cue != "" {
										trackCues = append(trackCues, cue)
									}
								} else {
									// VTTE (empty cue)
									if Debug {
										fmt.Printf("[%d of %d] sample: %d, %s box, %s => %s\n", sampleIDX, len(trun.Entries), n, string(payloadType), subs.WebvttTimeString(cueStart), subs.WebvttTimeString(cueEnd))
									}
									// skip the rest of the box
									in.Seek(int64(payloadSize)-int64(boxHeaderSize), io.SeekCurrent)
								}
								// else skip the rest
								// reader.skip(payloadSize - 8);
								// if duration && payload

								// read the payload

								// if n >= len(cues) {
								// 	panic(fmt.Sprintf("n: %d <= cues: %d, sampleSize: %d\n", n, len(cues), sampleSize))
								// 	// SHOULD NOT happen, that means we didn't extract many cues for a given sample/presentation.
								// 	continue
								// }
								// cue := fmt.Sprintf("%s --> %s\n", subs.WebvttTimeString(cueStart),
								// 	subs.WebvttTimeString(cueEnd))
								// for sampleSize > 0 {
								// 	fmt.Println("sampleSize:", sampleSize, "cue size:", cues[n].Size)
								// 	sampleSize -= cues[n].Size
								// 	cue += cues[n].Content + "\n"
								// 	n++
								// }

							}
						}
					}
				}
				return nil, nil
			})

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
	if sawVTT {
		fmt.Fprintf(out, "WEBVTT - mpdGrabber TrackID: %d - Language: %s\n\n", trackID, language)
		for _, cue := range trackCues {
			fmt.Fprintln(out, cue)
		}
	} else if TTMLFlag == 1 && ttmlDoc != nil {
		if err = ttmlDoc.Write(out); err != nil {
			return fmt.Errorf("failed to write ttmlDoc to %s - %w", outPath, err)
		}
	}

	return nil
}

func NewByteWriter(size int) *BytesWriter {
	return &BytesWriter{
		Size: size,
	}
}

type BytesWriter struct {
	Size int
	Buf  []byte
}

func (bw *BytesWriter) Write(p []byte) (n int, err error) {
	if len(p) < bw.Size {
		bw.Buf = p
	} else {
		bw.Buf = p[:bw.Size]
	}
	return len(bw.Buf), nil
}

/*
// TODELETE
				atoms, err := mp4.ParseAtoms(in)
				if err != nil {
					return fmt.Errorf("failed to parse atoms in %s - %w", fPath, err)
				}
				for _, atom := range atoms {

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
								webVTT = 0
							} else {
								TTMLFlag = 0
								// write the atom to the output file

								// check if webvtt
								// 4 bytes, unsigned be int, 0x00000000
								// vtt - 0x76 0x74 0x74
								if (len(atom.Data) > 7) &&
									(atom.Data[4] == 0x76) &&
									(atom.Data[5] == 0x74) &&
									(atom.Data[6] == 0x74) {
									if Debug {
										fmt.Println("-> webvtt content found")
									}
									webVTT = 1
								} else {
									webVTT = 0
								}

							}
						}

						// ttml assembling
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
							// webvtt assembling
							if webVTT == 1 {
								// parse the webvtt binary file, convert it to a string, write it to out
								data, err := extractAtomWebVTT(atom.Data)
								fmt.Println(string(data))
								if err != nil {
									return fmt.Errorf("failed to extract binary webvtt from %s - %w", fPath, err)
								}
								// write data to out
								_, err = out.Write(data)
								if err != nil {
									return fmt.Errorf("failed to write data to %s - %w", outPath, err)
								}

							} else {
								// just adding things up
								// fmt.Println("non TTLM subtitles might not work")
								_, err = atom.Write(out)
								if err != nil {
									return fmt.Errorf("failed to write atom to %s - %w", outPath, err)
								}
							}
						}
					}
				}
*/
