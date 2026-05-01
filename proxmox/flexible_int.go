package proxmox

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type JsonInt int

func (v *JsonInt) UnmarshalJSON(data []byte) error {
	if string(data) == "null" || string(data) == `""` {
		*v = 0
		return nil
	}

	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*v = JsonInt(n)
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("FlexibleInt: expected int or string, got %s", string(data))
	}

	if s == "" {
		*v = 0
		return nil
	}

	parsed, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("FlexibleInt: invalid int string %q: %w", s, err)
	}

	*v = JsonInt(parsed)
	return nil
}
