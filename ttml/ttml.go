package ttml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
)

// ttml is a package to parse, merge and save ttml files.
// It's more complicated than it should be because of the way the Go handles namespaces.
// I had to write custom marshalling methods to get the right output (with the right namespaces)
// without having to duplicate the whole structs (one for reading and one for writing).
// The ttml spec is here: https://www.w3.org/TR/ttml2/

type Document struct {
	XMLName xml.Name   `xml:"tt"`
	Attrs   []xml.Attr `xml:",attr"`
	Lang    string     `xml:"lang,attr,omitempty"`
	Head    Head       `xml:"head"`
	Body    Body       `xml:"body"`
}

func (d *Document) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name.Local = "tt"

	// add the namespaces
	d.Attrs = append(d.Attrs, xml.Attr{Name: xml.Name{Local: "xmlns"}, Value: "http://www.w3.org/ns/ttml"})
	d.Attrs = append(d.Attrs, xml.Attr{Name: xml.Name{Local: "xmlns:tts"}, Value: "http://www.w3.org/ns/ttml#styling"})
	d.Attrs = append(d.Attrs, xml.Attr{Name: xml.Name{Local: "xmlns:ttp"}, Value: "http://www.w3.org/ns/ttml#parameter"})
	d.Attrs = append(d.Attrs, xml.Attr{Name: xml.Name{Local: "xmlns:ttm"}, Value: "http://www.w3.org/ns/ttml#metadata"})

	oldLang := d.Lang
	if d.Lang != "" {
		d.Attrs = append(d.Attrs, xml.Attr{Name: xml.Name{Local: "xml:lang"}, Value: d.Lang})
	}
	d.Lang = ""

	err := e.EncodeElement(*d, start)
	d.Lang = oldLang

	// write the document
	return err
}

type Style struct {
	Attrs []xml.Attr `xml:",attr"`
}
type Division struct {
	Region     string      `xml:"region,attr"`
	Paragraphs []Paragraph `xml:"p"`
}

type Head struct {
	Metadata Metadata `xml:"metadata"`
	Styling  Styling  `xml:"styling"`
	Layout   Layout   `xml:"layout"`
}

type Metadata struct {
	Title       string `xml:"title"`
	Description string `xml:"desc"`
	Copyright   string `xml:"copyright"`
}

type Styling struct {
	Styles []Style `xml:"style"`
}

func (s *Style) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {

	s.Attrs = start.Attr

	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		switch element := token.(type) {
		case xml.StartElement:
			fmt.Println("****", element.Name.Local)

		}
	}
	return nil
}

func (s *Style) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Attr = s.Attrs

	for i, attr := range start.Attr {

		if attr.Name.Space != "" {
			start.Attr[i].Name.Space = ""
			switch attr.Name.Space {
			case "http://www.w3.org/ns/ttml#styling":
				start.Attr[i].Name.Local = "tts:" + attr.Name.Local
			case "http://www.w3.org/XML/1998/namespace":
				start.Attr[i].Name.Local = "xml:" + attr.Name.Local
			}
		}
	}

	if err := e.EncodeToken(start); err != nil {
		return err
	}

	return e.EncodeToken(start.End())
}

type Layout struct {
	Regions []Region `xml:"region"`
}

type Region struct {
	ID    string `xml:"id,attr"`
	Style string `xml:"style,attr"`
}

func (r Region) MarshalXML(e *xml.Encoder, start xml.StartElement) error {

	start.Attr = []xml.Attr{}
	if r.Style != "" {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "style"},
			Value: r.Style,
		})
	}
	if r.ID != "" {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "xml:id"},
			Value: r.ID,
		})
	}
	if err := e.EncodeToken(start); err != nil {
		return err
	}

	return e.EncodeToken(start.End())
}

type Body struct {
	Divisions []Division `xml:"div"`
}

type Paragraph struct {
	Begin  string `xml:"begin,attr"`
	End    string `xml:"end,attr"`
	Region string `xml:"region,attr"`
	ID     string `xml:"id,attr"`
	Role   string `xml:"role,attr"`

	Text string `xml:",innerxml"`

	Span []Span `xml:"span"`
	Br   string `xml:"br"`
}

