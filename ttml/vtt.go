package ttml

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// ToVTT writes the TTML document to the specified writer in WebVTT format.
func (doc *Document) ToVTT(w io.Writer) error {
	// Write the WebVTT file signature.
	if _, err := w.Write([]byte("WEBVTT\n\n")); err != nil {
		return err
	}

	// shared styles
	webVTTStyles := doc.Head.Styling.ToWebVTT()
	if len(webVTTStyles) > 0 {
		if _, err := w.Write([]byte("STYLE\n")); err != nil {
			return err
		}
		if len(webVTTStyles) == 1 {
			// default cue style
			if _, err := w.Write([]byte(fmt.Sprintf("::cue { %s }\n", ttmlToWebVTTStyle(doc.Head.Styling.Styles[0])))); err != nil {
			} else {
				for _, style := range webVTTStyles {
					if _, err := w.Write([]byte(style + "\n")); err != nil {
						return err
					}
				}
			}
			if _, err := w.Write([]byte("\n")); err != nil {
				return err
			}
		}
	}

	// Iterate over the paragraphs.
	for _, division := range doc.Body.Divisions {
		// var currentStyle string

		// if the division has a region, apply the style as the default style
		if division.Region != "" {
			if _, err := w.Write([]byte(division.Region + "\n")); err != nil {
				return err
			}
		}

		var cue string
		for _, p := range division.Paragraphs {
			cue = p.Begin + " --> " + p.End + "\n"

			for _, span := range p.Span {
				cue += strings.TrimSpace(span.Text) + "\n"
			}

			if _, err := w.Write([]byte(cue + "\n")); err != nil {
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

func (s *Styling) ToWebVTT() []string {
	var styles []string
	for _, style := range s.Styles {
		if len(style.Attrs) < 2 {
			continue
		}
		webvttStyle := fmt.Sprintf("::cue(%s) { %s }", style.GetAttr("id"), ttmlToWebVTTStyle(style))
		styles = append(styles, webvttStyle)
	}
	return styles
}

// Convert a TTML style to a WebVTT style.
func ttmlToWebVTTStyle(style Style) string {
	var webVTTStyles []string
	for _, attr := range style.Attrs {
		if webVTTAttr, ok := ttmlToWebVTT[attr.Name.Local]; ok {
			webVTTStyles = append(webVTTStyles, fmt.Sprintf(" %s: %s", webVTTAttr, attr.Value))
		}
	}
	return strings.Join(webVTTStyles, ";")
}

// Map of TTML styles to WebVTT styles.
var ttmlToWebVTT = map[string]string{
	"color":                   "color",
	"fontFamily":              "font-family",
	"fontSize":                "font-size",
	"fontWeight":              "font-weight",
	"fontStyle":               "font-style",
	"backgroundColor":         "background-color",
	"backgroundImage":         "background-image",
	"backgroundPosition":      "background-position",
	"backgroundSize":          "background-size",
	"backgroundRepeat":        "background-repeat",
	"backgroundOrigin":        "background-origin",
	"backgroundClip":          "background-clip",
	"backgroundAttachment":    "background-attachment",
	"backgroundBlendMode":     "background-blend-mode",
	"opacity":                 "opacity",
	"unicodeBidi":             "unicode-bidi",
	"direction":               "direction",
	"writingMode":             "writing-mode",
	"textAlign":               "text-align",
	"textAlignLast":           "text-align-last",
	"textDecoration":          "text-decoration",
	"textDecorationColor":     "text-decoration-color",
	"textDecorationLine":      "text-decoration-line",
	"textDecorationStyle":     "text-decoration-style",
	"textIndent":              "text-indent",
	"textShadow":              "text-shadow",
	"textTransform":           "text-transform",
	"lineHeight":              "line-height",
	"letterSpacing":           "letter-spacing",
	"wordSpacing":             "word-spacing",
	"whiteSpace":              "white-space",
	"wordBreak":               "word-break",
	"wordWrap":                "word-wrap",
	"overflowWrap":            "overflow-wrap",
	"tabSize":                 "tab-size",
	"hyphens":                 "hyphens",
	"border":                  "border",
	"borderTop":               "border-top",
	"borderRight":             "border-right",
	"borderBottom":            "border-bottom",
	"borderLeft":              "border-left",
	"borderColor":             "border-color",
	"borderTopColor":          "border-top-color",
	"borderRightColor":        "border-right-color",
	"borderBottomColor":       "border-bottom-color",
	"borderLeftColor":         "border-left-color",
	"borderStyle":             "border-style",
	"borderTopStyle":          "border-top-style",
	"borderRightStyle":        "border-right-style",
	"borderBottomStyle":       "border-bottom-style",
	"borderLeftStyle":         "border-left-style",
	"borderWidth":             "border-width",
	"borderTopWidth":          "border-top-width",
	"borderRightWidth":        "border-right-width",
	"borderBottomWidth":       "border-bottom-width",
	"borderLeftWidth":         "border-left-width",
	"borderRadius":            "border-radius",
	"borderTopLeftRadius":     "border-top-left-radius",
	"borderTopRightRadius":    "border-top-right-radius",
	"borderBottomRightRadius": "border-bottom-right-radius",
	"borderBottomLeftRadius":  "border-bottom-left-radius",

	// Add more mappings here as needed.
}

type CueSetting struct {
	Align            string `webvtt:"align"`
	Line             string `webvtt:"line"`
	Position         string `webvtt:"position"`
	Size             string `webvtt:"size"`
	SnapToLines      string `webvtt:"snap-to-lines"`
	Vertical         string `webvtt:"vertical"`
	WritingMode      string `webvtt:"writing-mode"`
	BackgroundColor  string `webvtt:"background-color"`
	Color            string `webvtt:"color"`
	Font             string `webvtt:"font"`
	FontFamily       string `webvtt:"font-family"`
	FontSize         string `webvtt:"font-size"`
	FontStyle        string `webvtt:"font-style"`
	FontVariant      string `webvtt:"font-variant"`
	FontWeight       string `webvtt:"font-weight"`
	TextDecoration   string `webvtt:"text-decoration"`
	TextShadow       string `webvtt:"text-shadow"`
	WordSpacing      string `webvtt:"word-spacing"`
	LineHeight       string `webvtt:"line-height"`
	LetterSpacing    string `webvtt:"letter-spacing"`
	Padding          string `webvtt:"padding"`
	Animate          string `webvtt:"animate"`
	AnimateColor     string `webvtt:"animate-color"`
	AnimateDirection string `webvtt:"animate-direction"`
	AnimateFill      string `webvtt:"animate-fill"`
	AnimateName      string `webvtt:"animate-name"`
	AnimateRepeat    string `webvtt:"animate-repeat"`
	AnimateState     string `webvtt:"animate-state"`
	AnimateTiming    string `webvtt:"animate-timing"`
	Lang             string `webvtt:"lang"`
	Ruby             string `webvtt:"ruby"`
	RubyAlign        string `webvtt:"ruby-align"`
	RubyOverhang     string `webvtt:"ruby-overhang"`
	RubyPosition     string `webvtt:"ruby-position"`
	RubySpan         string `webvtt:"ruby-span"`
	Voice            string `webvtt:"voice"`
	VoiceDuration    string `webvtt:"voice-duration"`
	VoiceRate        string `webvtt:"voice-rate"`
	VoicePitch       string `webvtt:"voice-pitch"`
	VoiceRange       string `webvtt:"voice-range"`
	VoiceVolume      string `webvtt:"voice-volume"`
}
