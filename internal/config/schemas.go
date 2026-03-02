package config

import "strings"

// ---------------------------------------------------------------------------
// Extraction schemas — ported from Python src/services/schemas.py
// ---------------------------------------------------------------------------
//
// Each schema is a map[string]interface{} with the structure:
//   {"fields": {"name": "...", "description": "...", "type": "array", "data": [...]}}
//
// The data array contains field descriptors:
//   {"name": "field_name", "description": "...", "type": "string"}

// field is a convenience builder for a single field descriptor.
func field(name, description string) map[string]interface{} {
	return map[string]interface{}{
		"name":        name,
		"description": description,
		"type":        "string",
	}
}

// schema builds a complete extraction schema.
func schema(name, description string, fields ...map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"fields": map[string]interface{}{
			"name":        name,
			"description": description,
			"type":        "array",
			"data":        fields,
		},
	}
}

// =====================================================================
// LinkedIn Schemas
// =====================================================================

var linkedinLoginSchema = schema("login_form", "Login form elements",
	field("username_input", "Input field for username/email"),
	field("password_input", "Input field for password"),
	field("login_btn", "Button to submit login form"),
)

var linkedinProfileInfoSchema = schema("profile_container", "Container for profile information",
	field("full_name", "The full name of the user on their profile page"),
	field("company_name", "The headline or company name displayed on the profile"),
	field("profile_picture", "The URL of the profile picture"),
	field("location", "User location"),
	field("followers_count", "Number of followers"),
)

var linkedinSendMessageSchema = schema("message_interaction", "Elements required to send a message",
	field("message_btn_icon", "The button to initiate a message (icon version)"),
	field("message_btn", "The button to initiate a message (text version)"),
	field("message_overlay_input", "The input field in the message overlay"),
	field("btn_message_close", "The button to close the message overlay"),
	field("send_button", "The button to send the message"),
	field("connect_btn", "The Connect button if message is not available"),
	field("more_btn", "The 'More' button to find Connect option"),
)

var linkedinKeywordSearchSchema = schema("people", "people search result",
	field("name", "The full name of the person."),
	field("position", "The current professional title and company."),
	field("profileUrl", "The URL to the person's detailed LinkedIn profile."),
	field("location", "The geographic location of the person."),
	field("followersCount", "The number of followers the person has."),
)

// =====================================================================
// X (Twitter) Schemas
// =====================================================================

var xLoginSchema = schema("login_form", "Login form elements",
	field("username_input", "Input field for username"),
	field("password_input", "Input field for password"),
	field("login_btn", "Button to submit login form"),
)

var xProfileInfoSchema = schema("profile_info", "User profile information",
	field("username", "The username (handle) starting with @"),
	field("full_name", "The display name of the user"),
	field("profile_image", "URL of the profile picture"),
	field("bio", "User biography text"),
	field("following_count", "Number of accounts following"),
	field("followers_count", "Number of followers"),
)

var xFollowersSchema = schema("followers_list", "List of followers",
	field("user_cell", "Container for a user in the list"),
	field("handle", "User handle (@username)"),
	field("profile_image", "Profile image URL"),
	field("full_name", "User's display name"),
	field("user_link", "Link to user profile"),
)

var xKeywordSearchMainPageSchema = schema("search_results", "Search results containing tweets",
	field("tweet_link", "Link to the tweet"),
	field("user_handle", "Handle of the tweet author"),
)

var xPostPageSchema = schema("tweet_info", "Information from a single tweet page",
	field("username", "Username of the tweet author"),
	field("full_name", "Display name of the user"),
	field("tweet_text", "Text content of the tweet"),
)

// =====================================================================
// Instagram Schemas
// =====================================================================

var instagramLoginSchema = schema("login_form", "Login form elements",
	field("username_input", "Input field for username"),
	field("password_input", "Input field for password"),
	field("login_btn", "Button to submit login form"),
)

var instagramKeywordSearchMainPageSchema = schema("search_results", "Grid of post results in the Instagram search or hashtag page",
	field("post_link", "The 'href' attribute of the link leading to a specific post or reel. Usually contains '/p/' or '/reel/'."),
)

var instagramPostPageSchema = schema("post_info", "Information from a single post page",
	field("username", "Username of the post author"),
	field("profile_link", "URL to the profile of the post creator (href)"),
	field("post_time", "Time the post was created"),
)

var instagramPostCommentingSchema = schema("post_commenting", "Elements for commenting on an Instagram post",
	field("comment_textarea", "The textarea or contenteditable div for typing a comment (aria-label contains 'Add a comment')."),
	field("post_button", "The 'Post' button that submits the comment."),
)

