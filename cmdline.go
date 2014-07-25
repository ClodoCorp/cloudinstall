package main

import (
	"fmt"
	"strings"
)

func cmdlineVar(key string) (ok bool, value string, err error) {
	err = fmt.Errorf("failed to find %s", key)
	ok = false

	for _, token := range cmdline {
		parts := strings.SplitN(token, "=", 2)
		if key == strings.TrimSpace(parts[0]) {
			ok = true
			err = nil
			if len(parts) == 1 {
				value = key
			} else {
				value = parts[1]
			}
			return
		}
	}
	return
}