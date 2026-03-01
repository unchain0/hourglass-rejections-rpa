package rpa

import (
	"context"
	"os"
	"strings"

	"github.com/chromedp/chromedp"
)

// Browser handles the browser automation setup and lifecycle.
type Browser struct {
	ctx    context.Context
	cancel context.CancelFunc
	alloc  context.CancelFunc
}

// NewBrowser creates a new Browser instance.
func NewBrowser() *Browser {
	return &Browser{}
}

// Setup initializes the chromedp allocator and context with required flags.
func (b *Browser) Setup() error {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	// Support debug mode (headless=false when DEBUG=true)
	if strings.ToLower(os.Getenv("DEBUG")) == "true" {
		opts = append(opts, chromedp.Flag("headless", false))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	b.alloc = allocCancel

	ctx, cancel := chromedp.NewContext(allocCtx)
	b.ctx = ctx
	b.cancel = cancel

	return nil
}

// Close cleans up the browser context and allocator.
func (b *Browser) Close() {
	if b.cancel != nil {
		b.cancel()
	}
	if b.alloc != nil {
		b.alloc()
	}
}

// Context returns the browser context.
func (b *Browser) Context() context.Context {
	return b.ctx
}
