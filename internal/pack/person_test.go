package pack

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"
)

// calendarAge is the age in whole years on a day, of a person born on a
// day: the count of birthdays up to and including that day.
func calendarAge(birth, day time.Time) int {
	age := day.Year() - birth.Year()
	if day.Month() < birth.Month() || day.Month() == birth.Month() && day.Day() < birth.Day() {
		age--
	}
	return age
}

func monthsOf(t time.Time) Months {
	m := 12*t.Year() + int(t.Month()) - 1
	return Months{m, m}
}

func yearOf(t time.Time) Months { return Months{12 * t.Year(), 12*t.Year() + 11} }

// TestAgesHoldEveryCalendarAge: for every day of birth and every day of a
// reading over some years, the calendar age lies from the lowest to the
// highest age that the months give. Both ends are reached, so the range is
// no wider than it must be. This holds for a birth and a reading that state
// a month, and for each that states a year alone.
func TestAgesHoldEveryCalendarAge(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	type key struct {
		birthYear, readingYear bool
		birth, reading         Months
	}
	lowest, highest := map[key]int{}, map[key]int{}
	// Births over two years, one a leap year, and readings over five.
	for b := day(1987, 1, 1); b.Before(day(1989, 1, 1)); b = b.AddDate(0, 0, 1) {
		for r := day(2006, 11, 1); r.Before(day(2011, 3, 1)); r = r.AddDate(0, 0, 3) {
			age := calendarAge(b, r)
			for _, k := range []key{
				{false, false, monthsOf(b), monthsOf(r)},
				{true, false, yearOf(b), monthsOf(r)},
				{false, true, monthsOf(b), yearOf(r)},
				{true, true, yearOf(b), yearOf(r)},
			} {
				lo, hi := Ages(k.reading, k.birth)
				if age < lo || age > hi {
					t.Fatalf("born %s, read %s: the age %d is outside %d to %d (%+v)", b.Format(time.DateOnly), r.Format(time.DateOnly), age, lo, hi, k)
				}
				if _, seen := lowest[k]; !seen || age < lowest[k] {
					lowest[k] = age
				}
				if age > highest[k] {
					highest[k] = age
				}
			}
		}
	}
	// A reading steps by three days, so a month has ten of them and a year
	// has more than a hundred: enough to reach each end, but for a month
	// that the reading's range cuts.
	for k := range lowest {
		lo, hi := Ages(k.reading, k.birth)
		cut := k.reading.First <= 12*2006+10 || k.reading.Last >= 12*2011+1
		if !cut && (lowest[k] != lo || highest[k] != hi) {
			t.Errorf("%+v: the ages seen are %d to %d, and Ages gives %d to %d", k, lowest[k], highest[k], lo, hi)
		}
	}
}

func TestAges(t *testing.T) {
	month := func(y, m int) Months { return Months{12*y + m - 1, 12*y + m - 1} }
	year := func(y int) Months { return Months{12 * y, 12*y + 11} }
	for _, tc := range []struct {
		reading, birth  Months
		lowest, highest int
	}{
		{month(2026, 4), month(1986, 5), 39, 39},
		{month(2026, 5), month(1986, 5), 39, 40}, // the month of the birthday
		{month(2026, 6), month(1986, 5), 40, 40},
		{month(2026, 3), year(1986), 39, 40}, // a year alone: every month of the year
		{month(2025, 12), year(1986), 38, 39},
		{year(2026), month(1986, 5), 39, 40},
		{year(2026), year(1986), 39, 40},
		{month(1986, 5), month(1986, 5), -1, 0},
		{month(1986, 4), month(1986, 5), -1, -1}, // before the birth: floor, not toward zero
		{month(1985, 5), month(1986, 5), -2, -1},
		{month(1984, 1), year(1986), -3, -2},
	} {
		if lo, hi := Ages(tc.reading, tc.birth); lo != tc.lowest || hi != tc.highest {
			t.Errorf("Ages(%v, %v) = %d, %d, want %d, %d", tc.reading, tc.birth, lo, hi, tc.lowest, tc.highest)
		}
	}
}