var instagramBulkReplyingSchema = schema("bulk_replying", "Elements for replying to Instagram DM conversations",
	field("conversation_list", "The container holding the list of DM conversations in the inbox."),
	field("conversation_item", "A single conversation entry in the inbox list (clickable link to thread)."),
	field("message_input", "The message input field (textbox or textarea) in a DM thread."),
	field("send_button", "The 'Send' button in a DM thread."),
)

var instagramProfileFetchSchema = schema("profile_fetch", "Elements for fetching followers/following lists from a profile",
	field("followers_link", "The clickable link or button that opens the followers list dialog."),
	field("following_link", "The clickable link or button that opens the following list dialog."),
	field("user_list_dialog", "The dialog/modal that contains the scrollable list of users."),
	field("user_item_link", "A single user entry link inside the followers/following dialog."),
)

var instagramPostInteractionSchema = schema("post_interaction", "Elements for interacting with a single Instagram post (like + comment)",
	field("like_button", "The Like button (or its parent) in the post's action bar section."),
	field("comment_textarea", "The textarea or contenteditable div for typing a comment."),
	field("post_button", "The 'Post' button that submits the comment."),
)

var instagramPublishContentSchema = schema("publish_content", "Elements for publishing a new Instagram post",
	field("create_button", "The 'New post' button or link that opens the create dialog."),
	field("file_input", "The file input element for uploading media."),
	field("next_button", "The 'Next' button to advance through the create flow."),
	field("caption_input", "The textarea or contenteditable div for writing a caption."),
	field("location_input", "The input field for adding a location tag."),
	field("share_button", "The 'Share' button that publishes the post."),
)

var instagramSearchResultsSchema = schema("search_results", "Elements on Instagram search/explore/tags pages",
	field("post_link", "A link to a post or reel in the search results grid (href contains '/p/' or '/reel/')."),
)

var instagramBulkFollowingSchema = schema("bulk_following", "Elements for following a user on their Instagram profile",
	field("follow_button", "The 'Follow' or 'Follow Back' button on the user's profile page."),
)

var instagramBulkUnfollowingSchema = schema("bulk_unfollowing", "Elements for unfollowing a user on their Instagram profile",
	field("following_button", "The 'Following' button on the user's profile page (visible when already following)."),
	field("unfollow_confirm_button", "The 'Unfollow' confirmation button inside the unfollow dialog."),
)

var instagramStoryViewingSchema = schema("story_viewing", "Elements for viewing stories on an Instagram profile",
	field("story_ring", "The clickable story ring (gradient-bordered profile picture) in the profile header."),
	field("story_next_button", "The 'Next' button or right-side tap target to advance to the next story."),
)

var instagramPostScrapingSchema = schema("post_scraping", "Elements for extracting data from an Instagram post page",
	field("caption_text", "The span or div containing the post caption text."),
	field("likes_count", "The element displaying the number of likes (e.g., '1,234 likes')."),
	field("comments_count", "The element showing the total number of comments (e.g., 'View all 56 comments')."),
	field("post_date", "The time element with a datetime attribute showing when the post was published."),
	field("media_element", "The main img or video element displaying the post content."),
)

var instagramCommentLikingSchema = schema("comment_liking", "Elements for liking a comment on an Instagram post",
	field("comment_container", "The container element wrapping a single comment (includes username link and like button)."),
	field("comment_like_button", "The standalone Like heart SVG/button next to a comment (not inside a section)."),
)

var instagramProfileInfoSchema = schema("profile_info", "Main container for user profile details (header and bio section)",
	field("full_name", "The display name of the user, typically found in a span inside the header."),
	field("profile_image", "The 'src' attribute of the profile picture 'img' tag."),
	field("post_count", "The number of posts (e.g., '1,234 posts'), found in the stats list."),
	field("followers_count", "The number of followers (e.g., '5.2M followers'), found in the stats list."),
	field("following_count", "The number of accounts followed by this user, found in the stats list."),
	field("bio", "The user's biography text found in the header/bio section."),
	field("website", "The clickable website link (href) in the bio section."),
	field("is_verified", "If a verification badge (blue check) element exists next to the username."),
	field("category", "The account category (e.g., 'Public Figure', 'Education') displayed under the name."),
)

// =====================================================================
// TikTok Schemas
// =====================================================================

var tiktokLoginSchema = schema("login_form", "Login form elements",
	field("username_input", "Input field for username"),
	field("password_input", "Input field for password"),
	field("login_btn", "Button to submit login form"),
)

var tiktokProfileInfoSchema = schema("profile_info", "TikTok profile details",
	field("username", "User unique ID"),
	field("full_name", "User display name"),
	field("followers_count", "Number of followers"),
	field("description", "User description/bio"),
	field("profile_pic", "Profile picture URL"),
	field("is_verified", "Verification badge element"),
)

