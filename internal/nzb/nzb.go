package nzb

import (
	"encoding/xml"
	"io"
	"regexp"
)

type Nzb struct {
	XMLName xml.Name `xml:"nzb"`
	Files   []*File  `xml:"file"`
}

type File struct {
	XMLName  xml.Name   `xml:"file"`
	Poster   string     `xml:"poster,attr"`
	Date     uint32     `xml:"date,attr"`
	Subject  string     `xml:"subject:attr"`
	Groups   []string   `xml:"groups>group"`
	Segments []*Segment `xml:"segments>segment"`
}

func (f *File) FileName() string {
	re := regexp.MustCompile(`"([^"]+")`)
	match := re.FindStringSubmatch(f.Subject)
	if len(match) > 1 {
		return match[1]
	}
	return f.Subject
}

type Segment struct {
	XMLName xml.Name `xml:"segment"`
	Bytes   uint32   `xml:"bytes,attr"`
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
