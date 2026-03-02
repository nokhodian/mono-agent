# Monoes Actions Reference

A plain-English guide to every automation action, what it does, what it needs, and a suggested name that makes its purpose immediately obvious.

---

## Instagram

### BULK_FOLLOWING
**Suggested name: `Follow Users`**

Goes through a list of Instagram profile URLs and follows each account. Skips profiles that are already followed. Waits a few seconds between each follow to stay under the radar.

| | |
|---|---|
| **Needs** | A list of profile URLs |
| **Does** | Visits each profile → clicks Follow |
| **Output** | Number of profiles followed |

---

### BULK_UNFOLLOWING
**Suggested name: `Unfollow Users`**

The reverse of Follow Users. Goes through a list of profile URLs and unfollows each one. Skips accounts you're not currently following.

| | |
|---|---|
| **Needs** | A list of profile URLs |
| **Does** | Visits each profile → clicks Following → confirms Unfollow |
| **Output** | Number of profiles unfollowed |

---

### BULK_MESSAGING
**Suggested name: `Send DMs`**

Sends a direct message to a list of users. Each user gets the same message text. Useful for outreach campaigns.

| | |
|---|---|
| **Needs** | A list of profile URLs, a message text |
| **Does** | Visits each profile → opens DM → types and sends the message |
| **Output** | Number of messages sent |

---

### BULK_REPLYING
**Suggested name: `Auto-Reply to DMs`**

Opens your Instagram inbox and automatically replies to incoming conversations within a given date range.

| | |
|---|---|
| **Needs** | Reply text, start date, end date |
| **Does** | Scans inbox → finds unanswered conversations → types and sends the reply |
| **Output** | Number of conversations replied to |

---

### POST_LIKING
**Suggested name: `Like Posts`**

Likes a list of Instagram posts. Skips posts that are already liked.

| | |
|---|---|
| **Needs** | A list of post URLs |
| **Does** | Visits each post → clicks the Like (heart) button |
| **Output** | Number of posts liked |

---

### POST_COMMENTING
**Suggested name: `Comment on Posts`**

Leaves the same comment on a list of Instagram posts.

| | |
|---|---|
| **Needs** | A list of post URLs, a comment text |
| **Does** | Visits each post → types the comment → submits |
| **Output** | Number of posts commented on |

---

### COMMENT_LIKING
**Suggested name: `Like Comments on Posts`**

Likes a specific user's comment on a list of posts. If you don't specify a target author, it likes the most recent comment.

| | |
|---|---|
| **Needs** | A list of post URLs |
| **Optional** | Comment author username to target |
| **Does** | Visits each post → finds the target comment → clicks its Like button |
| **Output** | Number of comments liked |

---

### STORY_VIEWING
**Suggested name: `Watch Stories`**

Views the stories of a list of accounts. Automatically advances through each story and moves on to the next profile.

| | |
|---|---|
| **Needs** | A list of profile URLs |
| **Does** | Visits each profile → clicks the story ring → watches until stories end |
| **Output** | Number of profiles' stories viewed |

---

### PROFILE_SEARCH
**Suggested name: `Scrape Profile Info`**

Visits a list of Instagram profiles and extracts their data (username, full name, bio, follower count, following count, profile image, website) into your database.

| | |
|---|---|
| **Needs** | A list of profile URLs |
| **Does** | Visits each profile → extracts all available info → saves to database |
| **Output** | Profile records saved in People tab |

---

### PROFILE_FETCH
**Suggested name: `Export Followers / Following List`**

Opens a profile's followers or following list and saves every account it finds into your database.

| | |
|---|---|
| **Needs** | Source type (`followers` or `following`) |
| **Optional** | Max number of results, target list ID |
| **Does** | Opens the followers/following dialog → scrolls through the list → saves each person |
| **Output** | People records in database |

---

### PROFILE_INTERACTION
**Suggested name: `Engage with Hashtag Posts`**

Searches Instagram by hashtag and automatically likes (and optionally comments on) the posts it finds. Great for growing reach around a topic.

| | |
|---|---|
| **Needs** | Keywords/hashtags, max post count |
| **Optional** | Comment text, target account filter |
| **Does** | Searches hashtag → opens each post → likes it → optionally comments |
| **Output** | Number of posts liked/commented on |

---

### USER_POSTS_INTERACTION
**Suggested name: `Engage with a User's Posts`**

Visits a specific account's profile and likes (and optionally comments on) their recent posts.

| | |
|---|---|
| **Needs** | Target username, max post count |
| **Optional** | Comment text |
| **Does** | Opens user's profile grid → visits each post → likes → optionally comments |
| **Output** | Number of posts liked/commented on |

---

### KEYWORD_SEARCH
**Suggested name: `Find Profiles by Hashtag`**

Searches Instagram posts by hashtag and saves the profile of every user who posted under that tag into your database.

