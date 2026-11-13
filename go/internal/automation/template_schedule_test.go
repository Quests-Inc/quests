package automation

import (
	"testing"
	"time"
)

func TestTemplateCalendarDates(t *testing.T) {
	for _, tc := range []struct {
		name, date string
		template   TemplateRow
		want       bool
	}{
		{"monthly15", "2026-02-15", TemplateRow{Freq: "monthly", Monthdays: "15,30"}, true},
		{"no30February", "2026-02-28", TemplateRow{Freq: "monthly", Monthdays: "30"}, false},
		{"noMarchCarry", "2026-03-02", TemplateRow{Freq: "monthly", Monthdays: "30"}, false},
		{"April30", "2026-04-30", TemplateRow{Freq: "monthly", Monthdays: "30,31"}, true},
		{"leap29", "2028-02-29", TemplateRow{Freq: "monthly", Monthdays: "29"}, true},
		{"unknown", "2026-02-15", TemplateRow{Freq: "bad"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := time.Parse("2006-01-02", tc.date)
			if got := TemplateDueOn(tc.template, d); got != tc.want {
				t.Fatalf("due=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestAnchoredCycleAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	tpl := TemplateRow{Freq: "cycle", CycleLength: 4, CycleStart: "2026-03-27", CycleDays: "1,2"}
	for i, want := range []bool{false, true, true, false, false, true, true, false, false, true} {
		d := time.Date(2026, 3, 26+i, 12, 0, 0, 0, loc)
		if got := TemplateDueOn(tpl, d); got != want {
			t.Errorf("%s due=%v want=%v", d, got, want)
		}
	}
	tpl.CycleStart = "2026-10-23"
	for i, want := range []bool{true, true, false, false, true, true} {
		d := time.Date(2026, 10, 23+i, 12, 0, 0, 0, loc)
		if TemplateDueOn(tpl, d) != want {
			t.Errorf("fall DST %s", d)
		}
	}
}

func TestScheduleValidation(t *testing.T) {
	for _, body := range []map[string]any{
		{"freq": "monthly", "monthdays": "0"}, {"freq": "monthly", "monthdays": "32"}, {"freq": "monthly", "monthdays": ""},
		{"freq": "cycle", "cycle_start": "2026-02-30"}, {"freq": "cycle", "cycle_start": "2026-01-01", "cycle_length": 0},
		{"freq": "cycle", "cycle_start": "2026-01-01", "cycle_length": 2.5},
		{"freq": "cycle", "cycle_start": "2026-01-01", "cycle_length": "4"},
		{"freq": "cycle", "cycle_start": "2026-01-01", "cycle_days": "5"}, {"freq": "unknown"},
	} {
		if validateTemplateSchedule(body) == nil {
			t.Errorf("accepted %+v", body)
		}
	}
	for _, body := range []map[string]any{{}, {"freq": "weekly"}, {"freq": "monthly", "monthdays": "15,31"}, {"freq": "cycle", "cycle_start": "2026-01-01", "cycle_length": 4, "cycle_days": "1,2"}} {
		if err := validateTemplateSchedule(body); err != nil {
			t.Errorf("%+v: %v", body, err)
		}
	}
}
