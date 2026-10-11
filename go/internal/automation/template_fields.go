package automation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

func parseTagIDsFromBody(body map[string]any) ([]int64, bool, error) {
	if raw, ok := body["tag_ids"]; ok {
		ids, err := coerceInt64Slice(raw)
		return ids, true, err
	}
	if raw, ok := body["tags"]; ok {
		// Accept [{id:…}, …] from GetTemplate round-trip / CopyTemplate.
		b, _ := json.Marshal(raw)
		var tags []struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(b, &tags); err != nil {
			return nil, true, err
		}
		ids := make([]int64, 0, len(tags))
		for _, t := range tags {
			if t.ID != 0 {
				ids = append(ids, t.ID)
			}
		}
		return ids, true, nil
	}
	return nil, false, nil
}

func coerceInt64Slice(raw any) ([]int64, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var ids []int64
	if err := json.Unmarshal(b, &ids); err == nil {
		return ids, nil
	}
	var floats []float64
	if err := json.Unmarshal(b, &floats); err != nil {
		return nil, fmt.Errorf("tag_ids must be an array of integers")
	}
	out := make([]int64, len(floats))
	for i, f := range floats {
		out[i] = int64(f)
	}
	return out, nil
}

func scanTemplate(row rowScanner) (TemplateRead, error) {
	var id int64
	var title, desc, freq, weekdays, tz, sig, emitMode string
	var pinned, enabled, sortOrder, automated int
	var dur sql.NullInt64
	var deadline, ewStart, ewEnd, emitPoolCommand, emitLimits, emitRunOn sql.NullString
	var emitChance float64
	var catID, lineID sql.NullInt64
	var created, updated sql.NullString
	var cSlug, cLabel, cColor sql.NullString
	var lTitle, lColor, lIcon, lCustom, lUpdated sql.NullString
	var lID sql.NullInt64
	err := row.Scan(
		&id, &title, &desc, &pinned, &sortOrder, &dur, &freq, &weekdays,
		&enabled, &tz, &deadline, &sig, &emitMode, &emitChance,
		&ewStart, &ewEnd, &emitPoolCommand, &emitLimits, &emitRunOn,
		&catID, &lineID,
		&created, &updated, &automated,
		&cSlug, &cLabel, &cColor, &lTitle, &lColor, &lIcon, &lCustom, &lUpdated, &lID,
	)
	if err != nil {
		return TemplateRead{}, err
	}
	tr := TemplateRead{
		ID: id, Title: title, Description: desc, Pinned: pinned > 0, OverlayHidden: pinned < 0, SortOrder: sortOrder,
		Freq: freq, Weekdays: weekdays, Enabled: enabled != 0, Timezone: tz,
		Significance: sig, EmitMode: emitMode, EmitChance: emitChance, Automated: automated != 0,
	}
	if emitLimits.Valid && strings.TrimSpace(emitLimits.String) != "" {
		var lim map[string]any
		if json.Unmarshal([]byte(emitLimits.String), &lim) == nil {
			tr.EmitLimits = lim
		}
	}
	if dur.Valid {
		v := int(dur.Int64)
		tr.DurationSecs = &v
	}
	if deadline.Valid {
		tr.DeadlineTime = &deadline.String
	}
	if ewStart.Valid {
		tr.EmitWindowFrom = &ewStart.String
	}
	if ewEnd.Valid {
		tr.EmitWindowTo = &ewEnd.String
	}
	if emitPoolCommand.Valid {
		tr.EmitPoolCmd = &emitPoolCommand.String
	}
	if emitRunOn.Valid && strings.TrimSpace(emitRunOn.String) != "" {
		v := strings.TrimSpace(emitRunOn.String)
		tr.EmitRunOn = &v
	}
	if catID.Valid {
		tr.CategoryID = &catID.Int64
	}
	if lineID.Valid {
		tr.QuestlineID = &lineID.Int64
	}
	if created.Valid {
		if t, e := timeutil.ParseFlexible(created.String); e == nil {
			tr.CreatedAt = timeutil.ToUTCISO(&t)
		}
	}
	if updated.Valid {
		if t, e := timeutil.ParseFlexible(updated.String); e == nil {
			tr.UpdatedAt = timeutil.ToUTCISO(&t)
		}
	}
	if cSlug.Valid {
		tr.CategorySlug = &cSlug.String
	}
	if cLabel.Valid {
		tr.CategoryLabel = &cLabel.String
	}
	if cColor.Valid {
		tr.CategoryColor = &cColor.String
	}
	if lTitle.Valid {
		tr.QuestlineTitle = &lTitle.String
	}
	if lColor.Valid {
		tr.QuestlineColor = &lColor.String
	}
	if lIcon.Valid {
		tr.QuestlineIcon = &lIcon.String
	}
	if lCustom.Valid && lCustom.String != "" && lID.Valid {
		u := "/api/questlines/" + itoa64(lID.Int64) + "/icon"
		if lUpdated.Valid {
			if t, e := timeutil.ParseFlexible(lUpdated.String); e == nil {
				if iso := timeutil.ToUTCISO(&t); iso != nil {
					u += "?v=" + *iso
				}
			}
		}
		tr.QuestlineIconURL = &u
	}
	return tr, nil
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func asStringDef(v any, def string) string {
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

func asStringPtr(v any) *string {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		return &s
	}
	return nil
}

func asBool(v any, def bool) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	default:
		return def
	}
}

func asInt(v any, def int) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	default:
		return def
	}
}

func asIntPtr(v any) *int {
	if v == nil {
		return nil
	}
	n := asInt(v, 0)
	return &n
}

func asI64Ptr(v any) *int64 {
	if v == nil {
		return nil
	}
	n := int64(asInt(v, 0))
	return &n
}

func asFloat(v any, def float64) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	default:
		return def
	}
}

func itoa64(n int64) string {
	return strconv.FormatInt(n, 10)
}

// emitLimitsColumn turns the emit_limits value of a create/update body (a JSON
// object, or the same as a string) into the column value: validated, NULL when
// empty. Updates merge over the stored object, so it may come back as a map.
func emitLimitsColumn(v any) (any, error) {
	var raw string
	switch t := v.(type) {
	case nil:
		return nil, nil
	case string:
		raw = t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return nil, fmt.Errorf("emit_limits: %v", err)
		}
		raw = string(b)
	}
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "null" || strings.TrimSpace(raw) == "{}" {
		return nil, nil
	}
	if _, err := ParseEmitLimits(raw); err != nil {
		return nil, err
	}
	return raw, nil
}
