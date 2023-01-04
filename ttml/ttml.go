package ttml

import (
	"bytes"
	"encoding/xml"
	"io"
	"io/ioutil"
)

// Document represents a TTML document.
type Document struct {
	XMLName xml.Name   `xml:"tt"`
	Attrs   []xml.Attr `xml:",attr"`
	// XmlNS string `xml:"xmlns:,attr"`
	Lang string `xml:"lang,attr"`
	Head Head   `xml:"head"`
	Body Body   `xml:"body"`
}

// Head represents the head of a TTML document.
type Head struct {
	Metadata Metadata `xml:"metadata"`
	Styling  Styling  `xml:"styling"`
	Layout   Layout   `xml:"layout"`
}

// Metadata represents the metadata of a TTML document.
type Metadata struct {
	Title       string `xml:"title"`
	Description string `xml:"desc"`
	Copyright   string `xml:"copyright"`
}

// Styling represents the styling of a TTML document.
type Styling struct {
	Styles []Style `xml:"style"`
}

// Style represents a style of a TTML document.
type Style struct {
	ID              string `xml:"id,attr"`
	BackgroundColor string `xml:"backgroundColor,attr"`
	DisplayAlign    string `xml:"displayAlign,attr"`
	Extent          string `xml:"extent,attr"`
	FontFamily      string `xml:"fontFamily,attr"`
	FontSize        string `xml:"fontSize,attr"`
	Origin          string `xml:"origin,attr"`
	TextAlign       string `xml:"textAlign,attr"`
	TextOutline     string `xml:"textOutline,attr"`
}

// Layout represents the layout of a TTML document.
type Layout struct {
	Regions []Region `xml:"region"`
}

// Region represents a region of a TTML document.
type Region struct {
	ID    string `xml:"id,attr"`
	Style string `xml:"style,attr"`
}

// Body represents the body of a TTML document.
type Body struct {
	Divisions []Division `xml:"div"`
}

// Division represents a division of a TTML document.
type Division struct {
	P []Paragraph `xml:"p"`
}

// Paragraph represents a paragraph of a TTML document.
type Paragraph struct {
	Begin string `xml:"begin,attr"`
	End   string `xml:"end,attr"`
	Text  string `xml:",innerxml"`
}

func Open(filename string) (*Document, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return New(data)
}

func New(data []byte) (*Document, error) {

	// Create a new XML decoder.
	dec := xml.NewDecoder(bytes.NewReader(data))

	// Iterate over the tokens, extracting the attributes and child elements of the tt element.
	var doc Document
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		switch element := token.(type) {
		case xml.StartElement:
			if element.Name.Local == "tt" {
				doc.Attrs = element.Attr
				if err := dec.DecodeElement(&doc, &element); err != nil {
					return nil, err
				}
			} else if element.Name.Local == "head" {
				if err := dec.DecodeElement(&doc.Head, &element); err != nil {
					return nil, err
				}
			} else if element.Name.Local == "body" {
				if err := dec.DecodeElement(&doc.Body, &element); err != nil {
					return nil, err
				}
			}
		}
	}
	return &doc, nil
}

func (doc *Document) Write(w io.Writer) error {
	// Write the XML declaration.
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return err
	}

	// Create a new XML encoder.
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")

	// Start the tt element.
	// Write the start element for the tt element, including all of the attributes.

	// There is a weird issue with the xml package where it will not encode xmlns attributes properly
	// hence this giant HACK :(
	nAttrs := make([]xml.Attr, len(doc.Attrs))
	for i, a := range doc.Attrs {
		if a.Name.Space == "xmlns" {
			a.Name.Local = "xmlns:" + a.Name.Local
			a.Name.Space = ""
		}
		nAttrs[i] = a
	}
	if err := enc.EncodeToken(xml.StartElement{Name: xml.Name{Local: "tt"}, Attr: nAttrs}); err != nil {
		return err
	}

	// Encode the head element.
	if err := enc.Encode(doc.Head); err != nil {
		return err
	}

	// Encode the body element.
	if err := enc.Encode(doc.Body); err != nil {
		return err
	}

	// End the tt element.
	if err := enc.EncodeToken(xml.EndElement{Name: xml.Name{Local: "tt"}}); err != nil {
		return err
	}

	// Flush the encoder.
	return enc.Flush()
}

// in memory merge of 2 TTML documents
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
		doc.Body.Divisions = append(doc.Body.Divisions, Division{P: []Paragraph{}})
	}
	if len(doc.Body.Divisions[0].P) == 0 {
		doc.Body.Divisions[0].P = append(doc.Body.Divisions[0].P, Paragraph{})
	}

	if len(other.Body.Divisions) == 0 {
		return doc
	}

	doc.Body.Divisions[0].P = append(doc.Body.Divisions[0].P, other.Body.Divisions[0].P...)

	return doc
}

func (doc *Document) MergeFromData(data []byte) *Document {
	var ttml Document
	if err := xml.Unmarshal(data, &ttml); err != nil {
		return nil
	}

	return doc.Merge(&ttml)
}

// // Write the merged TTML document to a string.
// var b bytes.Buffer
// if err := xml.NewEncoder(&b).Encode(doc); err != nil {
// 	fmt.Println(err)
// 	return
// }
// s := b.String()