func TestBirthMonths(t *testing.T) {
	for _, tc := range []struct {
		value map[string]any
		want  Months
		ok    bool
	}{
		{map[string]any{"n": 1986.0}, Months{12 * 1986, 12*1986 + 11}, true},
		{map[string]any{"s": "1986"}, Months{12 * 1986, 12*1986 + 11}, true},
		{map[string]any{"s": "1986-05"}, Months{12*1986 + 4, 12*1986 + 4}, true},
		{map[string]any{"s": "1986-01"}, Months{12 * 1986, 12 * 1986}, true},
		{map[string]any{"s": "1986-12"}, Months{12*1986 + 11, 12*1986 + 11}, true},
		{map[string]any{"n": 1000.0}, Months{12000, 12011}, true},
		{map[string]any{"n": 9999.0}, Months{12 * 9999, 12*9999 + 11}, true},
		// No earliest year: an old year is a year of four digits.
		{map[string]any{"s": "0986"}, Months{12 * 986, 12*986 + 11}, true},
		{map[string]any{"n": 999.0}, Months{}, false},
		{map[string]any{"n": 10000.0}, Months{}, false},
		{map[string]any{"n": 1986.5}, Months{}, false},
		{map[string]any{"n": -1986.0}, Months{}, false},
		{map[string]any{"n": 1986.0, "unit": "a"}, Months{}, false},
		{map[string]any{"s": "1986", "unit": "a"}, Months{}, false},
		{map[string]any{"s": "1986-00"}, Months{}, false},
		{map[string]any{"s": "1986-13"}, Months{}, false},
		{map[string]any{"s": "1986-5"}, Months{}, false},
		{map[string]any{"s": "1986-05-17"}, Months{}, false},
		{map[string]any{"s": "86"}, Months{}, false},
		{map[string]any{"s": " 1986"}, Months{}, false},
		{map[string]any{"s": "1986\n"}, Months{}, false},
		{map[string]any{"s": "１９８６"}, Months{}, false}, // digits that are not ASCII
		{map[string]any{"s": "May 1986"}, Months{}, false},
		{map[string]any{"s": ""}, Months{}, false},
		{map[string]any{"b": true}, Months{}, false},
		{map[string]any{"blob": "sha256:00"}, Months{}, false},
		{map[string]any{}, Months{}, false},
		{nil, Months{}, false},
	} {
		if got, ok := BirthMonths(tc.value); ok != tc.ok || got != tc.want {
			t.Errorf("BirthMonths(%v) = %v, %v, want %v, %v", tc.value, got, ok, tc.want, tc.ok)
		}
	}
}

func TestSex(t *testing.T) {
	for value, want := range map[string]bool{"male": true, "female": true, "Male": false, "FEMALE": false, "male ": false, " female": false, "m": false, "": false, "intersex": false, "male\n": false, "fe​male": false} {
		if got, ok := Sex(map[string]any{"s": value}); ok != want || got != value {
			t.Errorf("Sex(%q) = %q, %v, want %v", value, got, ok, want)
		}
	}
	for _, value := range []map[string]any{{"n": 1.0}, {"b": true}, {"s": "male", "unit": "x"}, {}, nil} {
		if _, ok := Sex(value); ok {
			t.Errorf("Sex(%v) is taken", value)
		}
	}
}

func TestReadingMonths(t *testing.T) {
	for _, tc := range []struct {
		start, precision string
		want             Months
		ok               bool
	}{
		{"2026-11-01T08:53:00Z", "minute", Months{12*2026 + 10, 12*2026 + 10}, true},
		{"2026-11-01T00:00:00+13:00", "day", Months{12*2026 + 10, 12*2026 + 10}, true}, // as written: no offset is applied
		{"2026-11-30T23:59:59-11:00", "second", Months{12*2026 + 10, 12*2026 + 10}, true},
		{"2026-11-01T00:00:00Z", "month", Months{12*2026 + 10, 12*2026 + 10}, true},
		{"2026-11-01T00:00:00Z", "year", Months{12 * 2026, 12*2026 + 11}, true},
		{"2026-01-01T00:00:00Z", "year", Months{12 * 2026, 12*2026 + 11}, true},
		{"", "day", Months{}, false},
		{"2026", "year", Months{}, false},
		{"2026-13-01T00:00:00Z", "day", Months{}, false},
		{"yesterday", "day", Months{}, false},
	} {
		if got, ok := ReadingMonths(tc.start, tc.precision); ok != tc.ok || got != tc.want {
			t.Errorf("ReadingMonths(%q, %q) = %v, %v, want %v, %v", tc.start, tc.precision, got, ok, tc.want, tc.ok)
		}
	}
}

func TestMonthsAfter(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value string
		after bool
	}{{"2026", false}, {"2026-11", false}, {"2026-12", true}, {"2027", true}, {"2027-01", true}, {"1986", false}} {
		m, _ := BirthMonths(map[string]any{"s": tc.value})
		if m.After(now) != tc.after {
			t.Errorf("%s after 2026-11: %v, want %v", tc.value, m.After(now), tc.after)
		}
	}
}

