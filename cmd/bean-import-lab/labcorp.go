package main

// The Labcorp patient report ("Enterprise Report Version 2.00"): what it
// prints and where. This is the one layout bean-import-lab reads today
// (DECISIONS 234). Everything here is a fact about that layout, taken from a
// real report (testdata/real-layout.txt); parse.go reads any report through
// it. A second layout gets a file of its own, and then a shared form for both.

import "regexp"

// system names the lab in each reading's source: provenance, not branding.
const system = "labcorp"

// collectionNote is how the report says it used the receipt date for the
// collection date, so that the time it prints may not be the draw time.
const collectionNote = "time of collection was not indicated"

var headerNames = [5]string{"Test", "Current Result and Flag", "Previous Result and Date", "Units", "Reference Interval"}

// commentLead is how far right of the test column a comment may start.
// Labcorp prints comments 43 columns right of it on every page, while the
// result column moves. A line between the test column and there is refused:
// a wrapped test name would sit there, and the reading above would be stored
// under half its name.
const commentLead = 40

var fieldSpecs = []fieldSpec{
	{"Patient ID", regexp.MustCompile(`^[!-~]+$`)},
	{"Age", regexp.MustCompile(`^\d{1,3}$`)},
	{"Account Number", regexp.MustCompile(`^[!-~]+$`)},
	{"Specimen ID", regexp.MustCompile(`^[!-~]+$`)},
	{"Sex", regexp.MustCompile(`^[A-Za-z]+$`)},
	{"Ordering Physician", nil},
	{"Date Collected", regexp.MustCompile(`^(\d\d/\d\d/\d{4})(?: (\d{4}) Local)?$`)},
	{"Date Received", regexp.MustCompile(`^\d\d/\d\d/\d{4}(?: \d{4} (?:Local|[A-Z]{2,4}))?$`)},
	{"Date Entered", regexp.MustCompile(`^\d\d/\d\d/\d{4}(?: \d{4} (?:Local|[A-Z]{2,4}))?$`)},
	{"Date Reported", regexp.MustCompile(`^(\d\d/\d\d/\d{4})(?: \d{4} [A-Z]{2,4})?$`)},
	{"Fasting", regexp.MustCompile(`^(?:Yes|No|Not Given)$`)},
	{"Ordered Items", regexp.MustCompile(`^[ -~]+$`)},
	{"Control ID", regexp.MustCompile(`^[!-~]+$`)},
	{"Alternate Control Number", regexp.MustCompile(`^[!-~]*$`)},
}

var (
	reDOB          = regexp.MustCompile(`^DOB: \d\d/\d\d/\d{4}$`)
	reCopyright    = regexp.MustCompile(`^©\d{4} Laboratory Corporation of America® Holdings$`)
	reRightsCall   = regexp.MustCompile(`^If you have received this document in error please call \d{3}-\d{3}-\d{4}$`)
	reConfidential = regexp.MustCompile(`^This document contains private and confidential health information(?: protected by state and federal law)?\.?$`)
	reCreated      = regexp.MustCompile(`^Date Created and Stored \d\d/\d\d/\d\d \d{4} [A-Z]{2,4} Final Report Page \d{1,4} of \d{1,4}$`)
)

const (
	rightsLine = "All Rights Reserved - Enterprise Report Version 2.00"
)

// boilerplate reports whether a line is one of the page header and footer
// lines, whole.
func boilerplate(toks []token) bool {
	first := toks[0]
	switch {
	case len(toks) == 3 && first.start == 0 && safeText(first.text) && reDOB.MatchString(toks[1].text) && toks[2].text == "Patient Report":
		return true // the patient's name; it is not read
	case first.start == 0 && reCopyright.MatchString(first.text):
		return len(toks) == 1 || len(toks) == 2 && reConfidential.MatchString(toks[1].text)
	case first.start == 0 && first.text == rightsLine:
		return len(toks) == 1 || len(toks) == 2 && reRightsCall.MatchString(toks[1].text)
	case len(toks) == 1 && first.start > 0 && reCreated.MatchString(first.text):
		return true
	}
	return false
}

var endHeadings = map[string]bool{"Disclaimer": true, "Icon Legend": true, "Performing Labs": true}

// legendItems are the words of the Icon Legend line; its icons do not come
// through pdftotext.
var legendItems = map[string]bool{"Out of Reference Range": true, "Critical or Alert": true}

var (
	reLab     = regexp.MustCompile(`^(\d\d): [A-Z0-9]{2,3} - ([^,]+), [ -~]+$`)
	reLabLike = regexp.MustCompile(`^\d\d: `)
)

var detailsNames = []string{"Patient Details", "Physician Details", "Specimen Details"}

// notPerformedPhrases are how a lab says it has no value for a test. The
// list is short on purpose: a row with no value and any other comment is
// refused.
var notPerformedPhrases = []string{
	"test not performed", "not performed",
	"test cancelled", "test canceled", "cancelled", "canceled",
}
