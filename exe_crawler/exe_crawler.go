package exe_crawler

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/queue"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrFileTooLarge = errors.New("file too large")
var ErrInvalidContent = errors.New("content type invalid")

type ExeCrawler struct {
	downloadFolderPath  string
	startPoints         []string
	allowedDomains      []string
	urls                chan string // 等待被downloader消费url
	downloaderNum       int
	maxDownloadFileSize int64
	crawDone            chan struct{}
	index               sync.Map // 下载过的url链接
	indexFileLocker     sync.Locker
	indexFile           string   // 下载过的url索引文件
	urlIndex            sync.Map // 下载过或者当前会话访问过的url链接
	indexWriter         *csv.Writer
	wg                  *sync.WaitGroup
	urlLock             sync.Locker // avoid concurrent close of urls
	queueNum            int64
	downloadTimeout     time.Duration
}

type OptFunc func(crawler *ExeCrawler) error

func WithDownloadFolderPath(downloadFolderPath string) OptFunc {
	return func(crawler *ExeCrawler) error {
		crawler.downloadFolderPath = downloadFolderPath
		return nil
	}
}

func WithStartPoints(startPoints ...string) OptFunc {
	return func(crawler *ExeCrawler) error {
		crawler.startPoints = startPoints
		return nil
	}
}

func WithAllowedDomains(domains ...string) OptFunc {
	return func(crawler *ExeCrawler) error {
		crawler.allowedDomains = domains
		return nil
	}
}

func WithDownloaderNum(num int) OptFunc {
	return func(crawler *ExeCrawler) error {
		crawler.downloaderNum = num
		return nil
	}
}

func WithMaxDownLoadFileSize(num int64) OptFunc {
	return func(crawler *ExeCrawler) error {
		crawler.maxDownloadFileSize = num
		return nil
	}
}

func WithQueueNum(num int64) OptFunc {
	return func(crawler *ExeCrawler) error {
		crawler.queueNum = num
		return nil
	}
}

func WithIndexFile(fpath string) OptFunc {
	return func(crawler *ExeCrawler) error {
		var f *os.File
		var err error
		var records [][]string
		crawler.indexFile = fpath
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
			crawler.index.Store(records[i][0], records[i][1])
			crawler.urlIndex.Store(records[i][1], struct{}{})
		}
		crawler.indexWriter = csv.NewWriter(f)
		return nil
	}
}

func WithDownloadTimeout(timeout time.Duration) OptFunc {
	return func(crawler *ExeCrawler) error {
		crawler.downloadTimeout = timeout
		return nil
	}
}

func (p *ExeCrawler) Init() { // non-empty initialization
	p.downloaderNum = 10
	p.urls = make(chan string, 100)
	p.crawDone = make(chan struct{})
	p.urlIndex = sync.Map{}
	p.wg = &sync.WaitGroup{}
	p.urlLock = &sync.Mutex{}
	p.index = sync.Map{}
	p.queueNum = 100
	p.indexFileLocker = &sync.Mutex{}
}

func New(opts ...OptFunc) (*ExeCrawler, error) {
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
	go p.crawler()
	for i := 0; i < p.downloaderNum; i++ {
		p.wg.Add(1)
		go p.downloader()
	}
	p.wg.Wait()
}

func (p *ExeCrawler) crawler() {
	defer p.wg.Done()
	log.Printf("crawling into %s", p.downloadFolderPath)

	c := colly.NewCollector(
		colly.AllowedDomains(p.allowedDomains...),
	)
	q, _ := queue.New(100, &queue.InMemoryQueueStorage{MaxSize: 100000000})

	c.OnHTML("a[href]", func(e *colly.HTMLElement) { // OnHTML并不是针对每一个html文档，而是针对匹配到的html元素
		link := e.Attr("href")
		if !(strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://")) { // link contains only query part
			tmpUrl := e.Request.URL
			path := link
			if !strings.HasPrefix(link, "/") { // 路径是同级目录
				currentPath := tmpUrl.RawPath
				i := len(currentPath) - 1
				if i < 0 {
					path = fmt.Sprintf("/%s", path)
				} else {
					for i >= 0 && currentPath[i] != '/' {
						i--
					}
					path = fmt.Sprintf("%s/%s", currentPath[:i], path)
				}
			}
			url := &url.URL{
				Scheme:  tmpUrl.Scheme,
				Host:    tmpUrl.Host,
				RawPath: path,
			}
			link = url.String()
		}
		if strings.HasSuffix(link, ".exe") { // 下载所有带.exe后缀的链接
			if _, ok := p.urlIndex.Load(link); !ok {
				p.urls <- link
				p.urlIndex.Store(link, struct{}{})
			}
		} else {
			q.AddURL(link)
		}
	})

	for _, s := range p.startPoints {
		q.AddURL(s)
	}

	q.Run(c)
	close(p.crawDone)
}

func (p *ExeCrawler) downloader() {
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
						p.urlLock.Lock()
						if _, ok := <-p.urls; ok {
							close(p.urls)
						}
						p.urlLock.Unlock()
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
		Timeout: p.downloadTimeout,
	}

	log.Printf("downloading %s", url)
	headResp, err := client.Head(url)
	if err == nil {
		defer headResp.Body.Close() // always remember to close io stream
		size, _ := strconv.Atoi(headResp.Header.Get("Content-Length"))
		downloadSize := int64(size)
		if downloadSize >= p.maxDownloadFileSize {
			return ErrFileTooLarge
		}
		contentType := headResp.Header.Get("Content-Type")
		if contentType != "application/octet-stream" && contentType != "application/x-msdos-program" { // ignore non-binary content
			return ErrInvalidContent
		}
	}
	// else ignore it

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	size, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	downloadSize := int64(size)
	if downloadSize >= p.maxDownloadFileSize {
		return ErrFileTooLarge
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/octet-stream" && contentType != "application/x-msdos-program" { // ignore non-binary content
		return ErrInvalidContent
	}

	// 用sha256作为文件名
	content, _ := ioutil.ReadAll(resp.Body)
	sha := sha256.New()
	sha.Write(content)
	fileName := hex.EncodeToString(sha.Sum(nil))

	out, err := os.Create(filepath.Join(p.downloadFolderPath, fileName))
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, bytes.NewBuffer(content)) // bytes.Buffer将[]byte封装成reader, io.Copy
	if err != nil {
		return err
	}

	if _, ok := p.index.Load(fileName); !ok {
		p.index.Store(fileName, url)
		p.indexFileLocker.Lock()
		defer p.indexFileLocker.Unlock()
		p.indexWriter.Write([]string{fileName, url})
		p.indexWriter.Flush()
		if p.indexWriter.Error() != nil {
			return p.indexWriter.Error()
		}
	}
	return nil
}
