package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/queue"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var ErrFileTooLarge = errors.New("file too large")

type ExeCrawler struct {
	DownloadFolderPath  string
	StartPoints         []string
	AllowedDomains      []string
	urls                chan string
	DownloaderNum       int
	MaxDownloadFileSize int64
	crawDone            chan struct{}
	indexLock           sync.Locker
	index               map[string]string   // 索引
	IndexFile           string              // 索引文件，从url->map[url]sha256
	urlIndex            map[string]struct{} // 下载过的url链接
	indexWriter         *csv.Writer
	wg                  *sync.WaitGroup
	lock                sync.Locker
}

type optFunc func(crawler *ExeCrawler) error

func WithDownloadFolderPath(downloadFolderPath string) optFunc {
	return func(crawler *ExeCrawler) error {
		crawler.DownloadFolderPath = downloadFolderPath
		return nil
	}
}

func WithStartPoints(startPoints ...string) optFunc {
	return func(crawler *ExeCrawler) error {
		crawler.StartPoints = startPoints
		return nil
	}
}

func WithAllowedDomains(domains ...string) optFunc {
	return func(crawler *ExeCrawler) error {
		crawler.AllowedDomains = domains
		return nil
	}
}

func WithDownloaderNum(num int) optFunc {
	return func(crawler *ExeCrawler) error {
		crawler.DownloaderNum = num
		return nil
	}
}

func WithMaxDownLoadFileSize(num int64) optFunc {
	return func(crawler *ExeCrawler) error {
		crawler.MaxDownloadFileSize = num
		return nil
	}
}

func WithIndexFile(fpath string) optFunc {
	return func(crawler *ExeCrawler) error {
		var f *os.File
		var err error
		var records [][]string
		crawler.IndexFile = fpath
		if _, err = os.Stat(fpath); err != nil {
			if f, err = os.Create(fpath); err != nil {
				return err
			}
			f.Close()
		}
		if f, err = os.OpenFile(fpath, os.O_APPEND|os.O_WRONLY, 0666); err != nil {
			return err
		}
		indexReader := csv.NewReader(f)
		records, err = indexReader.ReadAll()
		for i := range records {
			crawler.index[records[i][0]] = records[i][1]
			crawler.urlIndex[records[i][1]] = struct{}{}
		}
		crawler.indexWriter = csv.NewWriter(f)
		return nil
	}
}

func (p *ExeCrawler) Init() {
	p.DownloaderNum = 10
	p.urls = make(chan string, 100)
	p.crawDone = make(chan struct{})
	p.urlIndex = make(map[string]struct{})
	p.wg = &sync.WaitGroup{}
	p.lock = &sync.Mutex{}
	p.indexLock = &sync.Mutex{}
	p.index = make(map[string]string)
}

func NewExeCrawler(opts ...optFunc) (*ExeCrawler, error) {
	p := &ExeCrawler{}
	p.Init()
	for _, f := range opts {
		if err := f(p); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (p *ExeCrawler) Run() {
	// run
	p.wg.Add(1)
	go p.Crawler()
	for i := 0; i < p.DownloaderNum; i++ {
		p.wg.Add(1)
		go p.Downloader()
	}
	p.wg.Wait()
}

func (p *ExeCrawler) Crawler() {
	defer p.wg.Done()
	log.Printf("crawling into %s", p.DownloadFolderPath)

	c := colly.NewCollector(
		colly.AllowedDomains(p.AllowedDomains...),
	)
	q, _ := queue.New(100, &queue.InMemoryQueueStorage{MaxSize: 1000000})

	c.OnHTML("a[href]", func(e *colly.HTMLElement) { // OnHTML并不是针对每一个html文档，而是针对匹配到的html元素
		link := e.Attr("href")
		if strings.HasSuffix(link, ".exe") { // 下载所有带.exe后缀的链接
			p.indexLock.Lock()
			if _, ok := p.urlIndex[link]; !ok {
				p.urls <- link
			}
			p.indexLock.Unlock()
		} else {
			q.AddURL(link)
		}
	})

	for _, s := range p.StartPoints {
		q.AddURL(s)
	}

	q.Run(c)
	close(p.crawDone)
}

func (p *ExeCrawler) Downloader() {
	defer p.wg.Done()
	for {
		select {
		case url := <-p.urls:
			if err := p.download(url); err != nil {
				log.Printf("failed to download, err=%v", err)
			}
		case <-p.crawDone:
			for {
				select {
				case url := <-p.urls: // consume urls left
					if err := p.download(url); err != nil {
						log.Printf("failed to download, err=%v", err)
					}
				default: // p.urls has no data available now
					if _, ok := <-p.urls; ok { // no closed yet
						p.lock.Lock()
						if _, ok := <-p.urls; ok {
							close(p.urls)
						}
						p.lock.Unlock()
					}
					return
				}
			}
		}
	}
}

func (p *ExeCrawler) download(url string) error {

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	log.Printf("downloading %s", url)
	resp, err := client.Head(url)
	if err == nil {
		size, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
		downloadSize := int64(size)
		if downloadSize >= p.MaxDownloadFileSize {
			return ErrFileTooLarge
		}
	}

	resp, err = client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	size, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	downloadSize := int64(size)
	if downloadSize >= p.MaxDownloadFileSize {
		return ErrFileTooLarge
	}

	// 用sha256作为文件名
	content, _ := ioutil.ReadAll(resp.Body)
	sha := sha256.New()
	sha.Write(content)
	fileName := hex.EncodeToString(sha.Sum(nil))

	out, err := os.Create(filepath.Join(p.DownloadFolderPath, fileName))
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, bytes.NewBuffer(content)) // bytes.Buffer将[]byte封装成reader
	if err != nil {
		return err
	}

	p.indexLock.Lock()
	if _, ok := p.urlIndex[url]; !ok {
		p.index[fileName] = url
		p.urlIndex[url] = struct{}{}
		p.indexWriter.Write([]string{fileName, url})
		p.indexWriter.Flush()
		if p.indexWriter.Error() != nil {
			return p.indexWriter.Error()
		}
	}
	p.indexLock.Unlock()

	return nil
}
