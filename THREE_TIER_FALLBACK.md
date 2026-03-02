# 3-Tier Fallback System for Instagram Actions

## Overview

Every DOM interaction in an Instagram action JSON follows a consistent 3-tier cascade:

1. **Tier 1 — Go bot method** (`call_bot_method`): Handles the entire interaction in Go code with multiple internal fallback selectors, JS-based identification, and Rod native clicks. Most reliable.
2. **Tier 2 — JSON selectors** (`find_element` with `xpath`/`alternatives`): Hardcoded XPath selectors defined directly in the action JSON. Works when DOM structure matches.
3. **Tier 3 — AI-generated selectors** (`find_element` with `configKey`): Dynamically obtained selectors from the config manager, which can use AI to analyze the current DOM and generate selectors.

Each tier only runs if the previous tier failed. The cascade stops as soon as any tier succeeds.

## How It Works

### Execution Flow

```
Tier 1 (bot method) ─── success ──→ skip Tier 2 & 3 ──→ continue
         │
       failure (skip)
         │
   ┌── check_t1 (not_exists) ──→ false ──→ continue
   │
   └── true ──→ Tier 2 (XPath selectors) ─── success ──→ skip Tier 3 ──→ continue
                         │
                       failure (skip)
                         │
                   ┌── check_t2 (not_exists) ──→ false ──→ continue
                   │
                   └── true ──→ Tier 3 (configKey) ─── success ──→ continue
                                        │
                                      failure ──→ mark_failed
```

### JSON Template

```json
{
  "steps": [
    {
      "id": "action_t1",
      "type": "call_bot_method",
      "methodName": "method_name",
      "args": ["{{item.url}}"],
      "variable_name": "t1Result",
      "timeout": 30,
      "onError": { "action": "skip" }
    },
    {
      "id": "check_t1",
      "type": "condition",
      "condition": { "variable": "t1Result", "operator": "not_exists" },
      "then": ["action_t2_find", "action_t2_click", "check_t2"]
    },
    {
      "id": "action_t2_find",
      "type": "find_element",
      "xpath": "//primary/xpath",
      "alternatives": ["//fallback/xpath/1", "//fallback/xpath/2"],
      "variable_name": "t2Element",
      "timeout": 10,
      "onError": { "action": "skip" }
    },
    {
      "id": "action_t2_click",
      "type": "click",
      "elementRef": "action_t2_find"
    },
    {
      "id": "check_t2",
      "type": "condition",
      "condition": { "variable": "t2Element", "operator": "not_exists" },
      "then": ["action_t3_find", "action_t3_click"]
    },
    {
      "id": "action_t3_find",
      "type": "find_element",
      "configKey": "field_name",
      "timeout": 15,
      "onError": { "action": "mark_failed" }
    },
    {
      "id": "action_t3_click",
      "type": "click",
      "elementRef": "action_t3_find"
    }
  ]
}
```

### Key Rules

- **Tier 1** always uses `onError: { "action": "skip" }` — failure is expected and recoverable.
- **Tier 3** always uses `onError: { "action": "mark_failed" }` — it's the last resort.
- **Condition checks** use the `not_exists` operator on the variable set by the previous tier.
- **Loop steps** only list the top-level entry points (Tier 1 + check_t1 + wait + log). Tier 2/3 steps are invoked via condition `then` clauses.

## Connecting configKey to Schemas

When a step uses `configKey: "field_name"`, the executor:

1. Calls `configMgr.GetSelectorForField(platform, actionType, fieldName)`
2. The config manager looks up the schema in `internal/config/schemas.go` using the key `PLATFORM_ACTION` (e.g., `INSTAGRAM_POST_COMMENTING`)
3. If configured selectors exist (from prior AI analysis or admin override), they're returned
4. Otherwise, the config manager can invoke the AI to analyze the current DOM and generate a selector

### Available Schema Keys

| Schema Key | Fields |
|---|---|
| `INSTAGRAM_POST_COMMENTING` | `comment_textarea`, `post_button` |
| `INSTAGRAM_BULK_REPLYING` | `conversation_list`, `conversation_item`, `message_input`, `send_button` |
| `INSTAGRAM_PROFILE_FETCH` | `followers_link`, `following_link`, `user_list_dialog`, `user_item_link` |
| `INSTAGRAM_POST_INTERACTION` | `like_button`, `comment_textarea`, `post_button` |
| `INSTAGRAM_USER_POSTS_INTERACTION` | alias → `POST_INTERACTION` |
| `INSTAGRAM_PUBLISH_CONTENT` | `create_button`, `file_input`, `next_button`, `caption_input`, `location_input`, `share_button` |
| `INSTAGRAM_SEARCH_RESULTS` | `post_link` |

## Checklist: Adding a New Action with 3 Tiers

1. **Create the Go bot method** in `internal/bot/instagram/actions.go`
   - Follow the JS-identify → mark with `data-monoes-*` → Rod native click pattern
   - Use `page.Keyboard.Type()` not `element.Type()`
   - Call `dismissNotificationDialog()` after every navigation
   - Handle multiple internal fallback selectors within the method

2. **Register the method** in `GetMethodByName()` in `internal/bot/instagram/bot.go`
   - Handle `float64 → int` conversion for numeric args from JSON
   - Always cast `args[0]` as `*rod.Page`

3. **Add the schema** in `internal/config/schemas.go`
   - Define fields matching the configKeys used in Tier 3
   - Register in the `schemas` map

4. **Write the action JSON** in `data/actions/instagram/`
   - Tier 1: `call_bot_method` with `onError: skip`
   - Check: condition with `not_exists`
   - Tier 2: `find_element` with xpath + alternatives, `onError: skip`
   - Check: condition with `not_exists`
   - Tier 3: `find_element` with configKey, `onError: mark_failed`

5. **Add integration test** in `internal/action/instagram_integration_test.go`

## Debugging Guide

### Tier 1 Failures

Bot method errors appear in logs as `call_bot_method <stepID>/<methodName>: <error>`. Common causes:
- Navigation timeout (Instagram slow to load)
- Element not found by JS identification (DOM structure changed)
- Click didn't trigger expected state change

### Tier 2 Failures

XPath selector mismatches. Check:
- Has Instagram changed its DOM structure?
- Are the XPath alternatives comprehensive enough?
- Screenshot the page state with `saveScreenshot(t, page, "/tmp/debug.png")`

### Tier 3 Failures

Config manager couldn't generate a working selector. Check:
- Is the schema registered in `schemas.go`?
- Is the config manager properly initialized (not nil in tests)?
- Does the AI have enough context about the page structure?

### General Tips

- Use `page.Eval()` to dump DOM structure when selectors fail
- Check both screenshot AND JS eval output to understand page state
- Instagram swaps class names frequently — prefer aria-labels and role attributes
- Always test with `go test -tags integration -run TestName -v -timeout 3m ./internal/action/`
