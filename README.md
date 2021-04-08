# osmfile

A downloader and reader for the OSM planet files.

## Features

- Download the lastest planet files.
- Get a list of mirrors serving specific planet files.
- Stop and resume downloads.
- Includes an OSM PBF parser.
- Read and process PBF data while download is in process.

## Using

Download the Go package.

```
go get -u github.com/tidwall/osmfile
```

### Examples

Get the latest known osm planet files in order of most recently created.

```go
names, err := osmfile.Latest()
if err != nil {
	panic(err)
}
for _, name := range names {
	fmt.Printf("%s\n", name)
}

// Output something like:
// planet-210329.osm.pbf
// planet-210322.osm.pbf
// planet-210315.osm.pbf
// planet-210301.osm.pbf
// planet-210201.osm.pbf
// planet-210104.osm.pbf
```

Get a list of the mirror urls.

```go
// using a valid planet file name.
name := "planet-210329.osm.pbf"

mirrors, err := osmfile.Mirrors(name)
if err != nil {
	panic(err)
}
for _, mirror := range mirrors {
	fmt.Printf("%s\n", mirror)
}

// Output something like:
// https://free.nchc.org.tw/osm.planet/pbf/planet-210329.osm.pbf
// https://ftp.fau.de/osm-planet/pbf/planet-210329.osm.pbf
// https://ftp5.gwdg.de/pub/misc/openstreetmap/planet.openstreetmap.org/pbf/planet-210329.osm.pbf
// https://planet.osm-hr.org/pbf/planet-210329.osm.pbf
```

Download the planet file to disk.

```go

// using a valid mirror url
mirrorURL := "https://ftp.fau.de/osm-planet/pbf/planet-210329.osm.pbf"

// Start downloading. The downloading happens in a background, and will
// continue until an error occurs or the downloader is explicitly stopped
// with the Stop() function.

dl := osmfile.Download(mirrorURL, "planet.pbf")

// Here we will download the file and print a status every second.
for {
	status := dl.Status()
	fmt.Printf("%f.1%% %d / %d MB Downloaded\n",
		status.Downloaded/1024/1024,
		status.Size/1024/1024)
	if status.Done {
		// The downloader is done due to success or failure.
		break
	}
	time.Sleep(time.Second)
}
// Check if the download failed
if err := dl.Error(); err != nil {
	panic(err)
}
```

Here's a complete example that downloads the latest planet file from a
random mirror and parses PBF data at the same time.

```go
package main

import (
	"fmt"
	"io"
	"math/rand"

	"github.com/tidwall/osmfile"
)

func main() {
	names, err := osmfile.Latest()
	if err != nil {
		panic(err)
	}
	mirrors, err := osmfile.Mirrors(names[0])
	if err != nil {
		panic(err)
	}

	rand.Seed(time.Now().UnixNano())
	url := mirrors[rand.Int()%len(mirrors)]
	fmt.Printf("downloading %s\n", url)

	dl := osmfile.Download(url, names[0])
	defer dl.Stop()

	rd := dl.Reader()
	defer rd.Close()

	brd := osmfile.NewBlockReader(rd)
	var nodes, ways, relations int
	for num := 0; ; num++ {
		// Read and parse the next OSM PBF block. 
		// You can choose to skip the parsing by SkipBlock instead of ReadBlock.
		n, block, err := brd.ReadBlock()
		if err != nil {
			if err == io.EOF {
				// No more blocks
				break
			}
			panic(err)
		}

		// The block variable now contains all of the OSM data belonging to
		// the current PBF block.
		
		// Let's just print some basic info and the download status
		nodes += block.NumNodes()
		ways += block.NumWays()
		relations += block.NumRelations()

		status := dl.Status()
		fmt.Printf("BLOCK #%d (%s, %d bytes) ",
			num, block.DataKind(), n)
		fmt.Printf("(nodes: %d, ways: %d, rels: %d) ",
			nodes, ways, relations)
		fmt.Printf("(%d/%d MB, %.1f%%)\n",
			status.Downloaded/1024/1024,
			status.Size/1024/1024,
			float64(status.Downloaded)/float64(status.Size)*100)
	}
	// Check if the download failed
	if err := dl.Error(); err != nil {
		panic(err)
	}

	// Yay! the entire OSM file was download and every block parsed.
}
```


