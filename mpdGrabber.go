package mpdgrabber

import (
	"log"
	"os"
)

var Debug = false
var Logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

func strPtrtoS(s *string) string {
	if s == nil {
		return "unknown"
	}
	return *s
}

func int64PtrToI(d *int64) int {
	if d == nil {
		return 0
	}
	return int(*d)
}
