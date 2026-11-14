package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// values set accepts a JSON array from a file or stdin, preserving numeric spelling.
func cmdValues(c *Client, args []string) (int, error) {
	if len(args) != 3 || args[0] != "set" {
		return 1, fmt.Errorf("usage: quests values set note=N|quest=N|step=N|questline=N|template=N|workspace FILE|-")
	}
	owner := args[1]
	path := "/api/workspace/values"
	if owner != "workspace" {
		kind, id, ok := strings.Cut(owner, "=")
		segments := map[string]string{"note": "notes", "quest": "quests", "step": "steps", "questline": "questlines", "template": "templates"}
		n, err := strconv.ParseInt(id, 10, 64)
		if !ok || err != nil || n < 1 || segments[kind] == "" {
			return 1, fmt.Errorf("invalid owner")
		}
		path = "/api/" + segments[kind] + "/" + strconv.FormatInt(n, 10) + "/values"
	}
	var r io.Reader = os.Stdin
	if args[2] != "-" {
		file, err := os.Open(args[2])
		if err != nil {
			return 1, err
		}
		defer file.Close()
		r = file
	}
	raw, err := io.ReadAll(io.LimitReader(r, (256<<10)+1))
	if err != nil {
		return 1, err
	}
	var rows []json.RawMessage
	if len(raw) > 256<<10 || json.Unmarshal(raw, &rows) != nil || len(rows) == 0 {
		return 1, fmt.Errorf("нужен непустой JSON-массив до 256 КиБ")
	}
	response, err := c.Do(http.MethodPut, path, nil, map[string]any{"values": json.RawMessage(raw)})
	if err != nil {
		return 1, err
	}
	if c.AsJSON {
		Emit(true, response, "")
	} else {
		fmt.Printf("Записано переменных: %d\n", len(rows))
	}
	return 0, nil
}