var tiktokKeywordSearchMainPageSchema = schema("search_results", "Search results containing videos",
	field("video_link", "Link to the video"),
)

var tiktokPostPageSchema = schema("video_info", "Information from a single video page",
	field("username", "Username of the video author"),
	field("video_description", "Description of the video"),
)

// =====================================================================
// Schema registry
// =====================================================================

// schemas maps PLATFORM_ACTION keys to their extraction schemas.
// Aliases (e.g. PROFILE_PAGE -> PROFILE_INFO) are included as separate entries.
var schemas = map[string]map[string]interface{}{
	// LinkedIn
	"LINKEDIN_LOGIN":                    linkedinLoginSchema,
	"LINKEDIN_PROFILE_INFO":             linkedinProfileInfoSchema,
	"LINKEDIN_SEND_MESSAGE":             linkedinSendMessageSchema,
	"LINKEDIN_KEYWORD_SEARCH":           linkedinKeywordSearchSchema,
	"LINKEDIN_KEYWORD_SEARCH_MAIN_PAGE": linkedinKeywordSearchSchema,  // alias
	"LINKEDIN_PROFILE_PAGE":             linkedinProfileInfoSchema,    // alias

	// X (Twitter)
	"X_LOGIN":                        xLoginSchema,
	"X_PROFILE_INFO":                 xProfileInfoSchema,
	"X_FOLLOWERS":                    xFollowersSchema,
	"X_KEYWORD_SEARCH_MAIN_PAGE":     xKeywordSearchMainPageSchema,
	"X_POST_PAGE":                    xPostPageSchema,
	"X_PROFILE_PAGE":                 xProfileInfoSchema, // alias

	// Instagram
	"INSTAGRAM_LOGIN":                    instagramLoginSchema,
	"INSTAGRAM_KEYWORD_SEARCH_MAIN_PAGE": instagramKeywordSearchMainPageSchema,
	"INSTAGRAM_POST_PAGE":                instagramPostPageSchema,
	"INSTAGRAM_PROFILE_INFO":             instagramProfileInfoSchema,
	"INSTAGRAM_PROFILE_PAGE":             instagramProfileInfoSchema,    // alias
	"INSTAGRAM_POST_COMMENTING":          instagramPostCommentingSchema,
	"INSTAGRAM_BULK_REPLYING":            instagramBulkReplyingSchema,
	"INSTAGRAM_PROFILE_FETCH":            instagramProfileFetchSchema,
	"INSTAGRAM_POST_INTERACTION":         instagramPostInteractionSchema,
	"INSTAGRAM_USER_POSTS_INTERACTION":   instagramPostInteractionSchema, // alias → POST_INTERACTION
	"INSTAGRAM_PUBLISH_CONTENT":          instagramPublishContentSchema,
	"INSTAGRAM_SEARCH_RESULTS":           instagramSearchResultsSchema,
	"INSTAGRAM_BULK_FOLLOWING":           instagramBulkFollowingSchema,
	"INSTAGRAM_BULK_UNFOLLOWING":         instagramBulkUnfollowingSchema,
	"INSTAGRAM_STORY_VIEWING":            instagramStoryViewingSchema,
	"INSTAGRAM_POST_SCRAPING":            instagramPostScrapingSchema,
	"INSTAGRAM_COMMENT_LIKING":           instagramCommentLikingSchema,

	// TikTok
	"TIKTOK_LOGIN":                    tiktokLoginSchema,
	"TIKTOK_PROFILE_INFO":             tiktokProfileInfoSchema,
	"TIKTOK_KEYWORD_SEARCH_MAIN_PAGE": tiktokKeywordSearchMainPageSchema,
	"TIKTOK_POST_PAGE":                tiktokPostPageSchema,
	"TIKTOK_PROFILE_PAGE":             tiktokProfileInfoSchema, // alias
}

// GetSchema returns the extraction schema for the given platform, action, and
// optional configContext. It builds lookup keys like "INSTAGRAM_POST_PAGE" and
// falls back to "INSTAGRAM_LOGIN" if no context-specific match is found.
//
// Returns nil when no matching schema exists.
func GetSchema(platform, action, configContext string) map[string]interface{} {
	platform = strings.ToUpper(platform)
	action = strings.ToUpper(action)

	// If a configContext is provided, try PLATFORM_CONTEXT first.
	if configContext != "" {
		key := platform + "_" + strings.ToUpper(configContext)
		if s, ok := schemas[key]; ok {
			return s
		}
	}

	// Try PLATFORM_ACTION.
	key := platform + "_" + action
	if s, ok := schemas[key]; ok {
		return s
	}

	return nil
}
