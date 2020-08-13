package main

import (
	"bytes"
	"flag"
	"fmt"
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
	s.Equal("https://google.com/x", result[0])
	s.Equal("https://google.com", result[1])
	s.Equal("https://google.com/y", result[2])
	fmt.Println(result)
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
