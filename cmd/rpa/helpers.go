package main

import (
	"strconv"
	"strings"
)

func parseChatID(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}

func parseWhitelist(s string) []int64 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var result []int64
	for _, part := range strings.Split(s, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil {
			result = append(result, id)
		}
	}
	return result
}
