// Copyright 2018 Joshua J Baker. All rights reserved.

package osmfile

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var primary = "https://planet.openstreetmap.org/pbf/"

// AllMirrors are a list of all known mirrors
var AllMirrors = []string{
	"https://ftp.spline.de/pub/openstreetmap/pbf/",
	"https://ftp5.gwdg.de/pub/misc/openstreetmap/planet.openstreetmap.org/pbf/",
	"https://ftp.fau.de/osm-planet/pbf/",
	"https://free.nchc.org.tw/osm.planet/pbf/",
	"https://ftpmirror.your.org/pub/openstreetmap/pbf/",
	"https://download.bbbike.org/osm/planet/",
	"https://ftp.nluug.nl/maps/planet.openstreetmap.org/pbf/",
	"https://ftp.osuosl.org/pub/openstreetmap/pbf/",
	"https://planet.passportcontrol.net/pbf/",
	"https://planet.osm-hr.org/pbf/",
}

// Latest returns the latest (most recent) planet names on the primary OSM
// server.
func Latest() (names []string, err error) {
	resp, err := http.Get(primary)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New(resp.Status)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	anchors := strings.Split(string(body), "<a ")
	for i := 1; i < len(anchors); i++ {
		if parts := strings.Split(anchors[i], `href="`); len(parts) > 1 {
			name := filepath.Base(strings.Split(parts[1], `"`)[0])
			if validBaseName(name) && name != "planet-latest.osm.pbf" {
				names = append(names, name)
			}
		}
	}
	if len(names) == 0 {
		return nil, errors.New("no names found")
	}
	sort.Slice(names, func(i, j int) bool {
		return names[i] > names[j]
	})
	return names, nil
}

// Mirrors returns a list of OSM mirror urls that are hosting the planet file
// for the provide name.
func Mirrors(name string) (mirrors []string, err error) {
	client := &http.Client{Timeout: time.Second * 15}
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, mirror := range AllMirrors {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			resp, err := client.Head(url)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					mu.Lock()
					mirrors = append(mirrors, url)
					mu.Unlock()
				}
			}
		}(mirror + name)
	}
	wg.Wait()
	if len(mirrors) == 0 {
		return nil, errors.New("no mirrors found")
	}
	sort.Strings(mirrors)
	return mirrors, nil
}

// Downloader ...
type Downloader interface {
	Error() error
	Status() DownloadStatus
	Reader() io.ReadCloser
	Stop()
}

type dlfut struct {
	cond       *sync.Cond
	done       bool
	path       string
	err        error
	downloaded int64
	size       int64
}

// DownloadStatus ...
type DownloadStatus struct {
	Done       bool
	Path       string
	Downloaded int64
	Size       int64
}

func (dl *dlfut) Stop() {
	dl.cond.L.Lock()
	defer dl.cond.L.Unlock()
	defer dl.cond.Broadcast()
	if dl.done || dl.err != nil {
		return
	}
	dl.err = errors.New("stopped")
}

func (dl *dlfut) Error() error {
	dl.cond.L.Lock()
	defer dl.cond.L.Unlock()
	for !dl.done {
		dl.cond.Wait()
	}
	return dl.err
}

func (dl *dlfut) Status() DownloadStatus {
	dl.cond.L.Lock()
	defer dl.cond.L.Unlock()
	return DownloadStatus{
		Done:       dl.done,
		Path:       dl.path,
		Downloaded: dl.downloaded,
		Size:       dl.size,
	}
}

type dlErrReader struct {
	err error
}

func (rd *dlErrReader) Read(p []byte) (int, error) {
	return 0, rd.err
}
func (rd *dlErrReader) Close() error {
	return rd.err
}

type dlReader struct {
	cerr error // error at creation
	dl   *dlfut
	f    *os.File
	read int64
}

func (rd *dlReader) File() *os.File {
	return rd.f
}

