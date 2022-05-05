package main

import (
	"exe_crawler/exe_crawler"
	"log"
	"os"
	"path/filepath"
)

func main() {
	wd, _ := os.Getwd()

	c, err := exe_crawler.New(exe_crawler.WithAllowedDomains(),
		exe_crawler.WithStartPoints(
			"https://downloadcrew.com/",
			"https://www.snapfiles.com/new/list-whatsnew.html",
			"http://www.onlyfreewares.com",
		),
		exe_crawler.WithDownloadFolderPath(filepath.Join(wd, "downloads")),
		exe_crawler.WithDownloaderNum(100),
		exe_crawler.WithMaxDownLoadFileSize(50*(1<<20)),
		exe_crawler.WithIndexFile("index.csv"),
		exe_crawler.WithQueueNum(100))

	if err != nil {
		log.Fatalf("err = %v", err)
		return
	}

	c.Run()
}
