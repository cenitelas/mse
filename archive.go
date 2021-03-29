package main

import (
	"io/ioutil"
	"log"
	"mse/av"
	"mse/av/avutil"
	"mse/cgo/ffmpeg"
	"path"
	"strings"
	"time"
)

func durations(paths []string) time.Duration {
	Config.mutex.Lock()
	defer Config.mutex.Unlock()
	var allDuration time.Duration
	for _, p := range paths {
		durationFile := ffmpeg.FileDuration(p)
		allDuration += time.Duration(durationFile) * time.Second
	}

	return allDuration
}

func duration(path string) time.Duration {
	durationFile := ffmpeg.FileDuration(path)
	return time.Duration(durationFile)
}

func send(p chan av.Packet, paths []string, close chan bool, start time.Time, end time.Time) {
	files := make(map[string]Video)
	for _, pathFile := range paths {
		in, _ := avutil.Open(pathFile)
		st, _ := in.Streams()
		_, fileName := path.Split(pathFile)
		startTime, _ := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(fileName, ".flv", ""))
		v := Video{
			Data:      in,
			Codecs:    st,
			StartTime: startTime,
		}
		files[pathFile] = v
	}

	keyframe := false
	count := 0
	var err error
	var totalTime time.Duration
	var last av.Packet

	for {
		var pck av.Packet

		if pck, err = files[paths[count]].Data.ReadPacket(); err != nil {
			totalTime = last.Time
			if count < len(paths)-1 {
				count += 1
				continue
			} else {
				break
			}
		}

		if start.After(files[paths[count]].StartTime.Add(pck.Time)) {
			continue
		}

		if files[paths[count]].StartTime.Add(pck.Time).After(end) {
			continue
		}

		if pck.IsKeyFrame {
			keyframe = true
		}
		if !keyframe {
			continue
		} else {
			pck.Time = pck.Time + totalTime
			p <- pck
		}
		last = pck
	}

	for _, pathFile := range paths {
		files[pathFile].Data.Close()
	}

	close <- true
}

func files(p []string, start time.Time, end time.Time, checkDuration bool) ([]string, int, time.Duration) {
	var paths []string
	var length int
	length = 0
	allDuration := time.Duration(0)
	for _, pz := range p {
		fs, err := ioutil.ReadDir(pz)
		if err != nil {
			log.Println(err)
			return []string{}, 0, 0
		}

		for i, file := range fs {
			if !file.IsDir() {
				if file.Size() < (1024 * 500) {
					continue
				}
				if checkDuration {
					dur := duration(path.Join(pz, file.Name()))
					if dur <= 0 {
						continue
					}
					allDuration += dur
				}

				timeFile, err := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(file.Name(), ".flv", ""))
				if err != nil {
					continue
				}

				if timeFile.After(end) {
					break
				}

				if timeFile.Before(start) && !timeFile.Equal(start) {
					continue
				}

				if timeFile.After(start) || timeFile.Equal(start) {
					if len(paths) == 0 && !timeFile.Equal(start) && i > 0 {
						paths = append(paths, path.Join(pz, fs[i-1].Name()))
					}
					paths = append(paths, path.Join(pz, file.Name()))
					length += int(file.Size())
				}

			}
		}
	}
	return paths, length, allDuration
}

func filesStream(p []string, start time.Time) []string {
	var paths []string
	for _, pz := range p {
		fs, err := ioutil.ReadDir(pz)
		if err != nil {
			log.Println(err)
			return []string{}
		}

		for i, file := range fs {
			if !file.IsDir() {
				if file.Size() < (1024 * 500) {
					continue
				}
				timeFile, err := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(file.Name(), ".flv", ""))
				if err != nil {
					continue
				}

				if timeFile.Before(start) && !timeFile.Equal(start) {
					continue
				}

				if timeFile.After(start) || timeFile.Equal(start) {
					if len(paths) == 0 && !timeFile.Equal(start) && i > 0 {
						paths = append(paths, path.Join(pz, fs[i-1].Name()))
					}
					paths = append(paths, path.Join(pz, file.Name()))
				}
			}
		}
	}
	return paths
}
