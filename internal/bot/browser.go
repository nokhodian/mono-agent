package bot

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"
	"github.com/rs/zerolog"
)

// BrowserPool manages a pool of browser instances and their associated pages.
// It enforces a maximum number of concurrent browsers and provides stealth
// page creation with anti-detection measures.
type BrowserPool struct {
	mu       sync.Mutex
	browsers []*rod.Browser
	pages    map[string]*rod.Page
	maxSize  int
	headless bool
	logger   zerolog.Logger
}

// NewBrowserPool creates a new BrowserPool with the given settings.
// maxSize controls the maximum number of concurrent browser instances.
// When headless is true, browsers launch without a visible window.
func NewBrowserPool(maxSize int, headless bool, logger zerolog.Logger) *BrowserPool {
	if maxSize <= 0 {
		maxSize = 1
	}
	return &BrowserPool{
		browsers: make([]*rod.Browser, 0, maxSize),
		pages:    make(map[string]*rod.Page),
		maxSize:  maxSize,
		headless: headless,
		logger:   logger,
	}
}

// AcquirePage returns an existing stealth page for the given sessionID, or
// creates a new one. Each sessionID maps to exactly one page; calling
// AcquirePage twice with the same ID returns the same page.
func (bp *BrowserPool) AcquirePage(sessionID string) (*rod.Page, error) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Return existing page if already allocated for this session.
	if page, ok := bp.pages[sessionID]; ok {
		bp.logger.Debug().Str("session", sessionID).Msg("reusing existing page")
		return page, nil
	}

	browser, err := bp.getOrCreateBrowser()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire browser: %w", err)
	}

	// Create a stealth page that evades common bot-detection scripts.
	page, err := stealth.Page(browser)
	if err != nil {
		return nil, fmt.Errorf("failed to create stealth page: %w", err)
	}

	bp.pages[sessionID] = page
	bp.logger.Info().Str("session", sessionID).Msg("created new stealth page")
	return page, nil
}

// ReleasePage closes and removes the page associated with sessionID.
// It is safe to call with an unknown sessionID (no-op).
func (bp *BrowserPool) ReleasePage(sessionID string) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	page, ok := bp.pages[sessionID]
	if !ok {
		return
	}

	if err := page.Close(); err != nil {
		bp.logger.Warn().Err(err).Str("session", sessionID).Msg("error closing page")
	} else {
		bp.logger.Debug().Str("session", sessionID).Msg("page closed")
	}

	delete(bp.pages, sessionID)
}

// getOrCreateBrowser returns an existing browser if capacity allows, or
// launches a new one with anti-detection flags. Must be called with bp.mu held.
func (bp *BrowserPool) getOrCreateBrowser() (*rod.Browser, error) {
	// Reuse the last browser if it has not exceeded a reasonable page count.
	// For simplicity, each browser is reused until we hit maxSize browsers.
	if len(bp.browsers) > 0 {
		last := bp.browsers[len(bp.browsers)-1]
		// Try to reuse the most recent browser. If pages are balanced across
		// browsers, we could add smarter logic later.
		pages, err := last.Pages()
		if err == nil && len(pages) < 10 {
			return last, nil
		}
	}

	if len(bp.browsers) >= bp.maxSize {
		// Pool is full; reuse the first browser (round-robin).
		return bp.browsers[0], nil
	}

	// Launch a new browser with stealth / anti-detection flags.
	l := launcher.New().
		Headless(bp.headless).
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-infobars").
		Set("disable-dev-shm-usage").
		Set("no-sandbox").
		Set("disable-setuid-sandbox").
		Set("disable-gpu").
		Set("disable-extensions").
		Set("disable-popup-blocking").
		Set("disable-background-networking").
		Set("disable-background-timer-throttling").
		Set("disable-backgrounding-occluded-windows").
		Set("disable-renderer-backgrounding").
		Set("disable-component-update").
		Set("disable-default-apps").
		Set("disable-hang-monitor").
		Set("disable-prompt-on-repost").
		Set("disable-sync").
		Set("disable-translate").
		Set("metrics-recording-only").
		Set("no-first-run").
		Set("safebrowsing-disable-auto-update")

	controlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to browser: %w", err)
	}

	bp.browsers = append(bp.browsers, browser)
	bp.logger.Info().
		Int("pool_size", len(bp.browsers)).
		Bool("headless", bp.headless).
		Msg("launched new browser instance")

	return browser, nil
}

// Close shuts down every page and browser managed by the pool.
func (bp *BrowserPool) Close() {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Close all pages first.
	for sid, page := range bp.pages {
		if err := page.Close(); err != nil {
			bp.logger.Warn().Err(err).Str("session", sid).Msg("error closing page during shutdown")
		}
	}
	bp.pages = make(map[string]*rod.Page)

	// Close all browsers.
	for i, browser := range bp.browsers {
		if err := browser.Close(); err != nil {
			bp.logger.Warn().Err(err).Int("index", i).Msg("error closing browser during shutdown")
		}
	}
	bp.browsers = bp.browsers[:0]
	bp.logger.Info().Msg("browser pool closed")
}

// CleanupZombies kills orphaned Chrome/Chromium processes that may have been
// left behind by a previous crash. This is a best-effort operation; errors
// from the kill commands are logged but not returned.
func (bp *BrowserPool) CleanupZombies() {
	bp.logger.Info().Msg("cleaning up zombie Chrome processes")

	var cmds []*exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmds = append(cmds,
			exec.Command("taskkill", "/F", "/IM", "chrome.exe"),
			exec.Command("taskkill", "/F", "/IM", "chromium.exe"),
		)
	case "darwin":
		// On macOS, pkill returns exit code 1 when no process is matched,
		// which is harmless.
		cmds = append(cmds,
			exec.Command("pkill", "-f", "Google Chrome for Testing"),
			exec.Command("pkill", "-f", "Chromium"),
		)
	default:
		// Linux and other Unix-like systems.
		cmds = append(cmds,
			exec.Command("pkill", "-f", "chrome"),
			exec.Command("pkill", "-f", "chromium"),
		)
	}

	for _, cmd := range cmds {
		if out, err := cmd.CombinedOutput(); err != nil {
			// pkill/taskkill returns non-zero when no matching process exists;
			// this is expected and not worth warning about.
			bp.logger.Debug().
				Str("cmd", cmd.String()).
				Str("output", string(out)).
				Err(err).
				Msg("zombie cleanup command finished with error (may be expected)")
		} else {
			bp.logger.Info().
				Str("cmd", cmd.String()).
				Msg("zombie cleanup command succeeded")
		}
	}
}
