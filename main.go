package main

import (
	"exe_crawler/exe_crawler"
	"flag"
	"log"
)

var (
	downloadFolderPath string
	indexFilePath      string
)

func main() {
	flag.StringVar(&downloadFolderPath, "t", "", "download folder for exe")
	flag.StringVar(&indexFilePath, "i", "", "index file path")
	flag.Parse()

	c, err := exe_crawler.New(exe_crawler.WithAllowedDomains(),
		exe_crawler.WithStartPoints(
			"https://downloadcrew.com/",
			"https://www.snapfiles.com/new/list-whatsnew.html",
			"http://www.onlyfreewares.com",
		),
		exe_crawler.WithDownloaderNum(100),
		exe_crawler.WithMaxDownLoadFileSize(50*(1<<20)),
		exe_crawler.WithDownloadFolderPath(downloadFolderPath),
		exe_crawler.WithIndexFile(indexFilePath),
		exe_crawler.WithQueueNum(100))

	if err != nil {
		log.Fatalf("err = %v", err)
		return
	}

	c.Run()
}