| | |
|---|---|
| **Needs** | Keyword/hashtag, max result count |
| **Does** | Searches hashtag → collects post URLs → extracts username from each post → fetches and saves full profile data |
| **Output** | People records in database |

---

### POST_SCRAPING
**Suggested name: `Extract Post Data`**

Visits a list of Instagram posts and saves structured data about each one: caption, like count, comment count, post date, media URLs, author.

| | |
|---|---|
| **Needs** | A list of post URLs |
| **Does** | Visits each post → extracts all metadata → saves to output |
| **Output** | JSON data files with post details |

---

### PUBLISH_CONTENT
**Suggested name: `Post to Instagram`**

Publishes a new post or story to Instagram. Supports images/videos, captions, and location tags.

| | |
|---|---|
| **Needs** | Caption text, media file path |
| **Optional** | Location tag |
| **Does** | Opens Instagram create flow → uploads media → types caption → adds location → publishes |
| **Output** | Post published to your account |

---

## LinkedIn

### BULK_MESSAGING
**Suggested name: `Send LinkedIn DMs`**

Sends a direct message to a list of LinkedIn profiles. Supports a message subject line for InMail-style sends.

| | |
|---|---|
| **Needs** | A list of profile URLs, message text |
| **Optional** | Subject line, template ID |
| **Does** | Visits each profile → clicks Message → types and sends |
| **Output** | Number of messages sent |

---

### BULK_REPLYING
**Suggested name: `Auto-Reply to LinkedIn Messages`**

Scans your LinkedIn inbox and replies to conversations within a date range.

| | |
|---|---|
| **Needs** | Reply text, start date, end date |
| **Does** | Opens inbox → finds conversations in range → replies to each |
| **Output** | Number of conversations replied to |

---

### KEYWORD_SEARCH
**Suggested name: `Find People on LinkedIn`**

Searches LinkedIn for people using a keyword and saves their profiles to your database.

| | |
|---|---|
| **Needs** | Keyword, max result count |
| **Does** | Runs a people search → extracts profiles from results → saves to database |
| **Output** | People records in database |

---

### PROFILE_SEARCH
**Suggested name: `Scrape LinkedIn Profile Info`**

Visits a list of LinkedIn profiles and saves their data (name, company, followers, email if visible).

| | |
|---|---|
| **Needs** | A list of profile URLs |
| **Does** | Visits each profile → extracts info → saves to database |
| **Output** | Profile records in database |

---

### PROFILE_FETCH
**Suggested name: `Export LinkedIn Connections`**

Exports a profile's followers or connections list into your database.

| | |
|---|---|
| **Needs** | Source type (`followers` or `following`) |
| **Optional** | Max result count, list ID |
| **Does** | Opens the connections/followers list → scrolls → saves each person |
| **Output** | People records in database |

---

### PROFILE_INTERACTION
**Suggested name: `Engage with LinkedIn Posts`**

Searches LinkedIn by keyword and likes (and optionally comments on) matching posts.

| | |
|---|---|
| **Needs** | Keywords, max post count |
| **Optional** | Comment text, target filter |
| **Does** | Searches content → opens each post → likes → optionally comments |
| **Output** | Number of posts engaged with |

---

### PUBLISH_CONTENT
**Suggested name: `Post to LinkedIn`**

Publishes a new post to your LinkedIn feed. Supports text and media.

| | |
|---|---|
| **Needs** | Post text |
| **Optional** | Media file, location tag |
| **Does** | Opens feed → clicks Start a post → types text → attaches media → publishes |
| **Output** | Post published to your profile |

---

## TikTok

### BULK_MESSAGING
**Suggested name: `Send TikTok DMs`**

Sends a direct message to a list of TikTok users.

| | |
|---|---|
| **Needs** | A list of profile URLs, message text |
| **Does** | Visits each profile → clicks Message → types and sends |
| **Output** | Number of messages sent |

---

### BULK_REPLYING
**Suggested name: `Auto-Reply to TikTok Messages`**

Scans your TikTok inbox and replies to conversations within a date range.

| | |
|---|---|
| **Needs** | Reply text, start date, end date |
| **Does** | Opens inbox → finds conversations in range → replies |
| **Output** | Number of conversations replied to |

---

### KEYWORD_SEARCH
**Suggested name: `Find TikTok Videos by Keyword`**

Searches TikTok for videos matching a keyword and saves the results.

| | |
|---|---|
| **Needs** | Keyword, max result count |
| **Does** | Searches TikTok → extracts video data → saves to output |
| **Output** | Video data records |

---

### PROFILE_SEARCH
**Suggested name: `Scrape TikTok Profile Info`**

Visits a list of TikTok profiles and extracts their info (username, followers, bio, profile picture).

| | |
|---|---|
| **Needs** | A list of profile URLs |
| **Does** | Visits each profile → extracts info → saves to database |
| **Output** | Profile records in database |

