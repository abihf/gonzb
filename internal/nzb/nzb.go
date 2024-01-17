package nzb

import (
	"encoding/xml"
	"io"
	"regexp"
	"strconv"
)

type Nzb struct {
	XMLName xml.Name `xml:"nzb"`
	Files   []*File  `xml:"file"`
}

type File struct {
	XMLName  xml.Name   `xml:"file"`
	Poster   string     `xml:"poster,attr"`
	Date     uint32     `xml:"date,attr"`
	Subject  string     `xml:"subject,attr"`
	Groups   []string   `xml:"groups>group"`
	Segments []*Segment `xml:"segments>segment"`
}

var fileNameMatcher = regexp.MustCompile(`"([^"]+)"`)

func (f *File) FileName() string {
	match := fileNameMatcher.FindStringSubmatch(f.Subject)
	if len(match) > 1 {
		return match[1]
	}
	return f.Subject
}

var fileSizeMatcher = regexp.MustCompile(`\(.*?\).*?(\d+)(\s|$)`)

func (f *File) FileSize() (int64, bool) {
	match := fileSizeMatcher.FindStringSubmatch(f.Subject)
	if len(match) > 1 {
		s, err := strconv.ParseInt(match[1], 10, 64)
		return s, err == nil
	}
	return 0, false
}

type Segment struct {
	XMLName xml.Name `xml:"segment"`
	Bytes   int64    `xml:"bytes,attr"`
	Number  uint32   `xml:"number,attr"`
	ID      string   `xml:",chardata"`
}

func Parse(reader io.ReadCloser) (*Nzb, error) {
	var nzb Nzb
	err := xml.NewDecoder(reader).Decode(&nzb)
	if err != nil {
		return nil, err
	}
	return &nzb, nil
}