func (rd *dlReader) Read(p []byte) (int, error) {
	for {
		n, err := rd.f.Read(p)
		if n > 0 {
			rd.read += int64(n)
		}
		rd.dl.cond.L.Lock()
		if rd.dl.err != nil {
			rd.dl.cond.L.Unlock()
			return n, rd.dl.err
		}
		if err == io.EOF {
			if n > 0 {
				if rd.read < rd.dl.size {
					err = nil
				}
				rd.dl.cond.L.Unlock()
				return n, err
			}
			if rd.read > rd.dl.size {
				rd.dl.cond.L.Unlock()
				return n, errors.New("corrupt: too much data written")
			}
			if rd.read == rd.dl.size {
				rd.dl.cond.L.Unlock()
				return n, io.EOF
			}
			rd.dl.cond.Wait()
			rd.dl.cond.L.Unlock()
			continue
		}
		rd.dl.cond.L.Unlock()
		return n, err
	}

}
func (rd *dlReader) Close() error {
	return rd.f.Close()
}

func (dl *dlfut) Reader() io.ReadCloser {
	dl.cond.L.Lock()
	defer dl.cond.L.Unlock()
	for {
		if dl.err != nil {
			return &dlErrReader{err: dl.err}
		}
		if dl.path == "" {
			dl.cond.Wait()
			continue
		}
		f, err := os.Open(dl.path)
		if err != nil {
			return &dlErrReader{err: dl.err}
		}
		if dl.done {
			return f
		}
		return &dlReader{dl: dl, f: f}
	}
}

// Download the OSM planet file into the provide file path.
func Download(planetURL string, path string) Downloader {
	dl := new(dlfut)
	dl.cond = sync.NewCond(&sync.Mutex{})
	go func() {
		defer func() {
			dl.cond.L.Lock()
			dl.done = true
			dl.cond.Broadcast()
			dl.cond.L.Unlock()
		}()
		if err := download(planetURL, path, dl); err != nil {
			dl.cond.L.Lock()
			if dl.err == nil {
				dl.err = err
			}
			dl.cond.Broadcast()
			dl.cond.L.Unlock()
		}
	}()
	return dl
}

func download(url string, path string, dl *dlfut) error {
	client := &http.Client{}

	var primaryURL string
	if strings.HasPrefix(filepath.Base(url), "planet-") {
		primaryURL = primary + filepath.Base(url)
	} else {
		primaryURL = url
	}
	res, err := client.Head(primaryURL)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		return errors.New(res.Status)
	}
	size, err := strconv.ParseInt(res.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	start, err := f.Seek(0, 2)
	if err != nil {
		return err
	}
	if start > size {
		return errors.New("corrupt: too much data written")
	}

	dl.cond.L.Lock()
	dl.path = path
	dl.size = size
	dl.downloaded = start
	dl.cond.Broadcast()
	dl.cond.L.Unlock()
	if start == size {
		// already downloaded
		return nil
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, size-1))
	if res.StatusCode != 206 && res.StatusCode != 200 {
		return errors.New(res.Status)
	}
	res, err = client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	packet := make([]byte, 4096)
	written := start
	for {
		n, err := res.Body.Read(packet)
		if n > 0 {
			written += int64(n)
			if written > size {
				return errors.New("corrupt: too much data written")
			}
			dl.cond.L.Lock()
			if dl.err != nil {
				err := dl.err
				dl.cond.L.Unlock()
				return err
			}
			if _, err := f.Write(packet[:n]); err != nil {
				dl.cond.L.Unlock()
				return err
			}
			dl.downloaded = written
			dl.cond.Broadcast()
			dl.cond.L.Unlock()
		}
		if err != nil {
			if err == io.EOF {
				if written != size {
					return io.ErrUnexpectedEOF
				}
				break
			}
			return err
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

func validBaseName(name string) bool {
	if name == "planet-latest.osm.pbf" {
		return true
	}
	parts := strings.Split(name, ".")
	if len(parts) != 3 || parts[1] != "osm" || parts[2] != "pbf" {
		return false
	}
	if parts = strings.Split(parts[0], "-"); len(parts) != 2 {
		return false
	}
	if _, err := strconv.Atoi(parts[1]); err != nil || len(parts[1]) != 6 {
		return false
	}
	return true
}
