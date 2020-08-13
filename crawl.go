package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	requestTimeout time.Duration = time.Second * 5
)

// Lookup response about a URL lookup with the current depth
type Lookup struct {
	URL   string
	Depth int
}

// Opts provides options for the crawler
type Opts struct {
	Parallel int
	MaxDepth int
	Verbose  bool
	Limit    time.Duration
}

// ExceedsMaxDepth checks if given depth exceeds max depth specified in the options struct
func (o Opts) ExceedsMaxDepth(depth int) bool {
	return o.MaxDepth < depth
}

// HTTPClient interface for performing HTTP requests
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Crawler crawler is the client responsible for crawling URLs
type Crawler struct {
	Start      time.Time
	Ctx        context.Context
	HTTPClient HTTPClient
	StartURL   string
	Host       string
	Counter    map[string]*Count
	Opts       *Opts
	URLChan    chan Lookup
	ResChan    chan int
	Mu         *sync.Mutex
}

// Count object representing word and number count of a URL
type Count struct {
	URL     string
	Words   int
	Numbers int
}

// Lookup looks up given URL
func (c *Crawler) Lookup(url string, depth int) ([]string, error) {
	_, ok := c.readMap(url)
	if ok {
		// url already visited
		return nil, nil
	}
	c.writeMap(url, &Count{})

	if c.Opts.ExceedsMaxDepth(depth) {
		// Halt in case max depth has been exceeded
		return nil, nil
	}
	if c.Opts.Verbose {
		fmt.Printf("Visiting: %s on depth: %d\n", url, depth)
	}
	req, err := http.NewRequestWithContext(c.Ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			panic(fmt.Sprintf("unable to close response body: %s", err.Error()))
		}
	}()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	htmlBody, nextURLs := c.preProcessHTMLString(string(b))
	c.countWordsAndNumbers(url, htmlBody)
	if c.Opts.ExceedsMaxDepth(depth + 1) {
		return []string{}, nil
	}
	return nextURLs, nil
}

func (c *Crawler) countWordsAndNumbers(url, html string) {
	count := Count{}
	contentStart := 0
	contentEnd := 0

	for i := 0; i < len(html); i++ {
		// nolint: nestif
		if html[i] == '<' {
			if i != 0 {
				contentEnd = i - 1
			}
			if contentStart < contentEnd {
				// Trim spaces for given HTML node
				text := strings.TrimSpace(html[contentStart:contentEnd])
				// Split on strings as they define words
				words := strings.Split(text, " ")
				// If words are found and the text is not empty
				if len(words) != 0 && len(text) != 0 {
					for _, word := range words {
						// Check if words can be parsed as float
						_, err := strconv.ParseFloat(word, 64)
						if err == nil {
							count.Numbers++
						}
					}
					// Add words of current node to the total amount of words
					count.Words += len(words) - count.Numbers
				}
			}
		}
		if html[i] == '>' {
			contentStart = i + 1
		}
	}
	c.writeMap(url, &count)
}

func (c *Crawler) readMap(key string) (*Count, bool) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	count, ok := c.Counter[key]
	return count, ok
}

func (c *Crawler) writeMap(key string, val *Count) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.Counter[key] = val
}

func (c *Crawler) preProcessHTMLString(html string) (string, []string) {
	if len(html) == 0 || html[0] != '<' {
		// silently ignore non html strings
		return "", nil
	}
	html = c.getHTMLBodyString(html)
	html = strings.ReplaceAll(html, "\n", "")
	nextURLs := c.GetNextURLs(html)
	return html, nextURLs
}

func (c *Crawler) getHTMLBodyString(html string) string {
	bodyStart := 0
	bodyEnd := 0
	// nolint: gomnd
	for i := 0; i < len(html); i++ {
		// Calculate the start position of the body element
		if i+6 < len(html) && html[i:i+5] == "<body" {
			for j := i + 5; j < len(html); j++ {
				if html[j] == '>' {
					bodyStart = j + 1
					break
				}
			}
		}
		// Calculate the end position of the body element
		if i+7 < len(html) && html[i:i+7] == "</body>" {
			bodyEnd = i
		}
	}

	// Remove possible script elements nested in the body tag
	re := regexp.MustCompile(`<script(.*)<\/script>`)
	res := re.ReplaceAllString(html[bodyStart:bodyEnd], "")

	// Remove possible style elements nested in the body tag
	re = regexp.MustCompile(`<style(.*)<\/style>`)
	res = re.ReplaceAllString(res, "")

	return res
}

