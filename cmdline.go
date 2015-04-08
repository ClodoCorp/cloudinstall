package main

import "strings"

func cmdlineVar(key string) (ok bool, value string) {
	ok = false

	for _, token := range cmdline {
		parts := strings.SplitN(token, "=", 2)
		if key == strings.TrimSpace(parts[0]) {
			ok = true
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

func cmdlineBool(key string) (ok bool) {
	for _, token := range cmdline {
		if token == key {
			return true
		}
	}
	return false
}
