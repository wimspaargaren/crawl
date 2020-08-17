package main

import (
	"bytes"
	"flag"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

// ClientMock mock http client
type ClientMock struct {
	mock.Mock
}

// Do mocks the Do function
func (c *ClientMock) Do(req *http.Request) (*http.Response, error) {
	args := c.Called(req)
	return args.Get(0).(*http.Response), args.Error(1)
}

// CrawlTestSuite test suite for testing the crawler command line tool
type CrawlTestSuite struct {
	suite.Suite
}

func (s *CrawlTestSuite) TestInit() {
	tests := []struct {
		Name         string
		Args         []string
		ExpectedURL  string
		ExpectedOpts *Opts
	}{
		{
			Name:        "No opts",
			Args:        []string{"test.pro"},
			ExpectedURL: "https://test.pro",
			ExpectedOpts: &Opts{
				MaxDepth: 0,
				Parallel: 1,
				Limit:    0,
				Verbose:  false,
			},
		},
		{
			Name:        "Opts and flags",
			Args:        []string{"-v", "-p=10", "-d=2", "-limit=1000", "http://test.pro"},
			ExpectedURL: "http://test.pro",
			ExpectedOpts: &Opts{
				MaxDepth: 2,
				Parallel: 10,
				Limit:    time.Second,
				Verbose:  true,
			},
		},
		{
			Name: "No args",
		},
		{
			Name:         "Help",
			Args:         []string{"help"},
			ExpectedURL:  "",
			ExpectedOpts: nil,
		},
	}

	for _, test := range tests {
		s.Run(test.Name, func() {
			os.Args = append([]string{""}, test.Args...)
			url, options := initialise()
			s.Equal(test.ExpectedURL, url)
			s.Equal(test.ExpectedOpts, options)
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		})
	}
}

func (s *CrawlTestSuite) TestCrawl() {
	mockedClient := &ClientMock{}
	resp := GetMockHTTPResponse([]byte(`<html><body><div>some words and 1 number</div><<a href="google.com" /></body></html>`), 200)
	defer resp.Body.Close()
	mockedClient.On("Do", mock.Anything).Return(resp, nil)
	url := "https://google.com"
	crawler, err := NewCrawler(url, mockedClient, &Opts{
		MaxDepth: 2,
		Parallel: 1,
		Limit:    0,
		Verbose:  false,
	})
	s.Require().NoError(err)
	for i := 0; i < crawler.Opts.Parallel; i++ {
		go func() {
			crawler.processURLs()
		}()
	}
	crawler.URLChan <- Lookup{
		URL:   url,
		Depth: 0,
	}
	crawler.waitUntilDone()
	s.Equal(1, len(crawler.Counter))
	count, ok := crawler.Counter["https://google.com"]
	s.True(ok)
	s.Equal(4, count.Words)
	s.Equal(1, count.Numbers)
}

func (s *CrawlTestSuite) TestXMLString() {
	xmlString := `<?xml version="1.0" encoding="utf-8"?>
	<feed xmlns="http://www.w3.org/2005/Atom" xmlns:creativeCommons="http://backend.userland.com/creativeCommonsRssModule" xmlns:re="http://purl.org/atompub/rank/1.0">
		<title type="text">How to get regex to match multiple script tags? - Stack Overflow</title>
		<link rel="self" href="https://stackoverflow.com/feeds/question/1441463" type="application/atom+xml" />
		<link rel="alternate" href="https://stackoverflow.com/q/1441463" type="text/html" />
		<subtitle>most recent 30 from stackoverflow.com</subtitle>
		<updated>2020-08-17T19:38:24Z</updated>
		<id>https://stackoverflow.com/feeds/question/1441463</id>
		<creativeCommons:license>https://creativecommons.org/licenses/by-sa/4.0/rdf</creativeCommons:license> 
		<entry>
			<id>https://stackoverflow.com/q/1441463</id>
			<re:rank scheme="https://stackoverflow.com">9</re:rank>
			<title type="text">How to get regex to match multiple script tags?</title>
				<category scheme="https://stackoverflow.com/tags" term="javascript" />
				<category scheme="https://stackoverflow.com/tags" term="regex" />
			<author>
				<name>Geuis</name>
				<uri>https://stackoverflow.com/users/68788</uri>
			</author>
			<link rel="alternate" href="https://stackoverflow.com/questions/1441463/how-to-get-regex-to-match-multiple-script-tags" />
			<published>2009-09-17T21:37:02Z</published>
			<updated>2017-08-04T19:24:44Z</updated>
			<summary type="html">
				&lt;p&gt;I&#x27;m trying to return the contents of any  tags in a body of text. I&#x27;m currently using the following expression, but it only captures the contents of the first  tag and ignores any others after that. &lt;/p&gt;&#xA;&#xA;&lt;p&gt;Here&#x27;s a sample of the html:&lt;/p&gt;&#xA;&#xA;&lt;pre&gt;&lt;code&gt;    &amp;lt;script type=&quot;text/javascript&quot;&amp;gt;&#xA;        alert(&#x27;1&#x27;);&#xA;    &amp;lt;/script&amp;gt;&#xA;&#xA;    &amp;lt;div&amp;gt;Test&amp;lt;/div&amp;gt;&#xA;&#xA;    &amp;lt;script type=&quot;text/javascript&quot;&amp;gt;&#xA;        alert(&#x27;2&#x27;);&#xA;    &amp;lt;/script&amp;gt;&#xA;&lt;/code&gt;&lt;/pre&gt;&#xA;&#xA;&lt;p&gt;My regex looks like this:&lt;/p&gt;&#xA;&#xA;&lt;pre&gt;&lt;code&gt;//scripttext contains the sample&#xA;re = /&amp;lt;script\b[^&amp;gt;]*&amp;gt;([\s\S]*?)&amp;lt;\/script&amp;gt;/gm;&#xA;var scripts  = re.exec(scripttext);&#xA;&lt;/code&gt;&lt;/pre&gt;&#xA;&#xA;&lt;p&gt;When I run this on IE6, it returns 2 matches. The first containing the full  tag, the 2nd containing alert(&#x27;1&#x27;).&lt;/p&gt;&#xA;&#xA;&lt;p&gt;When I run it on &lt;a href=&quot;http://www.pagecolumn.com/tool/regtest.htm&quot; rel=&quot;noreferrer&quot;&gt;http://www.pagecolumn.com/tool/regtest.htm&lt;/a&gt; it gives me 2 results, each containing the script tags only.&lt;/p&gt;&#xA;
			</summary>
		</entry>
	</feed>`
	crawler, err := NewCrawler("https://stackoverflow.com", nil, &Opts{
		MaxDepth: 0,
		Parallel: 1,
		Limit:    0,
		Verbose:  false,
	})
	s.Require().NoError(err)
	res, urls := crawler.preProcessHTMLString(xmlString)
	s.Equal(1801, len(res))
	s.Equal(3, len(urls))
}

func (s *CrawlTestSuite) TestGetNextURLS() {
	html := `href="/x" href="something.com" href="google.com" href="google.com/x" href="/y"`
	crawler, err := NewCrawler("https://google.com", &ClientMock{}, &Opts{
		MaxDepth: 2,
		Parallel: 1,
		Limit:    0,
		Verbose:  false,
	})
	s.Require().NoError(err)
	result := crawler.GetNextURLs(html)
	s.Require().Equal(3, len(result))
	s.urlsContains(result, "https://google.com")
	s.urlsContains(result, "https://google.com/y")
	s.urlsContains(result, "https://google.com/x")
}

func (s *CrawlTestSuite) urlsContains(urls []string, url string) {
	found := false
	for _, foundURL := range urls {
		if foundURL == url {
			found = true
		}
	}
	s.True(found)
}

func TestCrawlTestSuite(t *testing.T) {
	suite.Run(t, new(CrawlTestSuite))
}

// GetMockHTTPResponse create an http response
func GetMockHTTPResponse(html []byte, statusCode int) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       ioutil.NopCloser(bytes.NewReader(html)),
	}
}