// TestGroupsOfThePacksOnRecord: for each pack on record, each cohort has
// its sex and ages, they say what its words say, and no two cohorts hold
// the same person. So one sex and one age are in one cohort at most.
func TestGroupsOfThePacksOnRecord(t *testing.T) {
	for digest, k := range Known {
		if !slices.Equal(slices.Sorted(maps.Keys(k.groups)), slices.Sorted(maps.Keys(k.Cohorts))) {
			t.Errorf("%s: the groups are not those of the cohorts", digest[:12])
		}
		for id, g := range k.groups {
			words := fmt.Sprintf("%s, age %d-%d", g.Sex, g.From, g.To)
			if g.To < 0 {
				words = fmt.Sprintf("%s, age %d and over", g.Sex, g.From)
			}
			if k.Cohorts[id] != words || g.Sex != "male" && g.Sex != "female" || g.To >= 0 && g.To < g.From {
				t.Errorf("%s: the cohort %s holds %+v, and its words are %q", digest[:12], id, g, k.Cohorts[id])
			}
		}
		for _, sex := range []string{"male", "female"} {
			for age := -2; age <= 130; age++ {
				var holds []string
				for id, g := range k.groups {
					if g.Sex == sex && age >= g.From && (g.To < 0 || age <= g.To) {
						holds = append(holds, id)
					}
				}
				want := ""
				switch {
				case age >= 80:
					want = sex + "-80-over"
				case age >= 20:
					want = fmt.Sprintf("%s-%d-%d", sex, age/10*10, age/10*10+9)
				}
				if len(holds) > 1 || k.cohortOf(sex, age) != want {
					t.Errorf("%s: %s of %d is in %v, cohortOf gives %q, want %q", digest[:12], sex, age, holds, k.cohortOf(sex, age), want)
				}
			}
		}
		if k.cohortOf("", 30) != "" || k.cohortOf("Male", 30) != "" {
			t.Errorf("%s: a sex that the pack does not have is in a cohort", digest[:12])
		}
	}
}

func TestCohortAt(t *testing.T) {
	var k OnRecordPack
	for _, k = range Known {
		break
	}
	month := func(y, m int) Months { return Months{12*y + m - 1, 12*y + m - 1} }
	year := func(y int) Months { return Months{12 * y, 12*y + 11} }
	for _, tc := range []struct {
		name           string
		sex            string
		birth, reading Months
		cohort         string
		why            Why
	}{
		{"the two readings of DECISIONS 345", "male", month(1986, 5), month(2014, 6), "male-20-29", HasCohort},
		{"ten years on", "male", month(1986, 5), month(2024, 6), "male-30-39", HasCohort},
		{"the month before the 30th birthday", "male", month(1986, 5), month(2016, 4), "male-20-29", HasCohort},
		{"the month of the 30th birthday", "male", month(1986, 5), month(2016, 5), "", Edge},
		{"the month after it", "male", month(1986, 5), month(2016, 6), "male-30-39", HasCohort},
		{"a year alone, in the year of 30", "female", year(1986), month(2016, 1), "", Edge},
		{"a year alone, the last month of that year", "female", year(1986), month(2016, 12), "", Edge},
		{"a year alone, the year before", "female", year(1986), month(2015, 12), "female-20-29", HasCohort},
		{"a year alone, the year after", "female", year(1986), month(2017, 1), "female-30-39", HasCohort},
		{"a reading of a whole year, on the birthday of 30", "female", month(1986, 5), year(2016), "", Edge},
		{"a reading of a whole year", "female", month(1986, 5), year(2015), "female-20-29", HasCohort},
		{"the month of the 20th birthday", "male", month(1986, 5), month(2006, 5), "", MenuEdge},
		{"19", "male", month(1986, 5), month(2006, 4), "", NoCohort},
		{"20", "male", month(1986, 5), month(2006, 6), "male-20-29", HasCohort},
		{"a child", "female", month(1986, 5), month(1996, 5), "", NoCohort},
		{"the month of the birth", "female", month(1986, 5), month(1986, 5), "", NoCohort},
		{"the month before the birth", "female", month(1986, 5), month(1986, 4), "", BeforeBirth},
		{"the year before a birth of a year alone", "female", year(1986), month(1985, 12), "", BeforeBirth},
		{"the month of the 80th birthday", "male", month(1940, 3), month(2020, 3), "", Edge},
		{"80", "male", month(1940, 3), month(2020, 4), "male-80-over", HasCohort},
		{"an age with no cap", "male", year(1500), month(2020, 4), "male-80-over", HasCohort},
		{"a sex that the pack does not have", "intersex", month(1986, 5), month(2014, 6), "", NoCohort},
	} {
		if cohort, why := k.CohortAt(tc.sex, tc.birth, tc.reading); cohort != tc.cohort || why != tc.why {
			t.Errorf("%s: %q, %d, want %q, %d", tc.name, cohort, why, tc.cohort, tc.why)
		}
	}
	// A pack that bean has no record of has no groups, and so no cohort.
	if cohort, why := (OnRecordPack{}).CohortAt("male", month(1986, 5), month(2014, 6)); cohort != "" || why != NoCohort {
		t.Errorf("a pack with no groups: %q, %d", cohort, why)
	}
}
