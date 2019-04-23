package page_test

import (
	"clamber/page"
	"github.com/stretchr/testify/assert"
	"testing"
)

type (
	FetchUrlTest struct {
		Url       string
		httpError bool
	}

	RelativeUrlTest struct {
		Url        string
		IsRelative bool
	}

	ParseUrlTest struct {
		Url         string
		ExpectedUrl string
	}
)

var FetchUrlTests = []FetchUrlTest{
	{"http://example.edu", false},
	{"HTTP://EXAMPLE.EDU", false},
	{"https://www.exmaple.com", true},
	{"ftp://example.edu/file.txt", true},
	{"//cdn.example.edu/lib.js", true},
	{"/myfolder/test.txt", true},
	{"test", true},
}

var RelativeUrlTests = []RelativeUrlTest{
	{"http://example.edu", false},
	{"HTTP://EXAMPLE.EDU", false},
	{"https://www.exmaple.com", false},
	{"ftp://example.edu/file.txt", false},
	{"//cdn.example.edu/lib.js", false},
	{"/myfolder/test.txt", true},
	{"test", true},
}

var ParseUrlTests = []ParseUrlTest{
	{"/myfolder/test", "http://example.edu/myfolder/test"},
	{"test", "http://example.edu/test"},
	{"test/", "http://example.edu/test"},
	{"test#jg380gj39v", "http://example.edu/test"},
}

func TestFetchUrlsHttpError(t *testing.T) {
	for _, test := range FetchUrlTests {
		thisPage := page.Page{Url: test.Url}
		_, err := thisPage.FetchChildPages()
		assert.Equal(t, test.httpError, err != nil)
	}
}

// simulate a page with a given number of links, and check that the number of links
// on the page reflect the number of links returned.
// Another test case is checking correct errors from parseHtml
// Would also test IsRelativeHtml regexs (very important to test Regex)

func TestIsRelativeUrl(t *testing.T) {
	for _, test := range RelativeUrlTests {
		assert.Equal(t, test.IsRelative, page.IsRelativeUrl(test.Url))
	}
}

func TestParseRelativeUrl(t *testing.T) {
	rootUrl := "http://example.edu"
	for _, test := range ParseUrlTests {
		absoluteUrl := page.ParseRelativeUrl(rootUrl, test.Url)
		assert.Equal(t, test.ExpectedUrl, absoluteUrl.String())
	}
}