func (p *Paragraph) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name.Local = "p"
	start.Attr = []xml.Attr{}

	if p.ID != "" {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "xml:id"},
			Value: p.ID,
		})
	}

	if p.Role != "" {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "ttm:role"},
			Value: p.Role,
		})
	}

	if p.Begin != "" {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "begin"},
			Value: p.Begin,
		})
	}

	if p.End != "" {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "end"},
			Value: p.End,
		})
	}

	if p.Region != "" {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "region"},
			Value: p.Region,
		})
	}

	if err := e.EncodeToken(start); err != nil {
		return err
	}

	for _, span := range p.Span {
		if err := e.EncodeElement(span, xml.StartElement{Name: xml.Name{Local: "span"}}); err != nil {
			return err
		}
	}

	return e.EncodeToken(start.End())
}

type Span struct {
	Text      string `xml:",chardata"`
	Color     string `xml:"color,attr"`
	TextAlign string `xml:"textAlign,attr"`
}

func (s Span) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name.Local = "span"
	start.Attr = []xml.Attr{}

	if s.Color != "" {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "tts:color"},
			Value: s.Color,
		})
	}
	if s.TextAlign != "" {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "tts:textAlign"},
			Value: s.TextAlign,
		})
	}

	if err := e.EncodeToken(start); err != nil {
		return err
	}

	if err := e.EncodeToken(xml.CharData("\n" + strings.TrimSpace(s.Text) + "\n")); err != nil {
		return err
	}

	return e.EncodeToken(start.End())
}

func Open(filename string) (*Document, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return New(data)
}

func New(data []byte) (*Document, error) {

	decoder := xml.NewDecoder(bytes.NewReader(data))
	var doc Document
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// Write writes the TTML document to the specified writer.
func (doc *Document) Write(w io.Writer) error {
	// Write the XML declaration.
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return err
	}

	// Create a new XML encoder.
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}

	return enc.Flush()
}

// Merge does an in-memory merge of 2 TTML documents
func (doc *Document) Merge(other *Document) *Document {
	if doc.Head.Metadata.Title != other.Head.Metadata.Title {
		doc.Head.Metadata.Title += " " + other.Head.Metadata.Title
	}
	if doc.Head.Metadata.Description != other.Head.Metadata.Description {
		doc.Head.Metadata.Description += " " + other.Head.Metadata.Description
	}
	if doc.Head.Metadata.Copyright != other.Head.Metadata.Copyright {
		doc.Head.Metadata.Copyright += " " + other.Head.Metadata.Copyright
	}
	if len(doc.Body.Divisions) == 0 {
		doc.Body.Divisions = other.Body.Divisions
		return doc
	}
	if len(doc.Body.Divisions) == 0 {
		doc.Body.Divisions = append(doc.Body.Divisions, Division{Paragraphs: []Paragraph{}})
	}
	if len(doc.Body.Divisions[0].Paragraphs) == 0 {
		doc.Body.Divisions[0].Paragraphs = append(doc.Body.Divisions[0].Paragraphs, Paragraph{})
	}

	if len(other.Body.Divisions) == 0 {
		return doc
	}

	doc.Body.Divisions[0].Paragraphs = append(doc.Body.Divisions[0].Paragraphs, other.Body.Divisions[0].Paragraphs...)

	return doc
}

// MergeFromData does an in-memory merge of 2 TTML documents, using the byte representation of a second doc
// Note that the timestamps aren't realigned, so the second doc should be a subset of the first.
func (doc *Document) MergeFromData(data []byte) *Document {
	var ttml Document
	if err := xml.Unmarshal(data, &ttml); err != nil {
		return nil
	}

	return doc.Merge(&ttml)
}

// ToVTT writes the TTML document to the specified writer in WebVTT format.
func (doc *Document) ToVTT(w io.Writer) error {
	// Write the WebVTT file signature.
	if _, err := w.Write([]byte("WEBVTT\n\n")); err != nil {
		return err
	}

	// Iterate over the paragraphs.
	for _, division := range doc.Body.Divisions {
		for _, p := range division.Paragraphs {
			// Write the start and end times.
			if _, err := w.Write([]byte(p.Begin + " --> " + p.End + "\n")); err != nil {
				return err
			}
			for _, span := range p.Span {
				// fmt.Println("->", strings.TrimSpace(span.Text))
				if _, err := w.Write([]byte(strings.TrimSpace(span.Text) + "\n")); err != nil {
					return err
				}
			}
			if _, err := w.Write([]byte("\n")); err != nil {
				return err
			}
		}
	}
	return nil
}

// SaveAsVTT saves the TTML document to the specified file in WebVTT format.
// This is a convenience method that calls ToVTT internally.
func (doc *Document) SaveAsVTT(outPath string) error {
	// Open the output file.
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Convert the document to WebVTT and write it to the output file.
	if err := doc.ToVTT(f); err != nil {
		return err
	}
	return nil
}
