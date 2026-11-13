package automation

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func scheduleDays(raw string, max int) (map[int]bool, error) {
	out := map[int]bool{}
	for _, part := range strings.Split(raw, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 1 || n > max {
			return nil, fmt.Errorf("выберите дни от 1 до %d", max)
		}
		out[n] = true
	}
	return out, nil
}

func validateTemplateSchedule(body map[string]any) error {
	freq := asStringDef(body["freq"], "daily")
	switch freq {
	case "daily", "weekly": // Preserve legacy weekday defaults and interpretation.
		return nil
	case "monthly":
		_, err := scheduleDays(asStringDef(body["monthdays"], "1"), 31)
		return err
	case "cycle":
		raw := body["cycle_length"]
		if raw != nil {
			switch raw.(type) {
			case int, int64, float64:
			default:
				return fmt.Errorf("длина цикла должна быть целым числом")
			}
		}
		length := asInt(raw, 4)
		if f, ok := raw.(float64); ok && (math.IsNaN(f) || f != float64(length)) {
			return fmt.Errorf("длина цикла должна быть целым числом")
		}
		if length < 1 || length > 366 {
			return fmt.Errorf("длина цикла: от 1 до 366 дней")
		}
		if _, err := time.Parse("2006-01-02", asStringDef(body["cycle_start"], "")); err != nil {
			return fmt.Errorf("укажите начальную дату цикла в формате YYYY-MM-DD")
		}
		_, err := scheduleDays(asStringDef(body["cycle_days"], "1,2"), length)
		return err
	default:
		return fmt.Errorf("неизвестный период: %s", freq)
	}
}

// TemplateDueOn matches a local calendar date. UTC civil midnights are used only
// for counting dates, so DST's 23/25-hour days never move an anchored cycle.
func TemplateDueOn(t TemplateRow, local time.Time) bool {
	switch strings.ToLower(t.Freq) {
	case "", "daily":
		return true
	case "weekly":
		day := (int(local.Weekday()) + 6) % 7
		anyValid := false
		for _, part := range strings.Split(t.Weekdays, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err == nil && n >= 0 && n <= 6 {
				anyValid = true
				if n == day {
					return true
				}
			}
		}
		return !anyValid // Existing templates with an empty weekday list ran daily.
	case "monthly":
		days, err := scheduleDays(t.Monthdays, 31)
		return err == nil && days[local.Day()]
	case "cycle":
		if t.CycleLength < 1 || t.CycleLength > 366 {
			return false
		}
		start, err := time.Parse("2006-01-02", t.CycleStart)
		if err != nil {
			return false
		}
		date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
		if date.Before(start) {
			return false
		}
		offset := int((date.Unix() - start.Unix()) / 86400)
		days, err := scheduleDays(t.CycleDays, t.CycleLength)
		return err == nil && days[offset%t.CycleLength+1]
	default:
		return false
	}
}
