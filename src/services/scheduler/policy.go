package scheduler

import (
	"fmt"
	"strings"
	"time"

	"hourglass-rejections-rpa/src/domain_models"
)

func intervalForHour(hour int) time.Duration {
	if hour >= 6 && hour < 22 {
		return 30 * time.Minute
	}

	return 2 * time.Hour
}

func buildNotificationSummary(rejections []domain.Rejection) string {
	sectionCounts, sectionOrder := summarizeSections(rejections)
	summary := fmt.Sprintf("%d rejections detected", len(rejections))
	if len(sectionOrder) == 0 {
		return summary
	}

	var details []string
	for _, section := range sectionOrder {
		details = append(details, fmt.Sprintf("%s (%d)", section, sectionCounts[section]))
	}

	return summary + ". Sections: " + strings.Join(details, ", ")
}

func summarizeSections(rejections []domain.Rejection) (map[string]int, []string) {
	counts := make(map[string]int)
	order := make([]string, 0, len(rejections))

	for _, rejection := range rejections {
		if _, seen := counts[rejection.Section]; !seen {
			order = append(order, rejection.Section)
		}
		counts[rejection.Section]++
	}

	return counts, order
}
