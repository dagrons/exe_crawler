package main

import (
	"log"
	"os"
	"path/filepath"
)

func main() {
	wd, _ := os.Getwd()
	c, err := NewExeCrawler(WithAllowedDomains(
		"www.onlyfreewares.com",
		"www.portablefreeware.com",
		"www.snapfiles.com",
		"https://downloadcrew.com/",
	),
		WithStartPoints("http://www.portablefreeware.com/",
			"https://downloadcrew.com/",
			"https://www.snapfiles.com/new/list-whatsnew.html",
			"http://www.onlyfreewares.com",
		),
		WithDownloadFolderPath(filepath.Join(wd, "downloads")),
		WithDownloaderNum(100),
		WithMaxDownLoadFileSize(50*(1<<20)),
		WithIndexFile("index.csv"))
	if err != nil {
		log.Fatalf("err = %v", err)
		return
	}
	c.Run()
}
