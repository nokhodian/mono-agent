package linkedin

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

// jsonUnmarshal is a package-local helper to decode JSON strings from page.Eval results.
func jsonUnmarshal(s string, v interface{}) error {
	return json.Unmarshal([]byte(s), v)
}

// ListUserPosts scrapes posts from a LinkedIn personal profile or company page.
// profileURL: full URL (e.g. https://www.linkedin.com/in/username/ or /company/slug/)
// maxCount: max number of posts to collect
// activityType: "shares" | "all"
func (b *LinkedInBot) ListUserPosts(ctx context.Context, page *rod.Page, profileURL string, maxCount int, activityType string) ([]map[string]interface{}, error) {
	if profileURL == "" {
		return nil, fmt.Errorf("linkedin: profileURL is required")
	}
	if maxCount <= 0 {
		maxCount = 20
	}
	if activityType == "" {
		activityType = "all"
	}

	base := strings.TrimRight(profileURL, "/") + "/"
	var feedURL string
	if strings.Contains(base, "/company/") {
		feedURL = base + "posts/"
	} else {
		if activityType == "shares" {
			feedURL = base + "recent-activity/shares/"
		} else {
			feedURL = base + "recent-activity/all/"
		}
	}

	if err := page.Navigate(feedURL); err != nil {
		return nil, fmt.Errorf("linkedin: failed to navigate to activity feed: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("linkedin: activity feed did not load: %w", err)
	}
	time.Sleep(3 * time.Second)

	activityRe := regexp.MustCompile(`activity[-:](\d+)`)

	var allPosts []map[string]interface{}
	seen := map[string]bool{}
	noNewCount := 0

	for len(allPosts) < maxCount && noNewCount < 3 {
		res, err := page.Eval(`() => {
			const posts = [];
			const links = Array.from(document.querySelectorAll('a[href*="/posts/"], a[href*="/feed/update/"]'));
			for (const a of links) {
				const href = a.href || '';
				if (!href) continue;
				const article = a.closest('div[data-id], article, li');
				let author = '', authorUrl = '', textPreview = '', timestamp = '', likesCount = 0, commentsCount = 0, repostsCount = 0;
				if (article) {
					const nameEl = article.querySelector('span.feed-shared-actor__name, span.update-components-actor__name, a.update-components-actor__meta-link span[aria-hidden="true"]');
					if (nameEl) author = nameEl.innerText.trim();
					const authorLink = article.querySelector('a[href*="/in/"], a[href*="/company/"]');
					if (authorLink) authorUrl = authorLink.href;
					const textEl = article.querySelector('span.break-words, div.feed-shared-update-v2__description, div.update-components-text');
					if (textEl) textPreview = textEl.innerText.trim().slice(0, 200);
					const timeEl = article.querySelector('time, span[class*="time"]');
					if (timeEl) timestamp = timeEl.getAttribute('datetime') || timeEl.innerText.trim();
					const socialEl = article.querySelector('span.social-details-social-counts__reactions-count, button[aria-label*="reaction"]');
					if (socialEl) likesCount = parseInt(socialEl.innerText.replace(/[^0-9]/g,'')) || 0;
				}
				posts.push({ url: href, author, authorUrl, textPreview, timestamp, likesCount, commentsCount, repostsCount });
			}
			return JSON.stringify(posts);
		}`)
		if err != nil {
			return nil, fmt.Errorf("linkedin: failed to evaluate posts script: %w", err)
		}

		var rawPosts []map[string]interface{}
		if jsonErr := jsonUnmarshal(res.Value.Str(), &rawPosts); jsonErr != nil {
			return nil, fmt.Errorf("linkedin: failed to parse posts JSON: %w", jsonErr)
		}

		newThisScroll := 0
		for _, p := range rawPosts {
			rawURL, _ := p["url"].(string)
			if rawURL == "" {
				continue
			}
			m := activityRe.FindStringSubmatch(rawURL)
			if len(m) < 2 {
				// URL doesn't contain activity ID pattern — skip silently (LinkedIn has varied URL formats)
				continue
			}
			activityID := m[1]
			if seen[activityID] {
				continue
			}
			seen[activityID] = true
			newThisScroll++
			post := map[string]interface{}{
				"url":            rawURL,
				"activity_id":    activityID,
				"shortcode":      activityID,
				"text_preview":   p["textPreview"],
				"author":         p["author"],
				"author_url":     p["authorUrl"],
				"timestamp":      p["timestamp"],
				"likes_count":    p["likesCount"],
				"comments_count": p["commentsCount"],
				"reposts_count":  p["repostsCount"],
			}
			allPosts = append(allPosts, post)
			if len(allPosts) >= maxCount {
				break
			}
		}

		if newThisScroll == 0 {
			noNewCount++
		} else {
			noNewCount = 0
		}

		if len(allPosts) < maxCount {
			if _, scrollErr := page.Eval(`() => window.scrollBy(0, window.innerHeight * 2)`); scrollErr != nil {
			break
		}
			time.Sleep(2 * time.Second)
		}
	}

	return allPosts, nil
}

// ListPostComments scrapes comments (and optionally replies) from a LinkedIn post.
func (b *LinkedInBot) ListPostComments(ctx context.Context, page *rod.Page, postURL string, maxCount int, includeReplies bool) ([]map[string]interface{}, error) {
	if postURL == "" {
		return nil, fmt.Errorf("linkedin: postURL is required")
	}
	if maxCount <= 0 {
		maxCount = 50
	}

	if err := page.Navigate(postURL); err != nil {
		return nil, fmt.Errorf("linkedin: failed to navigate to post: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("linkedin: post page did not load: %w", err)
	}
	time.Sleep(3 * time.Second)

	// Click "Load more comments" up to 10 times.
	for i := 0; i < 10; i++ {
		loadMoreRes, loadMoreErr := page.Eval(`() => {
			const btns = Array.from(document.querySelectorAll('button'));
			const loadMore = btns.find(b => b.innerText.match(/load more comments/i) || b.innerText.match(/show more/i));
			if (loadMore) { loadMore.click(); return true; }
			return false;
		}`)
		if loadMoreErr != nil || loadMoreRes == nil || !loadMoreRes.Value.Bool() {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// If includeReplies, click "Load X replies" for each comment.
	if includeReplies {
		for i := 0; i < 20; i++ {
			repliesRes, repliesErr := page.Eval(`() => {
				const btns = Array.from(document.querySelectorAll('button'));
				const loadReplies = btns.find(b => b.innerText.match(/load \d+ repl/i) || b.innerText.match(/view repl/i));
				if (loadReplies) { loadReplies.click(); return true; }
				return false;
			}`)
			if repliesErr != nil || repliesRes == nil || !repliesRes.Value.Bool() {
				break
			}
			time.Sleep(1500 * time.Millisecond)
		}
	}

	res, err := page.Eval(`() => {
		const comments = [];
		const commentEls = document.querySelectorAll('article.comments-comment-item, div[class*="comment-item"]');
		for (const el of commentEls) {
			const id = el.getAttribute('data-id') || el.getAttribute('id') || '';
			const authorEl = el.querySelector('span.comments-post-meta__name, a.comments-post-meta__name span');
			const author = authorEl ? authorEl.innerText.trim() : '';
			const authorLinkEl = el.querySelector('a[href*="/in/"]');
			const authorUrl = authorLinkEl ? authorLinkEl.href : '';
			const textEl = el.querySelector('span.comments-comment-item__main-content, div.update-components-text');
			const text = textEl ? textEl.innerText.trim() : '';
			const timeEl = el.querySelector('time');
			const timestamp = timeEl ? (timeEl.getAttribute('datetime') || timeEl.innerText.trim()) : '';
			const likeEl = el.querySelector('button[aria-label*="Like"][aria-label*="comment"], span.social-details-social-counts__reactions-count');
			const likesCount = likeEl ? (parseInt((likeEl.innerText || '').replace(/[^0-9]/g, '')) || 0) : 0;
			const isReply = !!el.closest('div.comments-comment-item__nested-items, div[class*="nested"]');
			const parentEl = isReply ? el.parentElement.closest('article.comments-comment-item') : null;
			const parentId = parentEl ? (parentEl.getAttribute('data-id') || '') : '';
			comments.push({ id, author, authorUrl, text, timestamp, likesCount, replyCount: 0, parentId: parentId || null });
		}
		return JSON.stringify(comments);
	}`)
	if err != nil {
		return nil, fmt.Errorf("linkedin: failed to extract comments: %w", err)
	}

	var rawComments []map[string]interface{}
	if err := jsonUnmarshal(res.Value.Str(), &rawComments); err != nil {
		return nil, fmt.Errorf("linkedin: failed to parse comments JSON: %w", err)
	}

	var result []map[string]interface{}
	for i, c := range rawComments {
		if i >= maxCount {
			break
		}
		result = append(result, map[string]interface{}{
			"id":          c["id"],
			"author":      c["author"],
			"author_url":  c["authorUrl"],
			"text":        c["text"],
			"timestamp":   c["timestamp"],
			"likes_count": c["likesCount"],
			"reply_count": c["replyCount"],
			"parent_id":   c["parentId"],
		})
	}

	return result, nil
}
