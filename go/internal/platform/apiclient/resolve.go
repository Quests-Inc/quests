package apiclient

import (
	"fmt"
	"strconv"
	"strings"
)

func IsNone(raw string) bool {
	s := strings.ToLower(strings.TrimSpace(raw))
	return s == "" || s == "none" || s == "-" || s == "нет" || s == "null"
}

func (c *Client) ResolveCategoryID(raw string) (*int64, error) {
	if IsNone(raw) {
		return nil, nil
	}
	raw = strings.TrimSpace(raw)
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return &n, nil
	}
	items, err := c.Get("/api/categories", nil)
	if err != nil {
		return nil, err
	}
	list, err := DecodeList(items)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(raw)
	var matches []int64
	for _, cat := range list {
		id, _ := AsInt64(cat["id"])
		slug := strings.ToLower(AsString(cat["slug"]))
		label := strings.ToLower(AsString(cat["label"]))
		if slug == needle || label == needle || strings.Contains(label, needle) {
			matches = append(matches, id)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("раздел %q не найден", raw)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("несколько разделов для %q; укажи id", raw)
	}
	return &matches[0], nil
}

func (c *Client) ResolveQuestlineID(raw string) (*int64, error) {
	if IsNone(raw) {
		return nil, nil
	}
	raw = strings.TrimSpace(raw)
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return &n, nil
	}
	items, err := c.Get("/api/questlines", nil)
	if err != nil {
		return nil, err
	}
	list, err := DecodeList(items)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(raw)
	var matches []int64
	for _, line := range list {
		id, _ := AsInt64(line["id"])
		title := strings.ToLower(AsString(line["title"]))
		if title == needle || strings.Contains(title, needle) {
			matches = append(matches, id)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("квестлайн %q не найден", raw)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("несколько квестлайнов для %q; укажи id", raw)
	}
	return &matches[0], nil
}
