// Package domain contains the core business logic and data structures.
package domain

import (
	"errors"
)

var (
	// ErrAuthenticationFailed is returned when login fails.
	ErrAuthenticationFailed = errors.New("authentication failed")

	// ErrNotAuthenticated is returned when an operation requires authentication but the user is not logged in.
	ErrNotAuthenticated = errors.New("not authenticated")

	// ErrSectionNotFound is returned when a requested section is not found.
	ErrSectionNotFound = errors.New("section not found")

	// ErrScrapingFailed is returned when a scraping operation fails.
	ErrScrapingFailed = errors.New("scraping failed")

	// ErrStorageFailed is returned when a storage operation fails.
	ErrStorageFailed = errors.New("storage operation failed")

	// ErrNotificationFailed is returned when a notification fails to send.
	ErrNotificationFailed = errors.New("notification failed")
)
