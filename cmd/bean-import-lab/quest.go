package main

// The Quest Diagnostics patient report: what it prints and where. It is read
// by word position (bbox.go, questread.go), not from -layout text, whose
// columns drift from page to page and row to row on this report. Every fact
// here was measured on a real report; testdata/quest-layout.html keeps that
// report's word positions with every name, id, date, value, flag and
// comment replaced (DECISIONS 235). Positions are in points from the top
// left of the page.

import (
	"regexp"
	"strings"
)

// questSystem names the lab in each reading's source: provenance.
const questSystem = "quest"

// questTrademark is the last line of every page.
const questTrademark = "Quest, Quest Diagnostics, the associated logo and all associated Quest Diagnostics marks are the trademarks of Quest Diagnostics."

// A results table. Each header sets the columns of the rows under it: a
// column starts at the x of its first header word, and a word belongs to
// the column whose start it is at or right of. On the real report the main
// header's columns sit at 240, 316, 403 and 571 on every page, and the
// alternate header's at 283, 354 and 565, with its cells printed a few
// points right of the header word.
type questTable struct {
	name    string
	header  []string
	columns []int // header word index where each column after the name starts
	value   []int // the columns that hold a value: In Range and Out Of Range, or Result
	ref     int   // the Reference Range column
	lab     int   // the Lab column
	// Rows start at these indents; a deeper indent is a row inside the
	// group above it. Comments start at commentFrom or further right.
	indents     []float64
	commentFrom float64
}

var (
	questMain = &questTable{
		name:   "Test Name, In Range, Out Of Range, Reference Range, Lab",
		header: []string{"Test", "Name", "In", "Range", "Out", "Of", "Range", "Reference", "Range", "Lab"},
		// columns: 0 name, 1 In Range, 2 Out Of Range, 3 Reference Range, 4 Lab
		columns: []int{2, 4, 7, 9}, value: []int{1, 2}, ref: 3, lab: 4,
		indents: []float64{28.9, 39.7, 50.6}, commentFrom: 56,
	}
	// The alternate table, seen under a section title such as
	// "Endocrinology": one value column, and rows at the margin of the
	// table rather than under "Test Name".
	questAlt = &questTable{
		name:   "Test Name, Result, Reference Range, Lab",
		header: []string{"Test", "Name", "Result", "Reference", "Range", "Lab"},
		// columns: 0 name, 1 Result, 2 Reference Range, 3 Lab
		columns: []int{2, 3, 5}, value: []int{1}, ref: 2, lab: 3,
		indents: []float64{26.3}, commentFrom: 30,
	}
	questTables = []*questTable{questMain, questAlt}
)

// indentTolerance is how far, in points, a row may start from its indent.
const indentTolerance = 1.0

// questFlags are the flags the Out Of Range column (or the Result column)
// prints after a value.
var questFlags = map[string]bool{"H": true, "L": true, "HH": true, "LL": true, "A": true}

// seeNote is how the report says a value is in the comment under it: the
// test is kept with no value and that comment.
const seeNote = "SEE NOTE:"

// The block at the top of each page: three columns that start at these
// words. Only the specimen column and the Fasting field are read.
var questBlockHeading = []string{"Patient", "Information", "Specimen", "Information", "Client", "Information"}

// questSpecimenFields are what the specimen column holds, with the shape of
// each value. Every line there must be one of them.
var questSpecimenFields = []struct {
	label []string
	value *regexp.Regexp
}{
	{[]string{"Specimen:"}, regexp.MustCompile(`^[!-~]+$`)},
	{[]string{"Requisition:"}, regexp.MustCompile(`^[!-~]+$`)},
	{[]string{"Lab", "Ref", "#:"}, regexp.MustCompile(`^[!-~]+$`)},
	{[]string{"Collected:"}, reQuestTime},
	{[]string{"Received:"}, reQuestTime},
	{[]string{"Reported:"}, reQuestTime},
}

// reQuestTime is "MM/DD/YYYY / HH:MM ZONE", the time and zone optional.
var reQuestTime = regexp.MustCompile(`^(\d\d/\d\d/\d{4})(?: / (\d\d):(\d\d) ([A-Z]{2,5}))?$`)

// questPatientLabels are the fields of the patient column. None is read but
// Fasting: the column also holds the patient's name, on a line of its own.
var questPatientLabels = [][]string{
	{"DOB:"}, {"AGE:"}, {"Gender:"}, {"Fasting:"}, {"Phone:"}, {"Patient", "ID:"}, {"Health", "ID:"},
}

var questFasting = map[string]string{"Y": "Yes", "N": "No"}

// questZones are the US zone abbreviations the report prints after a time,
// and their offsets from UTC in seconds. Any other zone leaves only the day.
var questZones = map[string]int{
	"EST": -5 * 3600, "EDT": -4 * 3600, "CST": -6 * 3600, "CDT": -5 * 3600,
	"MST": -7 * 3600, "MDT": -6 * 3600, "PST": -8 * 3600, "PDT": -7 * 3600,
	"AKST": -9 * 3600, "AKDT": -8 * 3600, "HST": -10 * 3600,
}

var (
	reQuestCode  = regexp.MustCompile(`^[A-Z0-9]{2,4}$`)
	reQuestPhone = regexp.MustCompile(`^[0-9().-]{7,20}$`)
	reQuestPage  = regexp.MustCompile(`^[1-9]\d{0,3}$`)
	reEndnote    = regexp.MustCompile(`^[1-9]\d?$`)
	// A reference range at the start of the Reference Range column:
	// 50-195, 0.40-4.50, <150, >280, > OR = 40, < or = 12.9.
	reQuestRange      = regexp.MustCompile(`^` + questRange + `(?: |$)`)
	reQuestRangeWhole = regexp.MustCompile(`^` + questRange + `$`)
	// A comment that gives the range of a test whose column has none.
	reQuestRefLine = regexp.MustCompile(`^Reference [Rr]ange:? (.+)$`)
)

const questRange = `(?:\d+(?:\.\d+)?-\d+(?:\.\d+)?|[<>]=?\d+(?:\.\d+)?|[<>] (?:OR|or) = \d+(?:\.\d+)?)`

// questDetect reports whether -layout text is a Quest report: its
// trademark line and a results column header.
func questDetect(text string) bool {
	trademark, header := false, false
	for _, l := range splitLines(text) {
		f := strings.Join(strings.Fields(l), " ")
		switch {
		case f == questTrademark:
			trademark = true
		case f == strings.Join(questMain.header, " ") || f == strings.Join(questAlt.header, " "):
			header = true
		}
	}
	return trademark && header
}
