package hourglass

import (
	"fmt"
	"time"

	"hourglass-rejections-rpa/src/domain_models"
)

type rejectionTitleResolver func(notificationType string, notification Notification) string
type rejectionDateFormatter func(date, language string) string
type rejectionUserResolver func(userID int) string

func mapDeclinedNotifications(
	notifications []Notification,
	sectionName string,
	timestamp time.Time,
	language string,
	resolveUser rejectionUserResolver,
	resolveTitle rejectionTitleResolver,
	formatDate rejectionDateFormatter,
) []domain.Rejection {
	var rejections []domain.Rejection
	seen := make(map[string]bool)

	for _, notif := range notifications {
		if notif.Status != "declined" {
			continue
		}

		rejection := domain.Rejection{
			Section:   sectionName,
			Who:       resolveUser(notif.Assignee),
			What:      resolveTitle(notif.Type, notif),
			When:      formatDate(notif.Date, language),
			Timestamp: timestamp,
		}

		key := rejectionIdentityKey(rejection)
		if seen[key] {
			continue
		}

		seen[key] = true
		rejections = append(rejections, rejection)
	}

	return rejections
}

func rejectionIdentityKey(rejection domain.Rejection) string {
	return fmt.Sprintf("%s|%s|%s|%s", rejection.Section, rejection.Who, rejection.What, rejection.When)
}
