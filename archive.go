package main

import (
	"io/ioutil"
	"log"
	"mse/av"
	"mse/av/avutil"
	"mse/cgo/ffmpeg"
	"path"
	"sort"
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

func files(p []string, start time.Time, end time.Time, fileStart string, checkDuration bool) ([]string, int, time.Duration) {
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
					allDuration += dur * time.Second
				}

				timeFile, err := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(file.Name(), ".flv", ""))
				if err != nil {
					continue
				}

				if timeFile.After(end) {
					break
				}

				if timeFile.Before(start) && !timeFile.Equal(start) {
					if len(paths) == 0 && checkDuration {
						dur := duration(path.Join(pz, file.Name()))
						if timeFile.Add(dur * time.Second).After(start) {
							paths = append(paths, path.Join(pz, file.Name()))
							length += int(file.Size())
						}
					} else if file.Name() == fileStart {
						paths = append(paths, path.Join(pz, file.Name()))
						length += int(file.Size())
					}
					continue
				}

				if timeFile.Equal(start) {
					paths = append(paths, path.Join(pz, fs[i].Name()))
					continue
				}

				if timeFile.After(start) {
					paths = append(paths, path.Join(pz, file.Name()))
					length += int(file.Size())
				}

			}
		}
	}
	sortPath(paths)
	return paths, length, allDuration
}

func filesStream(p []string, start time.Time, fileStart string) []string {
	var paths []string
	for _, pz := range p {
		fs, err := ioutil.ReadDir(pz)
		if err != nil {
			log.Println(err)
			return []string{}
		}

		for _, file := range fs {
			if !file.IsDir() {
				if file.Size() < (1024 * 500) {
					continue
				}
				timeFile, err := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(file.Name(), ".flv", ""))
				if err != nil {
					continue
				}

				if timeFile.Before(start) && !timeFile.Equal(start) {
					if file.Name() == fileStart {
						paths = append(paths, path.Join(pz, file.Name()))
					}
					continue
				}

				if timeFile.After(start) || timeFile.Equal(start) {
					paths = append(paths, path.Join(pz, file.Name()))
				}
			}
		}
	}
	sortPath(paths)
	return paths
}

func sortPath(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		_, fileNameA := path.Split(paths[i])
		timeFileA, err := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(fileNameA, ".flv", ""))
		if err != nil {
			return false
		}

		_, fileNameB := path.Split(paths[j])
		timeFileB, err := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(fileNameB, ".flv", ""))
		if err != nil {
			return false
		}
		return timeFileA.Before(timeFileB)
	})
}
