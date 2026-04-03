# Mono Agent — UI/UX Architecture Review

**Date:** 2026-04-03  
**Scope:** Full review of all 17 JSX components, 1580-line CSS design system, navigation, accessibility, and user experience.

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Critical Issues](#2-critical-issues)
3. [Major Issues](#3-major-issues)
4. [CSS Design System](#4-css-design-system)
5. [Accessibility](#5-accessibility)
6. [Per-Page Review](#6-per-page-review)
7. [Dead Code & Redundancy](#7-dead-code--redundancy)
8. [Recommended Fixes (Priority Order)](#8-recommended-fixes-priority-order)

---

## 1. Executive Summary

| Severity | Count |
|----------|-------|
| Critical | 11 |
| Major | 18 |
| Minor | 15 |

**Key themes:**
- **Two dead pages** (Workflow.jsx, Sessions.jsx) — never routed, unreachable
- **No undo/redo** in the workflow editor — destructive operations are irreversible
- **Inline styles dominate** (798 inline vs 344 className) — CSS design system is underused
- **Focus styles removed globally** — keyboard-only users cannot navigate forms
- **No loading/error states** — API failures leave spinners running forever
- **Expired connections show as "connected"** — green dot misleads users

---

## 2. Critical Issues

### Navigation & State

| ID | Issue | File:Line | Impact |
|----|-------|-----------|--------|
| NAV-1 | Back-navigation from Profile/PostDetail is hardcoded, not stack-based — user can get stranded | `App.jsx:36-42` | Cannot return to origin page |
| NAV-2 | `profileId` not cleared on navigate — stale profile renders on re-entry | `App.jsx:44-47` | Shows wrong person's data |
| NAV-3 | macOS drag region on `.page-header` can eat button clicks | `index.css:338-339` | Buttons silently unclickable |

### Workflow Editor

| ID | Issue | File:Line | Impact |
|----|-------|-----------|--------|
| WF-1 | "New Workflow" clears canvas with no save confirmation | `NodeRunner.jsx:1272-1276` | Loses all unsaved work |
| WF-2 | Inspector only opens via tiny 12px gear icon, not on node click | `NodeRunner.jsx:113,1629-1650` | Users think clicking a node should open config |
| WF-3 | `CREDENTIAL_PLATFORMS` is hardcoded — new node types silently get no credential picker | `NodeRunner.jsx:485-510` | Can't authenticate nodes |

### Connections

| ID | Issue | File:Line | Impact |
|----|-------|-----------|--------|
| CONN-1 | Disconnect fires immediately with no confirmation | `Connections.jsx:217-228` | Accidental click deletes credentials |
| CONN-2 | OAuth "Connect" button disabled with no visible explanation (Wails tooltip doesn't show) | `Connections.jsx:385-395` | User sees greyed-out button, doesn't know why |
| CONN-3 | Expired sessions show green "connected" dot | `Connections.jsx:39` | User thinks they're connected when session is dead |

### AI & Actions

| ID | Issue | File:Line | Impact |
|----|-------|-----------|--------|
| AI-1 | AIChatPanel: sending silently blocked when no provider — no feedback | `AIChatPanel.jsx:240` | User types, presses Enter, nothing happens |
| ACT-1 | Keywords/Message fields silently disappear when action type selected — data lost | `Actions.jsx:97-118` | User-entered text vanishes |

---

## 3. Major Issues

### Workflow Editor

| ID | Issue | File:Line |
|----|-------|-----------|
| WF-4 | No undo/redo — all destructive operations (delete node/edge) are irreversible | Both editors |
| WF-5 | "Clear Canvas" (Trash button) has no confirmation | `NodeRunner.jsx:1428-1434` |
| WF-6 | Dashboard workflow ChevronRight navigates to editor but doesn't open that workflow | `Dashboard.jsx:151` |
| WF-7 | Inspector has no placeholders or help text; required `*` marker is grey, not red | `NodeRunner.jsx:751-759` |
| WF-8 | Output preview capped at 5 items with no "Show All" button | `NodeRunner.jsx:818` |
| WF-9 | Multi-output edges have no labels; edges have no hover state | `NodeRunner.jsx:1605-1614` |
| WF-10 | `resource_picker` in Workflow.jsx is a plain text input, not ResourcePickerField | `Workflow.jsx:1407-1425` |

### Data & People

| ID | Issue | File:Line |
|----|-------|-----------|
| PPL-1 | API failures leave spinner running forever — no try/catch/finally | `People.jsx:428`, `Profile.jsx:259` |
| PPL-2 | Tag editor popup has no keyboard focus trap | `People.jsx:103-109` |
| PPL-3 | Profile interaction history loads all rows with no pagination | `Profile.jsx:263` |

### AI & Actions

| ID | Issue | File:Line |
|----|-------|-----------|
| AI-2 | "Save & Test" persists broken provider on test failure — no delete option | `AIProviders.jsx:105-127` |
| AI-3 | Stale closure in AIChatPanel streaming effect (`onWorkflowCreated` not in deps) | `AIChatPanel.jsx:170-230` |
| ACT-2 | Action types shown as raw SNAKE_CASE — not human-readable | `Actions.jsx:92-95` |
| ACT-3 | No way to edit an action after creation — must delete and recreate | `Actions.jsx:307-321` |
| ACT-4 | ActionInputsForm schema is frontend-only — unknown types silently show nothing | `ActionInputsForm.jsx:191-193` |

### General

| ID | Issue | File:Line |
|----|-------|-----------|
| GEN-1 | No loading state on Dashboard — stat cards flash "—" during fetch | `Dashboard.jsx:53-61` |
| GEN-2 | Version/update logic duplicated across 4 components — can show conflicting states | `StatusBar.jsx`, `Settings.jsx`, `Sidebar.jsx`, `Dashboard.jsx` |

---

## 4. CSS Design System

### Architecture Issues

| ID | Issue | Confidence |
|----|-------|------------|
| CSS-1 | **Inline styles dominate 2:1 over CSS classes** — hover state managed in JS useState/imperative DOM mutation instead of CSS `:hover` | 95 |
| CSS-2 | **Undefined CSS variables** (`--bg-hover`, `--bg-surface`, `--error`) used with wrong fallback colors in ResourcePicker section | 92 |
| CSS-3 | **`wfFlow` keyframe defined twice** with different values (index.css vs Workflow.jsx `<style>` tag) | 88 |
| CSS-4 | **No spacing/typography tokens** — `fontSize: 11` appears 80+ times as inline literal | 80 |
| CSS-5 | **Zero media queries** — no responsive behavior, stat-grid hardcodes 4 columns | 80 |

### Consistency Issues

| ID | Issue | Confidence |
|----|-------|------------|
| CON-1 | **Hardcoded hex colors** bypass design tokens (`#ef4444` instead of `var(--red)`, `#1e293b` has no token) | 90 |
| CON-2 | **Duplicate Avatar component** in People.jsx and Profile.jsx — identical logic, different implementation | 85 |
| CON-3 | **Modal pattern inconsistent** — CSS defines `.modal-overlay`/`.modal` classes but Connections, People, AIChatPanel use fully inline modals | 85 |
| CON-4 | **Canvas nodes use separate color palette** (`#0d1a28`, `#1e293b`, `#091220`) not in `:root` — can't retheme | 90 |

### Performance

| ID | Issue | Confidence |
|----|-------|------------|
| PERF-1 | **28 imperative style mutations** via `onMouseEnter`/`onMouseLeave` in NodeRunner canvas — bypasses React, creates split model | 88 |
| PERF-2 | **`QuickAccessCard` hover** uses `useState` + inline style objects — creates new objects every render for a CSS `:hover` equivalent | 80 |

---

## 5. Accessibility

| ID | Issue | WCAG | Confidence |
|----|-------|------|------------|
| A11Y-1 | **Focus styles globally removed** — `input:focus-visible { outline: none }` on all form controls. Custom box-shadow replacement barely visible (10% opacity cyan) | 2.4.7, 2.4.11 | 95 |
| A11Y-2 | **No `prefers-reduced-motion`** — 7 CSS animations run unconditionally including continuous `pulse-dot` and `wfFlow` | 2.3.3 | 95 |
| A11Y-3 | **Touch targets below 24px** — Settings gear (16px), tag edit button (18px), ArrowLeft close (16px), port circles | 2.5.8 | 83 |
| A11Y-4 | **Dashboard grid collapses badly** at narrow widths — fixed 340px right column, no responsive fallback | — | 85 |
| A11Y-5 | **Form input focus states use `:focus` not `:focus-visible`** — always visible on mouse click, not just keyboard | — | 88 |

---

## 6. Per-Page Review

### Dashboard
- Stat cards have no loading skeleton — show "—" during fetch
- Workflow ChevronRight doesn't open the specific workflow
- `stat-grid` hardcodes 4 columns with no responsive fallback
- `dashboard-grid` has fixed 340px right column — collapses at narrow widths

### Workflow Editor (NodeRunner.jsx)
- No undo/redo for any operation
- New/Clear canvas with no save confirmation
- Inspector opens only via tiny gear icon
- No dirty-state tracking — unsaved workflow name changes silently lost
- Password fields have no show/hide toggle
- Boolean fields are bare checkboxes (inconsistent with design)
- Select options show raw enum strings (`USER_ENTERED`, `gzip_compress`)
- Dot grid not scaled with zoom
- No edge labels for multi-output nodes

### Connections
- Disconnect has no confirmation
- OAuth button disabled with no visible explanation
- Expired sessions show green "connected" dot
- Browser login retry logic duplicated inline (diverges from original)
- No help guides for social platform browser login

### People & Profile
- API failures leave spinner forever (no try/catch)
- Tag editor has no keyboard focus trap
- Interaction history loads all rows (no pagination)
- Search input has no clear button
- Duplicate Avatar component (People vs Profile)

### AI Providers
- Connected provider name truncated to 72px with no tooltip
- Failed test leaves persisted broken record with no removal path
- Chat button completely hidden until provider is active (no hint it exists)

### Actions
- Keywords/Message fields silently disappear on type selection
- Action types shown as raw SNAKE_CASE
- No edit capability after creation
- `window.confirm` for delete is inconsistent with app's modal patterns
- Date fields use plain text input with no validation

### Logs
- Text-only filter — no level dropdown (ERROR, WARN, INFO)
- Good: auto-scroll on new entries, clear button, color-coded levels

### Settings
- Near-empty page — just two navigation cards to Connections and AI
- Adds no value over sidebar navigation

### Sessions (DEAD CODE)
- Not in sidebar nav, not in App.jsx pages map — completely unreachable

### Workflow.jsx (DEAD CODE)
- Not routed in App.jsx — has improvements NodeRunner lacks (flowing edges, rename-on-click) but never used

---

## 7. Dead Code & Redundancy

| Item | File | Status |
|------|------|--------|
| `Workflow.jsx` (2117 lines) | `pages/Workflow.jsx` | Never imported in App.jsx — fully dead |
| `Sessions.jsx` (211 lines) | `pages/Sessions.jsx` | Never imported in App.jsx — fully dead |
| `Credentials.jsx` | Previously deleted | Already removed in P0 fixes |
| `.stat-card-label` CSS rule | `index.css:85` | Class doesn't exist — dead CSS |
| `wfFlow` keyframe duplicate | `index.css:90` vs `Workflow.jsx:2068` | Conflicting values |

---

## 8. Recommended Fixes (Priority Order)

### P0 — Critical UX (blocks users)

| # | Fix | Files | Effort |
|---|-----|-------|--------|
| 1 | Add disconnect confirmation dialog | `Connections.jsx:217` | Small |
| 2 | Fix expired session tile — show yellow dot, not green | `Connections.jsx:39` | 1 line |
| 3 | Show inline hint when OAuth "Connect" is disabled | `Connections.jsx:385-395` | Small |
| 4 | Clear `profileId` on navigate (match `postId` pattern) | `App.jsx:44-47` | 1 line |
| 5 | Open Inspector on node click, not just gear icon | `NodeRunner.jsx:113` | Small |
| 6 | Add save confirmation before "New Workflow" | `NodeRunner.jsx:1272` | Small |
| 7 | Show feedback when AI chat send is blocked | `AIChatPanel.jsx:240` | Small |
| 8 | Remove global `input:focus-visible { outline: none }` | `index.css:506-508` | 1 line |

### P1 — Major UX

| # | Fix | Files | Effort |
|---|-----|-------|--------|
| 9 | Add try/catch/finally to all page load functions | `People.jsx`, `Profile.jsx`, `Connections.jsx` | Small |
| 10 | Add loading skeletons to Dashboard stat cards | `Dashboard.jsx` | Small |
| 11 | Implement undo/redo stack in workflow editor | `NodeRunner.jsx` | Medium |
| 12 | Add "Clear Canvas" confirmation | `NodeRunner.jsx:1428` | Small |
| 13 | Derive credential platforms from `node.schema.credential_platform` | `NodeRunner.jsx:485-510` | Medium |
| 14 | Add action edit capability | `Actions.jsx` | Medium |
| 15 | Human-readable action type labels | `Actions.jsx` | Small |
| 16 | Lift version/update state to App.jsx — remove duplication | Multiple | Medium |
| 17 | Add `prefers-reduced-motion` media query | `index.css` | Small |
| 18 | Add log level filter dropdown | `Logs.jsx` | Small |

### P2 — Polish & Architecture

| # | Fix | Files | Effort |
|---|-----|-------|--------|
| 19 | Delete dead `Workflow.jsx` and `Sessions.jsx` (or wire them in) | `pages/` | Small |
| 20 | Fix undefined CSS variables in ResourcePicker section | `index.css:1547-1580` | Small |
| 21 | Extract shared Avatar component | `People.jsx`, `Profile.jsx` | Small |
| 22 | Use CSS `:hover` instead of JS hover state (28 occurrences in NodeRunner) | `NodeRunner.jsx` | Medium |
| 23 | Replace inline styles with CSS classes for repeated patterns | All pages | Large |
| 24 | Add spacing/typography tokens to CSS variables | `index.css` | Medium |
| 25 | Add responsive breakpoints for narrow windows | `index.css` | Medium |
| 26 | Increase touch targets on gear/close/tag buttons to 24px minimum | Multiple | Small |
| 27 | Add keyboard focus trap to tag editor popup | `People.jsx:103` | Small |
| 28 | Add interaction history pagination in Profile | `Profile.jsx:263` | Medium |
| 29 | Use CSS modal classes consistently across all pages | `Connections.jsx`, `People.jsx` | Medium |
| 30 | Add `{ value, label }` support for select field options | `NodeRunner.jsx`, schema files | Medium |

---

## Key Files Reference

| File | Lines | Role |
|------|-------|------|
| `index.css` | 1580 | Full CSS design system |
| `App.jsx` | 120 | Root layout, routing, state |
| `NodeRunner.jsx` | 2218 | Primary workflow editor |
| `Workflow.jsx` | 2117 | Dead: alternative workflow editor |
| `Connections.jsx` | 662 | Platform auth management |
| `People.jsx` | 609 | Contact list with tags |
| `Profile.jsx` | 458 | Person detail view |
| `AIProviders.jsx` | 668 | AI model configuration |
| `AIChatPanel.jsx` | 497 | AI chat interface |
| `ActionInputsForm.jsx` | 401 | Dynamic action form |
| `Dashboard.jsx` | 386 | Landing page with stats |
| `Actions.jsx` | 342 | Legacy action management |
| `Settings.jsx` | 260 | Settings hub |
| `ResourcePickerField.jsx` | 241 | Google Sheets/Drive resource browser |
| `Sessions.jsx` | 211 | Dead: session management |
| `Sidebar.jsx` | 108 | Navigation sidebar |
| `StatusBar.jsx` | 161 | Bottom status bar |
| `Logs.jsx` | 117 | Live log viewer |
