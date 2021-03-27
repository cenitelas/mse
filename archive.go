package main

import (
	"errors"
	"io/ioutil"
	"log"
	"mse/av"
	"mse/av/avutil"
	"path"
	"strings"
	"time"
)

func durations(paths []string) (time.Duration, error) {
	timeChan := make(chan time.Duration)
	errChan := make(chan error)

	go func(tc chan time.Duration, e chan error) {
		files := make(map[string]av.DemuxCloser)
		for _, p := range paths {
			infile, err := avutil.Open(p)
			if err != nil {
				e <- err
				return
			}
			files[p] = infile
		}

		start := false
		count := 0
		var err error
		var totalTime time.Duration
		var last av.Packet
		if len(files) == 0 {
			e <- errors.New("not found")
			return
		}
		for {
			var pck av.Packet

			if pck, err = files[paths[count]].ReadPacket(); err != nil {
				totalTime = last.Time
				if count < len(files)-1 {
					count++
					continue
				} else {
					break
				}
			}

			if pck.IsKeyFrame {
				start = true
			}
			if !start {
				continue
			}

			if start {
				last = pck
				last.Time += totalTime
			}
		}
		timeChan <- totalTime
	}(timeChan, errChan)

	select {
	case t := <-timeChan:
		return t, nil
	case e := <-errChan:
		return 0, e
	}
}

func duration(path string) (time.Duration, error) {
	timeChan := make(chan time.Duration)
	errChan := make(chan error)

	go func(tc chan time.Duration, e chan error) {

		infile, err := avutil.Open(path)
		if err != nil {
			e <- err
			return
		}
		start := false
		var totalTime time.Duration
		var last av.Packet
		for {
			var pck av.Packet

			if pck, err = infile.ReadPacket(); err != nil {
				totalTime = last.Time
				break
			}

			if pck.IsKeyFrame {
				start = true
			}
			if !start {
				continue
			}

			if start {
				last = pck
				last.Time += totalTime
			}
		}
		timeChan <- totalTime
	}(timeChan, errChan)

	select {
	case t := <-timeChan:
		return t, nil
	case e := <-errChan:
		return 0, e
	}
}

func send(p chan av.Packet, paths []string, close chan bool, start time.Time, end time.Time) {
	files := make(map[string]Vidos)
	for _, pathFile := range paths {
		in, _ := avutil.Open(pathFile)
		st, _ := in.Streams()
		_, fileName := path.Split(pathFile)
		startTime, _ := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(fileName, ".flv", ""))
		v := Vidos{
			Data:      in,
			Streams:   st,
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

func files(p []string, start time.Time, end time.Time) ([]string, int64) {
	var paths []string
	var length int64
	length = 0

	for _, pz := range p {
		fs, err := ioutil.ReadDir(pz)
		if err != nil {
			log.Println(err)
			return []string{}, 0
		}

		for i, file := range fs {
			if !file.IsDir() {
				length += file.Size()
				timeFile, err := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(file.Name(), ".flv", ""))
				if err != nil {
					continue
				}

				if timeFile.Before(start) && i < len(fs)-1 {
					next, err := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(fs[i+1].Name(), ".flv", ""))
					if err != nil {
						continue
					}
					if next.After(start) {
						paths = append(paths, path.Join(pz, file.Name()))
					}

				}

				if timeFile.After(start) || timeFile.Equal(start) {
					paths = append(paths, path.Join(pz, file.Name()))
				}

				if timeFile.After(end) {
					break
				}
			}
		}
	}
	return paths, length
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

				timeFile, err := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(file.Name(), ".flv", ""))
				if err != nil {
					continue
				}

				if timeFile.Before(start) && i < len(fs)-1 {
					next, err := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(fs[i+1].Name(), ".flv", ""))
					if err != nil {
						continue
					}
					if next.After(start) {
						paths = append(paths, path.Join(pz, file.Name()))
					}

				}

				if timeFile.After(start) || timeFile.Equal(start) {
					paths = append(paths, path.Join(pz, file.Name()))
				}
			}
		}
	}

	return paths
}