func initialise() (string, *Opts) {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println("Please provide the URL as first input argument")
		fmt.Println("Run 'crawl help' for usage")
		return "", nil
	}
	inputURL := args[len(args)-1]
	if inputURL == "help" {
		fmt.Println("use -d to indicate how many times the crawler needs to recurse")
		fmt.Println("use -p to indicate the amount of parallel threads")
		fmt.Println("use -v to run the crawler in verbose mode")
		fmt.Println("use -limit to specify the time interval to wait between requests")
		return "", nil
	}
	if !strings.HasPrefix(inputURL, "http") {
		inputURL = fmt.Sprintf("https://%s", inputURL)
	}
	_, err := url.Parse(inputURL)
	if err != nil {
		fmt.Printf("Invalid URL provided: %s", err)
		return "", nil
	}
	dFlag := flag.Int("d", 0, "Use d to provide the max depth for the crawler to search")
	pFlag := flag.Int("p", 1, "Use p to provide the amount of parallel requests which can be executed")
	vFlag := flag.Bool("v", false, "Use v to indicate running the crawler in verbose mode")
	limitFlag := flag.Int("limit", 0, "Use d to provide the max depth for the crawler to search")

	flag.Parse()
	return inputURL, &Opts{
		MaxDepth: *dFlag,
		Parallel: *pFlag,
		Verbose:  *vFlag,
		Limit:    time.Millisecond * time.Duration(*limitFlag),
	}
}

func main() {
	url, opts := initialise()
	if url == "" {
		return
	}

	crawler, err := NewCrawler(url,
		&http.Client{
			Timeout: requestTimeout,
		},
		opts,
	)
	if err != nil {
		panic(err)
	}
	// Handle URLs to visit in a go routine
	for i := 0; i < crawler.Opts.Parallel; i++ {
		go func() {
			crawler.processURLs()
		}()
	}

	// Put the initial URL in the url channel
	crawler.URLChan <- Lookup{
		URL:   url,
		Depth: 0,
	}

	crawler.waitUntilDone()
}

// waitUntilDone wait until all requests are executed and print the result
func (c *Crawler) waitUntilDone() {
	countTotalRequests := 1
	requestsDone := 0
	for {
		res := <-c.ResChan
		countTotalRequests += res
		requestsDone++
		if requestsDone == countTotalRequests {
			break
		}
	}
	if c.Opts.Verbose {
		fmt.Printf("Visited: %d URLS\n", requestsDone)
	}
	c.PrintResults()
}

// processURLs process urls from the url channel
func (c *Crawler) processURLs() {
	for {
		lookup := <-c.URLChan
		// Wait limit specified between requests
		time.Sleep(c.Opts.Limit)
		// urlChannel
		nextURLs, err := c.Lookup(lookup.URL, lookup.Depth)
		if err != nil {
			fmt.Printf("error looking up url: %s\n", err.Error())
			c.ResChan <- 0
			continue
		}
		// Add new URLs to the channel in a goroutine
		go func(next []string, curDepth int) {
			for _, url := range next {
				c.URLChan <- Lookup{
					URL:   url,
					Depth: curDepth + 1,
				}
			}
		}(nextURLs, lookup.Depth)
		c.ResChan <- len(nextURLs)
	}
}

// GetNextURLs retrieve the next URLs pointing to other URLS
// on the same host
func (c *Crawler) GetNextURLs(htmlBody string) []string {
	res := []string{}
	// Use map to create distinct URLs
	resMap := make(map[string]struct{})
	re := regexp.MustCompile(`href="(.*?)"`)
	// Find al hrefs
	urls := re.FindAllString(htmlBody, -1)
	for _, href := range urls {
		trimmedURL := href[6 : len(href)-1]
		if strings.HasPrefix(trimmedURL, "http") {
			url, err := url.Parse(trimmedURL)
			// If valid URL and same host append
			if err != nil || url.Host != c.Host {
				continue
			}
		} else {
			_, err := url.Parse(trimmedURL)
			if err != nil {
				// If href just a path append it to the current host
				trimmedURL = fmt.Sprintf("https://%s", path.Join(c.Host, href[6:len(href)-1]))
			} else {
				trimmedURL = fmt.Sprintf("https://%s", trimmedURL)
			}
		}
		url, err := url.Parse(trimmedURL)
		if err != nil {
			continue
		}
		resMap[fmt.Sprintf("https://%s", path.Join(url.Host, url.Path))] = struct{}{}
	}
	// Create slice of resulting URLs
	for k := range resMap {
		res = append(res, k)
	}

	return res
}

// PrintResults prints the results of the gathered pages
func (c *Crawler) PrintResults() {
	totalWords := 0
	totalNumbers := 0
	for path, count := range c.Counter {
		url, err := url.Parse(path)
		if err != nil {
			continue
		}
		fmt.Printf("%s\t\t%d\t%d\t\t%s\n", url.Host, count.Words, count.Numbers, url.Path)
		totalWords += count.Words
		totalNumbers += count.Numbers
	}
	if c.Opts.Verbose {
		fmt.Printf("Found %d words and %d numbers for base URL %s with depth %d\n", totalWords, totalNumbers, c.StartURL, c.Opts.MaxDepth)
		fmt.Printf("Execution duration: %s\n", time.Since(c.Start))
	}
}

// NewCrawler creates a new crawler
func NewCrawler(startURL string, httpClient HTTPClient, opts *Opts) (*Crawler, error) {
	u, err := url.Parse(startURL)
	if err != nil {
		return nil, err
	}

	return &Crawler{
		Start:      time.Now(),
		Ctx:        context.Background(),
		HTTPClient: httpClient,
		StartURL:   startURL,
		Host:       u.Host,
		Counter:    make(map[string]*Count),
		Opts:       opts,
		URLChan:    make(chan Lookup),
		ResChan:    make(chan int),
		Mu:         &sync.Mutex{},
	}, nil
}