---

### PROFILE_FETCH
**Suggested name: `Export TikTok Followers / Following`**

Exports a TikTok account's followers or following list into your database.

| | |
|---|---|
| **Needs** | Source type (`followers` or `following`) |
| **Optional** | Max result count, list ID |
| **Does** | Opens followers/following list → scrolls → saves each person |
| **Output** | People records in database |

---

### PROFILE_INTERACTION
**Suggested name: `Engage with TikTok Videos`**

Searches TikTok by keyword and likes (and optionally comments on) the videos found.

| | |
|---|---|
| **Needs** | Keywords, max video count |
| **Optional** | Comment text, target filter |
| **Does** | Searches videos → opens each → likes → optionally comments |
| **Output** | Number of videos engaged with |

---

### PUBLISH_CONTENT
**Suggested name: `Post to TikTok`**

Uploads and publishes a video to TikTok with a caption.

| | |
|---|---|
| **Needs** | Caption text, video file path |
| **Optional** | Location tag |
| **Does** | Opens TikTok upload page → uploads video → types caption → publishes |
| **Output** | Video published to your account |

---

## X (Twitter)

### BULK_MESSAGING
**Suggested name: `Send X DMs`**

Sends a direct message to a list of X (Twitter) users.

| | |
|---|---|
| **Needs** | A list of profile URLs, message text |
| **Does** | Visits each profile → clicks Message → types and sends |
| **Output** | Number of messages sent |

---

### BULK_REPLYING
**Suggested name: `Auto-Reply to X Messages`**

Scans your X inbox and replies to conversations within a date range.

| | |
|---|---|
| **Needs** | Reply text, start date, end date |
| **Does** | Opens inbox → finds conversations in range → replies |
| **Output** | Number of conversations replied to |

---

### KEYWORD_SEARCH
**Suggested name: `Find Posts on X by Keyword`**

Searches X for tweets matching a keyword and saves the data.

| | |
|---|---|
| **Needs** | Keyword, max result count |
| **Does** | Searches X → extracts tweet data → saves to output |
| **Output** | Tweet data records |

---

### PROFILE_SEARCH
**Suggested name: `Scrape X Profile Info`**

Visits a list of X profiles and extracts their info (username, bio, follower count, following count).

| | |
|---|---|
| **Needs** | A list of profile URLs |
| **Does** | Visits each profile → extracts info → saves to database |
| **Output** | Profile records in database |

---

### PROFILE_FETCH
**Suggested name: `Export X Followers / Following`**

Exports a profile's followers or following list into your database.

| | |
|---|---|
| **Needs** | Source type (`followers` or `following`) |
| **Optional** | Max result count, list ID |
| **Does** | Opens followers/following list → scrolls → saves each person |
| **Output** | People records in database |

---

### PROFILE_INTERACTION
**Suggested name: `Engage with X Posts`**

Searches X by keyword and likes (and optionally replies to) matching tweets.

| | |
|---|---|
| **Needs** | Keywords, max post count |
| **Optional** | Reply text, target filter |
| **Does** | Searches tweets → opens each → likes → optionally replies |
| **Output** | Number of tweets engaged with |

---

### PUBLISH_CONTENT
**Suggested name: `Post a Tweet`**

Publishes a new tweet to your X account. Supports text and media.

| | |
|---|---|
| **Needs** | Tweet text |
| **Optional** | Media file |
| **Does** | Opens home feed → clicks tweet compose → types text → attaches media → tweets |
| **Output** | Tweet published to your account |

---

## Quick Reference

| Internal Name | Platform | Suggested Name |
|---|---|---|
| BULK_FOLLOWING | Instagram | Follow Users |
| BULK_UNFOLLOWING | Instagram | Unfollow Users |
| BULK_MESSAGING | Instagram / LinkedIn / TikTok / X | Send DMs |
| BULK_REPLYING | Instagram / LinkedIn / TikTok / X | Auto-Reply to DMs |
| POST_LIKING | Instagram | Like Posts |
| POST_COMMENTING | Instagram | Comment on Posts |
| COMMENT_LIKING | Instagram | Like Comments on Posts |
| STORY_VIEWING | Instagram | Watch Stories |
| PROFILE_SEARCH | Instagram / LinkedIn / TikTok / X | Scrape Profile Info |
| PROFILE_FETCH | Instagram / LinkedIn / TikTok / X | Export Followers / Following |
| PROFILE_INTERACTION | Instagram / LinkedIn / TikTok / X | Engage with Posts |
| USER_POSTS_INTERACTION | Instagram | Engage with a User's Posts |
| KEYWORD_SEARCH | Instagram / LinkedIn / TikTok / X | Find Profiles / Posts by Keyword |
| POST_SCRAPING | Instagram | Extract Post Data |
| PUBLISH_CONTENT | Instagram / LinkedIn / TikTok / X | Publish Post |
