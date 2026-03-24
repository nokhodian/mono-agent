import { useState, useEffect, useRef, useCallback } from 'react'
import {
  X, ZoomIn, ZoomOut, Trash2, Database, Zap,
  ChevronLeft, ChevronRight, Play, RotateCcw, Save, Settings2,
  List, Plus, ArrowLeft, Search, ChevronDown, Check, AlertCircle,
  Clock, RefreshCw, ToggleLeft, ToggleRight, Loader,
} from 'lucide-react'

// ── Wails backend (static import with localStorage mock fallback) ─────
import * as _WailsApp from '../wailsjs/go/main/App'

// localStorage mock for dev without Wails runtime
const LS_MOCK = 'monoes-wf-mock-v1'
const _mockStore = () => { try { return JSON.parse(localStorage.getItem(LS_MOCK) || '{}') } catch { return {} } }
const _persist = (s) => { try { localStorage.setItem(LS_MOCK, JSON.stringify(s)) } catch {} }

const _ListWorkflows = _WailsApp.ListWorkflows ?? (() => Promise.resolve(Object.values(_mockStore())))
const _GetWorkflow   = _WailsApp.GetWorkflow   ?? ((id) => Promise.resolve(_mockStore()[id] || null))
const _SaveWorkflow  = _WailsApp.SaveWorkflow  ?? ((req) => {
  const s = _mockStore()
  const id = req.id || `wf_${Date.now()}`
  const now = new Date().toISOString()
  const existing = s[id] || {}
  const next = { ...existing, ...req, id, updated_at: now, created_at: existing.created_at || now, version: (existing.version || 0) + 1 }
  s[id] = next; _persist(s)
  return Promise.resolve(next)
})
const _DeleteWorkflow       = _WailsApp.DeleteWorkflow       ?? ((id) => { const s = _mockStore(); delete s[id]; _persist(s); return Promise.resolve() })
const _SetWorkflowActive    = _WailsApp.SetWorkflowActive    ?? ((id, act) => { const s = _mockStore(); if (s[id]) { s[id].active = act; _persist(s) } return Promise.resolve() })
const _RunWorkflow          = _WailsApp.RunWorkflow          ?? (() => Promise.resolve({ execution_id: `exec_${Date.now()}`, status: 'running' }))
const _GetWorkflowExecutions = _WailsApp.GetWorkflowExecutions ?? (() => Promise.resolve([]))

// ── Layout constants ─────────────────────────────────────────────────
const NODE_W   = 236
const HEAD_H   = 48
const PORT_H   = 30
const PORT_PAD = 10
const PORT_R   = 6.5

// ── Platform accent colors ───────────────────────────────────────────
const PCOL = {
  instagram: '#e1306c',
  linkedin:  '#0a66c2',
  x:         '#8899aa',
  tiktok:    '#ff0050',
  default:   '#00b4d8',
}

// ── Node template catalogue ──────────────────────────────────────────
// Each category: { id, label, color, icon, nodes: [...] }
// Each node: { subtype, label, platform?, color?, inputs, outputs, configFields }

const TRIGGER_COLOR   = '#7c3aed'
const CONTROL_COLOR   = '#0891b2'
const DATA_COLOR      = '#d97706'
const HTTP_COLOR      = '#d97706'
const SYSTEM_COLOR    = '#64748b'
const DB_COLOR        = '#1d4ed8'
const COMM_COLOR      = '#9333ea'
const SERVICES_COLOR  = '#0f766e'

const NODE_CATEGORIES = [
  {
    id: 'triggers',
    label: 'TRIGGERS',
    color: TRIGGER_COLOR,
    nodes: [
      {
        subtype: 'trigger.manual', label: 'Manual Trigger', color: TRIGGER_COLOR,
        inputs: [], outputs: [{ id: 'out', label: 'output' }], configFields: [],
      },
      {
        subtype: 'trigger.schedule', label: 'Schedule Trigger', color: TRIGGER_COLOR,
        inputs: [], outputs: [{ id: 'out', label: 'output' }],
        configFields: [{ key: 'cron', label: 'Cron Expression', type: 'text', default: '0 9 * * *' }],
      },
      {
        subtype: 'trigger.webhook', label: 'Webhook Trigger', color: TRIGGER_COLOR,
        inputs: [], outputs: [{ id: 'body', label: 'body' }, { id: 'headers', label: 'headers' }],
        configFields: [{ key: 'path', label: 'Webhook Path', type: 'text' }],
      },
    ],
  },
  {
    id: 'control',
    label: 'CONTROL',
    color: CONTROL_COLOR,
    nodes: [
      {
        subtype: 'if', label: 'If / Branch', color: CONTROL_COLOR,
        inputs: [{ id: 'in', label: 'in' }],
        outputs: [{ id: 'true', label: 'true' }, { id: 'false', label: 'false' }],
        configFields: [{ key: 'condition', label: 'Condition', type: 'text' }],
      },
      {
        subtype: 'switch', label: 'Switch', color: CONTROL_COLOR,
        inputs: [{ id: 'in', label: 'in' }],
        outputs: [{ id: 'case0', label: 'case0' }, { id: 'case1', label: 'case1' }, { id: 'default', label: 'default' }],
        configFields: [{ key: 'value', label: 'Value', type: 'text' }],
      },
      {
        subtype: 'merge', label: 'Merge', color: CONTROL_COLOR,
        inputs: [{ id: 'input0', label: 'input0' }, { id: 'input1', label: 'input1' }],
        outputs: [{ id: 'out', label: 'out' }], configFields: [],
      },
      {
        subtype: 'split_in_batches', label: 'Split In Batches', color: CONTROL_COLOR,
        inputs: [{ id: 'in', label: 'in' }],
        outputs: [{ id: 'batch', label: 'batch' }, { id: 'done', label: 'done' }],
        configFields: [{ key: 'batchSize', label: 'Batch Size', type: 'number', default: '10' }],
      },
      {
        subtype: 'wait', label: 'Wait', color: CONTROL_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'duration', label: 'Duration (seconds)', type: 'number', default: '5' }],
      },
      {
        subtype: 'stop_error', label: 'Stop & Error', color: '#ef4444',
        inputs: [{ id: 'in', label: 'in' }], outputs: [],
        configFields: [{ key: 'message', label: 'Error Message', type: 'text' }],
      },
      {
        subtype: 'set', label: 'Set', color: CONTROL_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'fields', label: 'Fields (JSON)', type: 'textarea' }],
      },
      {
        subtype: 'code', label: 'Code (JS)', color: CONTROL_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'code', label: 'JavaScript Code', type: 'textarea', default: 'return items;' }],
      },
      {
        subtype: 'filter', label: 'Filter', color: CONTROL_COLOR,
        inputs: [{ id: 'in', label: 'in' }],
        outputs: [{ id: 'pass', label: 'pass' }, { id: 'fail', label: 'fail' }],
        configFields: [{ key: 'condition', label: 'Condition', type: 'text' }],
      },
      {
        subtype: 'sort', label: 'Sort', color: CONTROL_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'key', label: 'Sort Key', type: 'text' }],
      },
      {
        subtype: 'limit', label: 'Limit', color: CONTROL_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'max', label: 'Max Items', type: 'number', default: '100' }],
      },
      {
        subtype: 'remove_duplicates', label: 'Remove Duplicates', color: CONTROL_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'key', label: 'Key Field', type: 'text' }],
      },
      {
        subtype: 'aggregate', label: 'Aggregate', color: CONTROL_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'operation', label: 'Operation', type: 'select', options: ['sum', 'avg', 'count', 'min', 'max'] }],
      },
    ],
  },
  {
    id: 'data',
    label: 'DATA',
    color: DATA_COLOR,
    nodes: [
      {
        subtype: 'keywords', label: 'Keywords', color: DATA_COLOR,
        inputs: [], outputs: [{ id: 'items', label: 'items' }],
        configFields: [{ key: 'items', label: 'Keywords (one per line)', type: 'textarea' }],
      },
      {
        subtype: 'profile_urls', label: 'Profile URLs', color: DATA_COLOR,
        inputs: [], outputs: [{ id: 'items', label: 'items' }],
        configFields: [{ key: 'items', label: 'Profile URLs (one per line)', type: 'textarea' }],
      },
      {
        subtype: 'post_urls', label: 'Post URLs', color: DATA_COLOR,
        inputs: [], outputs: [{ id: 'items', label: 'items' }],
        configFields: [{ key: 'items', label: 'Post URLs (one per line)', type: 'textarea' }],
      },
      {
        subtype: 'text_value', label: 'Text Value', color: DATA_COLOR,
        inputs: [], outputs: [{ id: 'value', label: 'value' }],
        configFields: [{ key: 'value', label: 'Value', type: 'text' }],
      },
      {
        subtype: 'datetime', label: 'Date & Time', color: DATA_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'operation', label: 'Operation', type: 'text' }],
      },
      {
        subtype: 'crypto', label: 'Crypto', color: DATA_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'operation', label: 'Operation (hash/encrypt)', type: 'text' }],
      },
      {
        subtype: 'html', label: 'HTML Extract', color: DATA_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'selector', label: 'CSS Selector', type: 'text' }],
      },
      {
        subtype: 'xml', label: 'XML', color: DATA_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [],
      },
      {
        subtype: 'markdown', label: 'Markdown', color: DATA_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [],
      },
      {
        subtype: 'spreadsheet', label: 'Spreadsheet', color: DATA_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'filePath', label: 'File Path', type: 'text' }],
      },
      {
        subtype: 'compression', label: 'Compress/Extract', color: DATA_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [],
      },
      {
        subtype: 'write_binary_file', label: 'Write File', color: DATA_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [],
        configFields: [{ key: 'path', label: 'File Path', type: 'text' }],
      },
    ],
  },
  {
    id: 'http',
    label: 'HTTP',
    color: HTTP_COLOR,
    nodes: [
      {
        subtype: 'http.request', label: 'HTTP Request', color: HTTP_COLOR,
        inputs: [{ id: 'in', label: 'in' }],
        outputs: [{ id: 'out', label: 'out' }, { id: 'error', label: 'error' }],
        configFields: [
          { key: 'method', label: 'Method', type: 'select', options: ['GET', 'POST', 'PUT', 'DELETE', 'PATCH'] },
          { key: 'url', label: 'URL', type: 'text' },
          { key: 'body', label: 'Body (JSON)', type: 'textarea' },
        ],
      },
      {
        subtype: 'http.ftp', label: 'FTP', color: HTTP_COLOR,
        inputs: [{ id: 'in', label: 'in' }],
        outputs: [{ id: 'out', label: 'out' }, { id: 'error', label: 'error' }],
        configFields: [
          { key: 'host', label: 'Host', type: 'text' },
          { key: 'operation', label: 'Operation', type: 'text' },
        ],
      },
      {
        subtype: 'http.ssh', label: 'SSH', color: HTTP_COLOR,
        inputs: [{ id: 'in', label: 'in' }],
        outputs: [{ id: 'out', label: 'out' }, { id: 'error', label: 'error' }],
        configFields: [
          { key: 'host', label: 'Host', type: 'text' },
          { key: 'command', label: 'Command', type: 'text' },
        ],
      },
    ],
  },
  {
    id: 'system',
    label: 'SYSTEM',
    color: SYSTEM_COLOR,
    nodes: [
      {
        subtype: 'system.execute_command', label: 'Execute Command', color: SYSTEM_COLOR,
        inputs: [{ id: 'in', label: 'in' }],
        outputs: [{ id: 'stdout', label: 'stdout' }, { id: 'stderr', label: 'stderr' }],
        configFields: [{ key: 'command', label: 'Command', type: 'text' }],
      },
      {
        subtype: 'system.rss_read', label: 'RSS Read', color: SYSTEM_COLOR,
        inputs: [], outputs: [{ id: 'items', label: 'items' }],
        configFields: [{ key: 'url', label: 'Feed URL', type: 'text' }],
      },
    ],
  },
  {
    id: 'database',
    label: 'DATABASE',
    color: DB_COLOR,
    nodes: [
      {
        subtype: 'mysql', label: 'MySQL', color: DB_COLOR,
        inputs: [{ id: 'in', label: 'in' }],
        outputs: [{ id: 'rows', label: 'rows' }, { id: 'error', label: 'error' }],
        configFields: [{ key: 'query', label: 'SQL Query', type: 'textarea' }],
      },
      {
        subtype: 'postgres', label: 'PostgreSQL', color: DB_COLOR,
        inputs: [{ id: 'in', label: 'in' }],
        outputs: [{ id: 'rows', label: 'rows' }, { id: 'error', label: 'error' }],
        configFields: [{ key: 'query', label: 'SQL Query', type: 'textarea' }],
      },
      {
        subtype: 'mongodb', label: 'MongoDB', color: DB_COLOR,
        inputs: [{ id: 'in', label: 'in' }],
        outputs: [{ id: 'docs', label: 'docs' }, { id: 'error', label: 'error' }],
        configFields: [
          { key: 'collection', label: 'Collection', type: 'text' },
          { key: 'operation', label: 'Operation', type: 'text' },
        ],
      },
      {
        subtype: 'redis', label: 'Redis', color: DB_COLOR,
        inputs: [{ id: 'in', label: 'in' }],
        outputs: [{ id: 'result', label: 'result' }, { id: 'error', label: 'error' }],
        configFields: [{ key: 'command', label: 'Command', type: 'text' }],
      },
    ],
  },
  {
    id: 'communication',
    label: 'COMMUNICATION',
    color: COMM_COLOR,
    nodes: [
      {
        subtype: 'email_send', label: 'Send Email', color: COMM_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [],
        configFields: [
          { key: 'to', label: 'To', type: 'text' },
          { key: 'subject', label: 'Subject', type: 'text' },
          { key: 'body', label: 'Body', type: 'textarea' },
        ],
      },
      {
        subtype: 'email_read', label: 'Read Email', color: COMM_COLOR,
        inputs: [], outputs: [{ id: 'emails', label: 'emails' }],
        configFields: [{ key: 'folder', label: 'Folder', type: 'text', default: 'INBOX' }],
      },
      {
        subtype: 'slack', label: 'Slack', color: COMM_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [
          { key: 'channel', label: 'Channel', type: 'text' },
          { key: 'message', label: 'Message', type: 'text' },
        ],
      },
      {
        subtype: 'telegram', label: 'Telegram', color: COMM_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [
          { key: 'chatId', label: 'Chat ID', type: 'text' },
          { key: 'message', label: 'Message', type: 'text' },
        ],
      },
      {
        subtype: 'discord', label: 'Discord', color: COMM_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [
          { key: 'channel', label: 'Channel ID', type: 'text' },
          { key: 'message', label: 'Message', type: 'text' },
        ],
      },
      {
        subtype: 'twilio', label: 'Twilio SMS', color: COMM_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [],
        configFields: [
          { key: 'to', label: 'Phone Number', type: 'text' },
          { key: 'message', label: 'Message', type: 'text' },
        ],
      },
      {
        subtype: 'whatsapp', label: 'WhatsApp', color: COMM_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [],
        configFields: [
          { key: 'to', label: 'Phone Number', type: 'text' },
          { key: 'message', label: 'Message', type: 'text' },
        ],
      },
    ],
  },
  {
    id: 'services',
    label: 'SERVICES',
    color: SERVICES_COLOR,
    nodes: [
      {
        subtype: 'github', label: 'GitHub', color: SERVICES_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [
          { key: 'operation', label: 'Operation', type: 'text' },
          { key: 'repo', label: 'Repo (owner/name)', type: 'text' },
        ],
      },
      {
        subtype: 'notion', label: 'Notion', color: SERVICES_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [
          { key: 'operation', label: 'Operation', type: 'text' },
          { key: 'database_id', label: 'Database ID', type: 'text' },
        ],
      },
      {
        subtype: 'airtable', label: 'Airtable', color: SERVICES_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [
          { key: 'base_id', label: 'Base ID', type: 'text' },
          { key: 'table', label: 'Table', type: 'text' },
        ],
      },
      {
        subtype: 'jira', label: 'Jira', color: SERVICES_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'operation', label: 'Operation', type: 'text' }],
      },
      {
        subtype: 'linear', label: 'Linear', color: SERVICES_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'operation', label: 'Operation', type: 'text' }],
      },
      {
        subtype: 'asana', label: 'Asana', color: SERVICES_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'operation', label: 'Operation', type: 'text' }],
      },
      {
        subtype: 'stripe', label: 'Stripe', color: SERVICES_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'operation', label: 'Operation', type: 'text' }],
      },
      {
        subtype: 'shopify', label: 'Shopify', color: SERVICES_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'operation', label: 'Operation', type: 'text' }],
      },
      {
        subtype: 'salesforce', label: 'Salesforce', color: SERVICES_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'operation', label: 'Operation', type: 'text' }],
      },
      {
        subtype: 'hubspot', label: 'HubSpot', color: SERVICES_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'operation', label: 'Operation', type: 'text' }],
      },
      {
        subtype: 'google_sheets', label: 'Google Sheets', color: SERVICES_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'spreadsheet_id', label: 'Spreadsheet ID', type: 'text' }],
      },
      {
        subtype: 'gmail', label: 'Gmail', color: SERVICES_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'operation', label: 'Operation', type: 'text' }],
      },
      {
        subtype: 'google_drive', label: 'Google Drive', color: SERVICES_COLOR,
        inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'out', label: 'out' }],
        configFields: [{ key: 'operation', label: 'Operation', type: 'text' }],
      },
    ],
  },
  {
    id: 'instagram',
    label: 'INSTAGRAM',
    color: PCOL.instagram,
    nodes: [
      { subtype: 'instagram.find_by_keyword', label: 'Find by Keyword', platform: 'instagram', inputs: [{ id: 'keywords', label: 'keywords' }], outputs: [{ id: 'profiles', label: 'profiles' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'maxCount', label: 'Max Count', type: 'number', default: '50' }] },
      { subtype: 'instagram.export_followers', label: 'Export Followers', platform: 'instagram', inputs: [{ id: 'profiles', label: 'profiles' }], outputs: [{ id: 'followers', label: 'followers' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'maxCount', label: 'Max Count', type: 'number', default: '200' }] },
      { subtype: 'instagram.scrape_profile_info', label: 'Scrape Profile Info', platform: 'instagram', inputs: [{ id: 'profiles', label: 'profiles' }], outputs: [{ id: 'data', label: 'data' }, { id: 'errors', label: 'errors' }], configFields: [] },
      { subtype: 'instagram.engage_with_posts', label: 'Engage with Posts', platform: 'instagram', inputs: [{ id: 'items', label: 'items' }], outputs: [{ id: 'result', label: 'result' }, { id: 'errors', label: 'errors' }], configFields: [] },
      { subtype: 'instagram.send_dms', label: 'Send DMs', platform: 'instagram', inputs: [{ id: 'profiles', label: 'profiles' }, { id: 'message', label: 'message' }], outputs: [{ id: 'sent', label: 'sent' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'messageText', label: 'Message Text', type: 'text' }] },
      { subtype: 'instagram.auto_reply_dms', label: 'Auto Reply DMs', platform: 'instagram', inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'replied', label: 'replied' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'replyText', label: 'Reply Text', type: 'text' }] },
      { subtype: 'instagram.publish_post', label: 'Publish Post', platform: 'instagram', inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'result', label: 'result' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'caption', label: 'Caption', type: 'textarea' }] },
      { subtype: 'instagram.like_posts', label: 'Like Posts', platform: 'instagram', inputs: [{ id: 'items', label: 'items' }], outputs: [{ id: 'liked', label: 'liked' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'maxCount', label: 'Max Count', type: 'number', default: '100' }] },
      { subtype: 'instagram.comment_on_posts', label: 'Comment on Posts', platform: 'instagram', inputs: [{ id: 'items', label: 'items' }, { id: 'text', label: 'comment text' }], outputs: [{ id: 'commented', label: 'commented' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'commentText', label: 'Comment Text', type: 'text' }] },
      { subtype: 'instagram.follow_users', label: 'Follow Users', platform: 'instagram', inputs: [{ id: 'profiles', label: 'profiles' }], outputs: [{ id: 'followed', label: 'followed' }, { id: 'errors', label: 'errors' }], configFields: [] },
      { subtype: 'instagram.unfollow_users', label: 'Unfollow Users', platform: 'instagram', inputs: [{ id: 'profiles', label: 'profiles' }], outputs: [{ id: 'unfollowed', label: 'unfollowed' }, { id: 'errors', label: 'errors' }], configFields: [] },
      { subtype: 'instagram.watch_stories', label: 'Watch Stories', platform: 'instagram', inputs: [{ id: 'profiles', label: 'profiles' }], outputs: [{ id: 'viewed', label: 'viewed' }, { id: 'errors', label: 'errors' }], configFields: [] },
    ],
  },
  {
    id: 'linkedin',
    label: 'LINKEDIN',
    color: PCOL.linkedin,
    nodes: [
      { subtype: 'linkedin.find_by_keyword', label: 'Find by Keyword', platform: 'linkedin', inputs: [{ id: 'keywords', label: 'keywords' }], outputs: [{ id: 'profiles', label: 'profiles' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'maxCount', label: 'Max Count', type: 'number', default: '50' }] },
      { subtype: 'linkedin.export_followers', label: 'Export Followers', platform: 'linkedin', inputs: [{ id: 'profiles', label: 'profiles' }], outputs: [{ id: 'followers', label: 'followers' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'maxCount', label: 'Max Count', type: 'number', default: '200' }] },
      { subtype: 'linkedin.scrape_profile_info', label: 'Scrape Profile Info', platform: 'linkedin', inputs: [{ id: 'profiles', label: 'profiles' }], outputs: [{ id: 'data', label: 'data' }, { id: 'errors', label: 'errors' }], configFields: [] },
      { subtype: 'linkedin.engage_with_posts', label: 'Engage with Posts', platform: 'linkedin', inputs: [{ id: 'items', label: 'items' }], outputs: [{ id: 'result', label: 'result' }, { id: 'errors', label: 'errors' }], configFields: [] },
      { subtype: 'linkedin.send_dms', label: 'Send DMs', platform: 'linkedin', inputs: [{ id: 'profiles', label: 'profiles' }, { id: 'message', label: 'message' }], outputs: [{ id: 'sent', label: 'sent' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'messageText', label: 'Message Text', type: 'text' }] },
      { subtype: 'linkedin.auto_reply_dms', label: 'Auto Reply DMs', platform: 'linkedin', inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'replied', label: 'replied' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'replyText', label: 'Reply Text', type: 'text' }] },
      { subtype: 'linkedin.publish_post', label: 'Publish Post', platform: 'linkedin', inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'result', label: 'result' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'content', label: 'Post Content', type: 'textarea' }] },
    ],
  },
  {
    id: 'x',
    label: 'X / TWITTER',
    color: PCOL.x,
    nodes: [
      { subtype: 'x.find_by_keyword', label: 'Find by Keyword', platform: 'x', inputs: [{ id: 'keywords', label: 'keywords' }], outputs: [{ id: 'profiles', label: 'profiles' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'maxCount', label: 'Max Count', type: 'number', default: '50' }] },
      { subtype: 'x.export_followers', label: 'Export Followers', platform: 'x', inputs: [{ id: 'profiles', label: 'profiles' }], outputs: [{ id: 'followers', label: 'followers' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'maxCount', label: 'Max Count', type: 'number', default: '200' }] },
      { subtype: 'x.scrape_profile_info', label: 'Scrape Profile Info', platform: 'x', inputs: [{ id: 'profiles', label: 'profiles' }], outputs: [{ id: 'data', label: 'data' }, { id: 'errors', label: 'errors' }], configFields: [] },
      { subtype: 'x.engage_with_posts', label: 'Engage with Posts', platform: 'x', inputs: [{ id: 'items', label: 'items' }], outputs: [{ id: 'result', label: 'result' }, { id: 'errors', label: 'errors' }], configFields: [] },
      { subtype: 'x.send_dms', label: 'Send DMs', platform: 'x', inputs: [{ id: 'profiles', label: 'profiles' }, { id: 'message', label: 'message' }], outputs: [{ id: 'sent', label: 'sent' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'messageText', label: 'Message Text', type: 'text' }] },
      { subtype: 'x.auto_reply_dms', label: 'Auto Reply DMs', platform: 'x', inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'replied', label: 'replied' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'replyText', label: 'Reply Text', type: 'text' }] },
      { subtype: 'x.publish_post', label: 'Publish Post', platform: 'x', inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'result', label: 'result' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'text', label: 'Tweet Text', type: 'textarea' }] },
    ],
  },
  {
    id: 'tiktok',
    label: 'TIKTOK',
    color: PCOL.tiktok,
    nodes: [
      { subtype: 'tiktok.find_by_keyword', label: 'Find by Keyword', platform: 'tiktok', inputs: [{ id: 'keywords', label: 'keywords' }], outputs: [{ id: 'profiles', label: 'profiles' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'maxCount', label: 'Max Count', type: 'number', default: '50' }] },
      { subtype: 'tiktok.export_followers', label: 'Export Followers', platform: 'tiktok', inputs: [{ id: 'profiles', label: 'profiles' }], outputs: [{ id: 'followers', label: 'followers' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'maxCount', label: 'Max Count', type: 'number', default: '200' }] },
      { subtype: 'tiktok.scrape_profile_info', label: 'Scrape Profile Info', platform: 'tiktok', inputs: [{ id: 'profiles', label: 'profiles' }], outputs: [{ id: 'data', label: 'data' }, { id: 'errors', label: 'errors' }], configFields: [] },
      { subtype: 'tiktok.engage_with_posts', label: 'Engage with Posts', platform: 'tiktok', inputs: [{ id: 'items', label: 'items' }], outputs: [{ id: 'result', label: 'result' }, { id: 'errors', label: 'errors' }], configFields: [] },
      { subtype: 'tiktok.send_dms', label: 'Send DMs', platform: 'tiktok', inputs: [{ id: 'profiles', label: 'profiles' }, { id: 'message', label: 'message' }], outputs: [{ id: 'sent', label: 'sent' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'messageText', label: 'Message Text', type: 'text' }] },
      { subtype: 'tiktok.auto_reply_dms', label: 'Auto Reply DMs', platform: 'tiktok', inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'replied', label: 'replied' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'replyText', label: 'Reply Text', type: 'text' }] },
      { subtype: 'tiktok.publish_post', label: 'Publish Post', platform: 'tiktok', inputs: [{ id: 'in', label: 'in' }], outputs: [{ id: 'result', label: 'result' }, { id: 'errors', label: 'errors' }], configFields: [{ key: 'caption', label: 'Caption', type: 'textarea' }] },
    ],
  },
]

// Flat lookup for hydrating nodes from backend
const NODE_TYPE_MAP = {}
NODE_CATEGORIES.forEach(cat => {
  cat.nodes.forEach(n => { NODE_TYPE_MAP[n.subtype] = n })
})

// ── Geometry helpers ─────────────────────────────────────────────────
function nodeH(n) {
  return HEAD_H + PORT_PAD + Math.max(n.inputs.length, n.outputs.length, 1) * PORT_H + PORT_PAD
}
function inPortPos(n, i) {
  return { x: n.x, y: n.y + HEAD_H + PORT_PAD + i * PORT_H + PORT_H / 2 }
}
function outPortPos(n, i) {
  return { x: n.x + NODE_W, y: n.y + HEAD_H + PORT_PAD + i * PORT_H + PORT_H / 2 }
}
function edgePath(sx, sy, tx, ty) {
  const dx = Math.max(60, Math.abs(tx - sx) * 0.5)
  return `M${sx},${sy} C${sx + dx},${sy} ${tx - dx},${ty} ${tx},${ty}`
}

// ── ID generator ─────────────────────────────────────────────────────
let _seq = 1
const uid = () => `wf${_seq++}_${Math.random().toString(36).slice(2, 6)}`

// ── Data mapping: canvas ↔ backend ───────────────────────────────────
function canvasToBackend(workflowId, workflowName, nodes, edges) {
  return {
    id: workflowId || '',
    name: workflowName,
    description: '',
    nodes: nodes.map(n => ({
      id: n.id,
      node_type: n.platform ? `${n.platform}.${n.subtype.split('.').pop().toLowerCase()}` : n.subtype,
      name: n.label,
      config: n.config || {},
      position_x: Math.round(n.x),
      position_y: Math.round(n.y),
      disabled: false,
    })),
    connections: edges.map(e => ({
      id: e.id,
      source_node_id: e.source,
      source_handle: e.sourcePortId || 'main',
      target_node_id: e.target,
      target_handle: e.targetPortId || 'main',
      position: 0,
    })),
  }
}

function backendToCanvas(workflow) {
  const nodes = (workflow.nodes || []).map(n => {
    const tpl = NODE_TYPE_MAP[n.node_type] || {
      subtype: n.node_type, label: n.name || n.node_type,
      inputs: [], outputs: [], configFields: [], color: PCOL.default,
    }
    return {
      id: n.id,
      type: tpl.platform ? 'action' : (n.node_type.startsWith('trigger') ? 'trigger' : 'data'),
      subtype: tpl.subtype,
      label: n.name || tpl.label,
      platform: tpl.platform || null,
      color: tpl.color || (tpl.platform ? PCOL[tpl.platform] : PCOL.default),
      inputs: tpl.inputs || [],
      outputs: tpl.outputs || [],
      configFields: tpl.configFields || [],
      config: n.config || {},
      x: n.position_x || 100,
      y: n.position_y || 100,
    }
  })
  const edges = (workflow.connections || []).map(c => ({
    id: c.id || uid(),
    source: c.source_node_id,
    sourcePortIdx: 0,
    sourcePortId: c.source_handle,
    target: c.target_node_id,
    targetPortIdx: 0,
    targetPortId: c.target_handle,
  }))
  return { nodes, edges }
}

// ── Time formatter ───────────────────────────────────────────────────
function timeAgo(isoStr) {
  if (!isoStr) return '—'
  const d = new Date(isoStr)
  if (isNaN(d)) return '—'
  const secs = Math.floor((Date.now() - d) / 1000)
  if (secs < 60) return `${secs}s ago`
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`
  return `${Math.floor(secs / 86400)}d ago`
}

// ════════════════════════════════════════════════════════════════════
// WorkflowManager — list view
// ════════════════════════════════════════════════════════════════════
function WorkflowManager({ onSelect, onNew }) {
  const [workflows, setWorkflows] = useState([])
  const [loading, setLoading]     = useState(true)
  const [error, setError]         = useState(null)
  const [deleting, setDeleting]   = useState(null)
  const [running, setRunning]     = useState(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const list = await _ListWorkflows()
      setWorkflows(Array.isArray(list) ? list : [])
    } catch (e) {
      setError(e?.message || 'Failed to load workflows')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  const handleDelete = async (e, id) => {
    e.stopPropagation()
    if (!window.confirm('Delete this workflow? This cannot be undone.')) return
    setDeleting(id)
    try {
      await _DeleteWorkflow(id)
      setWorkflows(prev => prev.filter(w => w.id !== id))
    } catch (err) {
      alert('Failed to delete: ' + (err?.message || err))
    } finally {
      setDeleting(null)
    }
  }

  const handleRun = async (e, id) => {
    e.stopPropagation()
    setRunning(id)
    try {
      await _RunWorkflow(id)
      await load()
    } catch (err) {
      alert('Run failed: ' + (err?.message || err))
    } finally {
      setRunning(null)
    }
  }

  const handleToggleActive = async (e, wf) => {
    e.stopPropagation()
    try {
      await _SetWorkflowActive(wf.id, !wf.active)
      setWorkflows(prev => prev.map(w => w.id === wf.id ? { ...w, active: !wf.active } : w))
    } catch (err) {
      alert('Toggle failed: ' + (err?.message || err))
    }
  }

  return (
    <div style={{
      display: 'flex', flexDirection: 'column', height: '100%',
      background: '#04060a', overflow: 'hidden',
    }}>
      {/* Header */}
      <div style={{
        height: 52, flexShrink: 0, display: 'flex', alignItems: 'center',
        padding: '0 20px', gap: 10,
        background: '#080d16', borderBottom: '1px solid rgba(0,180,216,0.1)',
      }}>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 700, color: '#dde6f0', letterSpacing: 2, flex: 1 }}>
          ◈ WORKFLOWS
        </span>
        <button
          style={{
            display: 'flex', alignItems: 'center', gap: 6,
            background: 'rgba(0,180,216,0.1)', border: '1px solid rgba(0,180,216,0.3)',
            borderRadius: 6, padding: '6px 14px', color: '#00b4d8',
            fontFamily: 'var(--font-mono)', fontSize: 11, cursor: 'pointer',
            letterSpacing: 0.5, transition: 'all 120ms',
          }}
          onMouseEnter={e => e.currentTarget.style.background = 'rgba(0,180,216,0.18)'}
          onMouseLeave={e => e.currentTarget.style.background = 'rgba(0,180,216,0.1)'}
          onClick={onNew}
        >
          <Plus size={13} /> New Workflow
        </button>
        <button
          style={{
            display: 'flex', alignItems: 'center', gap: 5,
            background: 'transparent', border: '1px solid rgba(0,180,216,0.15)',
            borderRadius: 6, padding: '6px 10px', color: 'var(--text-muted)',
            cursor: 'pointer', transition: 'all 120ms',
          }}
          onMouseEnter={e => e.currentTarget.style.color = '#00b4d8'}
          onMouseLeave={e => e.currentTarget.style.color = 'var(--text-muted)'}
          onClick={load}
          title="Refresh"
        >
          <RefreshCw size={13} />
        </button>
      </div>

      {/* Body */}
      <div style={{ flex: 1, overflowY: 'auto', padding: '24px 24px' }}>
        {loading && (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 10, padding: 60, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
            <Loader size={16} style={{ animation: 'spin 1s linear infinite' }} /> Loading workflows...
          </div>
        )}
        {error && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '12px 16px', background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.2)', borderRadius: 8, color: '#ef4444', fontFamily: 'var(--font-mono)', fontSize: 11, marginBottom: 16 }}>
            <AlertCircle size={14} /> {error}
          </div>
        )}
        {!loading && !error && workflows.length === 0 && (
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 16, padding: 80, color: 'var(--text-muted)' }}>
            <div style={{ fontSize: 48, opacity: 0.06, fontFamily: 'var(--font-mono)', color: '#00b4d8' }}>⬡⬡⬡</div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, letterSpacing: 2, textTransform: 'uppercase' }}>No workflows yet</div>
            <button
              style={{
                display: 'flex', alignItems: 'center', gap: 6,
                background: 'rgba(0,180,216,0.1)', border: '1px solid rgba(0,180,216,0.3)',
                borderRadius: 6, padding: '8px 18px', color: '#00b4d8',
                fontFamily: 'var(--font-mono)', fontSize: 11, cursor: 'pointer',
              }}
              onClick={onNew}
            >
              <Plus size={13} /> Create your first workflow
            </button>
          </div>
        )}
        {!loading && workflows.length > 0 && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6, maxWidth: 860 }}>
            {workflows.map(wf => (
              <WorkflowRow
                key={wf.id}
                wf={wf}
                running={running === wf.id}
                deleting={deleting === wf.id}
                onClick={() => onSelect(wf.id)}
                onRun={(e) => handleRun(e, wf.id)}
                onDelete={(e) => handleDelete(e, wf.id)}
                onToggleActive={(e) => handleToggleActive(e, wf)}
              />
            ))}
          </div>
        )}
      </div>

      <style>{`
        @keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }
      `}</style>
    </div>
  )
}

function WorkflowRow({ wf, running, deleting, onClick, onRun, onDelete, onToggleActive }) {
  const [hov, setHov] = useState(false)

  return (
    <div
      style={{
        display: 'flex', alignItems: 'center', gap: 12,
        padding: '12px 16px',
        background: hov ? 'rgba(0,180,216,0.05)' : '#080d16',
        border: `1px solid ${hov ? 'rgba(0,180,216,0.2)' : 'rgba(0,180,216,0.08)'}`,
        borderRadius: 8, cursor: 'pointer', transition: 'all 140ms',
      }}
      onMouseEnter={() => setHov(true)}
      onMouseLeave={() => setHov(false)}
      onClick={onClick}
    >
      {/* Status dot */}
      <div style={{
        width: 7, height: 7, borderRadius: '50%', flexShrink: 0,
        background: wf.active ? '#10b981' : 'rgba(100,120,140,0.3)',
        boxShadow: wf.active ? '0 0 6px #10b98188' : 'none',
      }} />

      {/* Name */}
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: '#dde6f0', fontWeight: 600, letterSpacing: 0.3, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {wf.name || 'Untitled Workflow'}
        </div>
        {wf.description && (
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {wf.description}
          </div>
        )}
      </div>

      {/* Active badge */}
      <button
        style={{
          display: 'flex', alignItems: 'center', gap: 5,
          padding: '3px 8px', borderRadius: 20,
          background: wf.active ? 'rgba(16,185,129,0.12)' : 'rgba(100,120,140,0.1)',
          border: `1px solid ${wf.active ? 'rgba(16,185,129,0.3)' : 'rgba(100,120,140,0.2)'}`,
          color: wf.active ? '#10b981' : 'var(--text-muted)',
          fontFamily: 'var(--font-mono)', fontSize: 9, cursor: 'pointer',
          letterSpacing: 1, textTransform: 'uppercase', transition: 'all 120ms',
        }}
        onClick={onToggleActive}
        title={wf.active ? 'Deactivate' : 'Activate'}
      >
        {wf.active ? <ToggleRight size={12} /> : <ToggleLeft size={12} />}
        {wf.active ? 'Active' : 'Draft'}
      </button>

      {/* Version */}
      {wf.version != null && (
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 0.5, flexShrink: 0 }}>
          v{wf.version}
        </span>
      )}

      {/* Time */}
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', flexShrink: 0, minWidth: 56 }}>
        <Clock size={9} style={{ verticalAlign: 'middle', marginRight: 4 }} />
        {timeAgo(wf.updated_at)}
      </span>

      {/* Node count */}
      {Array.isArray(wf.nodes) && (
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', flexShrink: 0 }}>
          {wf.nodes.length} nodes
        </span>
      )}

      {/* Run button */}
      <button
        style={{
          display: 'flex', alignItems: 'center', gap: 5,
          padding: '5px 11px', borderRadius: 5,
          background: running ? 'rgba(0,180,216,0.15)' : 'rgba(16,185,129,0.08)',
          border: `1px solid ${running ? 'rgba(0,180,216,0.3)' : 'rgba(16,185,129,0.25)'}`,
          color: running ? '#00b4d8' : '#10b981',
          fontFamily: 'var(--font-mono)', fontSize: 10, cursor: 'pointer',
          letterSpacing: 0.5, transition: 'all 120ms', flexShrink: 0,
        }}
        onClick={onRun}
        disabled={running}
        title="Run workflow"
      >
        {running
          ? <><Loader size={11} style={{ animation: 'spin 1s linear infinite' }} /> Running</>
          : <><Play size={11} /> Run</>}
      </button>

      {/* Delete button */}
      <button
        style={{
          display: 'flex', alignItems: 'center',
          padding: '5px 7px', borderRadius: 5,
          background: 'transparent',
          border: '1px solid rgba(239,68,68,0.15)',
          color: deleting ? '#ef4444' : 'rgba(239,68,68,0.5)',
          cursor: 'pointer', transition: 'all 120ms', flexShrink: 0,
        }}
        onMouseEnter={e => { e.currentTarget.style.background = 'rgba(239,68,68,0.1)'; e.currentTarget.style.color = '#ef4444' }}
        onMouseLeave={e => { e.currentTarget.style.background = 'transparent'; e.currentTarget.style.color = 'rgba(239,68,68,0.5)' }}
        onClick={onDelete}
        disabled={deleting}
        title="Delete workflow"
      >
        {deleting ? <Loader size={12} style={{ animation: 'spin 1s linear infinite' }} /> : <Trash2 size={12} />}
      </button>
    </div>
  )
}

// ════════════════════════════════════════════════════════════════════
// NodePalette — collapsible sections + search
// ════════════════════════════════════════════════════════════════════
function NodePalette({ onAdd }) {
  const [search, setSearch] = useState('')
  const [openSections, setOpenSections] = useState(() => {
    try {
      const s = localStorage.getItem('monoes-wf-palette-sections')
      return s ? JSON.parse(s) : {}
    } catch { return {} }
  })

  const toggleSection = (id) => {
    setOpenSections(prev => {
      const next = { ...prev, [id]: !prev[id] }
      try { localStorage.setItem('monoes-wf-palette-sections', JSON.stringify(next)) } catch {}
      return next
    })
  }

  const q = search.trim().toLowerCase()

  const filteredCategories = NODE_CATEGORIES.map(cat => ({
    ...cat,
    nodes: q
      ? cat.nodes.filter(n =>
          n.label.toLowerCase().includes(q) ||
          n.subtype.toLowerCase().includes(q) ||
          (n.platform || '').toLowerCase().includes(q)
        )
      : cat.nodes,
  })).filter(cat => cat.nodes.length > 0)

  return (
    <div style={{
      width: 220, flexShrink: 0,
      background: '#080d16',
      borderRight: '1px solid rgba(0,180,216,0.1)',
      display: 'flex', flexDirection: 'column', overflow: 'hidden',
    }}>
      {/* Header */}
      <div style={{
        padding: '12px 12px 8px',
        borderBottom: '1px solid rgba(0,180,216,0.08)',
        flexShrink: 0,
      }}>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 2, textTransform: 'uppercase', marginBottom: 8 }}>
          NODE PALETTE
        </div>
        {/* Search */}
        <div style={{ position: 'relative' }}>
          <Search size={10} style={{ position: 'absolute', left: 8, top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input
            style={{
              width: '100%', boxSizing: 'border-box',
              background: '#0d1a28', border: '1px solid rgba(0,180,216,0.15)',
              borderRadius: 5, padding: '5px 8px 5px 24px',
              color: '#dde6f0', fontFamily: 'var(--font-mono)', fontSize: 10,
              outline: 'none',
            }}
            placeholder="Search nodes..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            onFocus={e => e.currentTarget.style.borderColor = 'rgba(0,180,216,0.4)'}
            onBlur={e => e.currentTarget.style.borderColor = 'rgba(0,180,216,0.15)'}
          />
        </div>
      </div>

      <div style={{ overflowY: 'auto', flex: 1, padding: '4px 0 12px' }}>
        {filteredCategories.map(cat => {
          const isOpen = q ? true : (openSections[cat.id] !== false)
          return (
            <div key={cat.id}>
              {/* Section header */}
              <div
                style={{
                  display: 'flex', alignItems: 'center', gap: 6,
                  padding: '8px 12px 5px',
                  cursor: 'pointer', userSelect: 'none',
                  fontFamily: 'var(--font-mono)', fontSize: 9,
                  color: cat.color, letterSpacing: 2, textTransform: 'uppercase',
                  transition: 'opacity 100ms',
                }}
                onClick={() => !q && toggleSection(cat.id)}
              >
                <span style={{ flex: 1 }}>{cat.label}</span>
                {!q && (
                  isOpen
                    ? <ChevronDown size={9} />
                    : <ChevronRight size={9} />
                )}
                <span style={{ fontSize: 8, opacity: 0.5 }}>{cat.nodes.length}</span>
              </div>

              {/* Nodes */}
              {isOpen && cat.nodes.map(t => (
                <PaletteItem
                  key={t.subtype}
                  template={t}
                  type={t.platform ? 'action' : (t.subtype.startsWith('trigger') ? 'trigger' : 'data')}
                  onAdd={onAdd}
                />
              ))}
            </div>
          )
        })}

        {filteredCategories.length === 0 && (
          <div style={{ padding: '24px 12px', textAlign: 'center', fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)' }}>
            No nodes match "{search}"
          </div>
        )}
      </div>
    </div>
  )
}

function PaletteItem({ template, type, onAdd }) {
  const [hov, setHov] = useState(false)
  const color = template.color || (template.platform ? PCOL[template.platform] : PCOL.default)
  return (
    <div
      style={{
        padding: '6px 12px',
        borderRadius: 5, margin: '0 6px 1px',
        cursor: 'pointer',
        background: hov ? `${color}14` : 'transparent',
        border: `1px solid ${hov ? `${color}30` : 'transparent'}`,
        transition: 'all 110ms',
        display: 'flex', alignItems: 'center', gap: 8,
      }}
      onMouseEnter={() => setHov(true)}
      onMouseLeave={() => setHov(false)}
      onClick={() => onAdd(type, template)}
    >
      <div style={{
        width: 5, height: 5, borderRadius: '50%',
        background: color, flexShrink: 0,
        boxShadow: hov ? `0 0 5px ${color}` : 'none',
        transition: 'box-shadow 110ms',
      }} />
      <div style={{ minWidth: 0 }}>
        <div style={{
          fontFamily: 'var(--font-mono)', fontSize: 10,
          color: hov ? '#dde6f0' : 'var(--text-secondary)',
          transition: 'color 110ms', letterSpacing: 0.2,
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>
          {template.label}
        </div>
        {template.platform && (
          <div style={{ fontSize: 8, fontFamily: 'var(--font-mono)', color, opacity: 0.65, letterSpacing: 0.5, textTransform: 'uppercase' }}>
            {template.platform}
          </div>
        )}
      </div>
    </div>
  )
}

// ════════════════════════════════════════════════════════════════════
// WorkflowNode
// ════════════════════════════════════════════════════════════════════
function WorkflowNode({
  node, selected, zoom,
  onHeaderMouseDown, onOutputPortMouseDown, onInputPortMouseUp,
  onClick, onDelete,
}) {
  const h     = nodeH(node)
  const rows  = Math.max(node.inputs.length, node.outputs.length, 1)
  const color = node.color || PCOL.default

  return (
    <div
      style={{
        position: 'absolute', left: node.x, top: node.y,
        width: NODE_W, height: h,
        background: 'linear-gradient(160deg, #0d1a28 0%, #091220 100%)',
        border: `1px solid ${selected ? color : 'rgba(0,180,216,0.12)'}`,
        borderRadius: 10,
        boxShadow: selected
          ? `0 0 0 1.5px ${color}55, 0 12px 32px rgba(0,0,0,0.7), 0 0 28px ${color}18`
          : '0 6px 20px rgba(0,0,0,0.5)',
        userSelect: 'none', overflow: 'visible',
        transition: 'border-color 140ms, box-shadow 140ms',
      }}
      onMouseDown={(e) => { e.stopPropagation(); onClick?.() }}
    >
      {/* Header */}
      <div
        style={{
          height: HEAD_H,
          background: `linear-gradient(110deg, ${color}1a 0%, ${color}0a 100%)`,
          borderBottom: `1px solid ${color}22`,
          borderRadius: '10px 10px 0 0',
          display: 'flex', alignItems: 'center',
          padding: '0 10px 0 10px', cursor: 'grab', gap: 7,
        }}
        onMouseDown={(e) => { e.stopPropagation(); onHeaderMouseDown(e) }}
      >
        <div style={{ width: 7, height: 7, borderRadius: '50%', background: color, boxShadow: `0 0 8px ${color}`, flexShrink: 0 }} />
        <span style={{
          flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
          fontFamily: 'var(--font-mono)', fontSize: 11, fontWeight: 700,
          color: '#dde6f0', letterSpacing: 0.5, textTransform: 'uppercase',
        }}>
          {node.label}
        </span>
        {node.platform && (
          <span style={{ fontSize: 9, fontFamily: 'var(--font-mono)', padding: '2px 5px', borderRadius: 3, background: `${color}22`, color, textTransform: 'uppercase', letterSpacing: 1 }}>
            {node.platform.slice(0, 2).toUpperCase()}
          </span>
        )}
        <span style={{
          fontSize: 9, fontFamily: 'var(--font-mono)', padding: '2px 5px', borderRadius: 3,
          background: node.type === 'data' ? 'rgba(217,119,6,0.15)' : (node.type === 'trigger' ? 'rgba(124,58,237,0.15)' : 'rgba(0,180,216,0.1)'),
          color: node.type === 'data' ? '#d97706' : (node.type === 'trigger' ? '#7c3aed' : '#00b4d8'),
          textTransform: 'uppercase', letterSpacing: 1,
        }}>
          {node.type}
        </span>
        {selected && (
          <button
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'rgba(100,120,140,0.6)', padding: '2px 1px', display: 'flex', borderRadius: 3, flexShrink: 0 }}
            onMouseDown={e => e.stopPropagation()}
            onClick={e => { e.stopPropagation(); onDelete() }}
            title="Delete node"
          >
            <X size={12} />
          </button>
        )}
      </div>

      {/* Port rows */}
      <div style={{ padding: `${PORT_PAD}px 0` }}>
        {Array.from({ length: rows }).map((_, i) => {
          const inp = node.inputs[i]
          const out = node.outputs[i]
          return (
            <div key={i} style={{ height: PORT_H, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <div style={{ display: 'flex', alignItems: 'center', minWidth: '45%', paddingLeft: 0 }}>
                {inp && (
                  <>
                    <div
                      className="wf-port-in"
                      style={{
                        width: PORT_R * 2, height: PORT_R * 2, borderRadius: '50%',
                        background: '#0d1a28', border: '2px solid rgba(0,180,216,0.35)',
                        marginLeft: -PORT_R, cursor: 'crosshair', flexShrink: 0,
                        boxSizing: 'border-box', zIndex: 3, position: 'relative',
                        transition: 'border-color 120ms, box-shadow 120ms',
                      }}
                      onMouseUp={e => { e.stopPropagation(); onInputPortMouseUp?.(e, i) }}
                    />
                    <span style={{ marginLeft: 8, fontSize: 10, fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: 0.2 }}>
                      {inp.label}
                    </span>
                  </>
                )}
              </div>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', minWidth: '45%', paddingRight: 0 }}>
                {out && (
                  <>
                    <span style={{ marginRight: 8, fontSize: 10, fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: 0.2 }}>
                      {out.label}
                    </span>
                    <div
                      className="wf-port-out"
                      style={{
                        width: PORT_R * 2, height: PORT_R * 2, borderRadius: '50%',
                        background: color, marginRight: -PORT_R, cursor: 'crosshair',
                        flexShrink: 0, boxShadow: `0 0 7px ${color}99`,
                        boxSizing: 'border-box', zIndex: 3, position: 'relative',
                        transition: 'box-shadow 120ms',
                      }}
                      onMouseDown={e => { e.stopPropagation(); onOutputPortMouseDown?.(e, i) }}
                    />
                  </>
                )}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

// ════════════════════════════════════════════════════════════════════
// NodeInspector
// ════════════════════════════════════════════════════════════════════
function NodeInspector({ node, onUpdateLabel, onUpdateConfig, onDelete, onClose }) {
  const color = node.color || PCOL.default

  return (
    <div style={{
      width: 264, flexShrink: 0,
      background: '#080d16', borderLeft: '1px solid rgba(0,180,216,0.1)',
      display: 'flex', flexDirection: 'column', overflow: 'hidden',
    }}>
      <div style={{
        padding: '12px 14px', borderBottom: `1px solid ${color}18`,
        display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0,
        background: `${color}08`,
      }}>
        <div style={{ width: 8, height: 8, borderRadius: '50%', background: color, boxShadow: `0 0 8px ${color}`, flexShrink: 0 }} />
        <div style={{ flex: 1 }}>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 2, marginBottom: 2 }}>
            {node.type} NODE
          </div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: '#dde6f0', fontWeight: 600 }}>
            {node.subtype}
          </div>
        </div>
        <button
          style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', padding: 4, display: 'flex', borderRadius: 4 }}
          onClick={onClose}
          title="Close inspector"
        >
          <ChevronRight size={14} />
        </button>
      </div>

      <div style={{ overflowY: 'auto', flex: 1, padding: 14 }}>
        <InspectorField label="LABEL">
          <input
            style={{
              width: '100%', background: '#0d1a28',
              border: '1px solid rgba(0,180,216,0.2)', borderRadius: 5,
              padding: '6px 9px', color: '#dde6f0',
              fontFamily: 'var(--font-mono)', fontSize: 11, outline: 'none',
            }}
            value={node.label}
            onChange={e => onUpdateLabel(node.id, e.target.value)}
            onFocus={e => e.currentTarget.style.borderColor = `${color}60`}
            onBlur={e => e.currentTarget.style.borderColor = 'rgba(0,180,216,0.2)'}
          />
        </InspectorField>

        {node.platform && (
          <InspectorField label="PLATFORM">
            <div style={{ display: 'flex', alignItems: 'center', gap: 7, padding: '5px 9px', background: '#0d1a28', borderRadius: 5, border: `1px solid ${color}22` }}>
              <div style={{ width: 6, height: 6, borderRadius: '50%', background: color, boxShadow: `0 0 6px ${color}` }} />
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color, textTransform: 'capitalize' }}>{node.platform}</span>
            </div>
          </InspectorField>
        )}

        {(node.inputs.length > 0 || node.outputs.length > 0) && (
          <InspectorField label="PORTS">
            <div style={{ background: '#0d1a28', border: '1px solid rgba(0,180,216,0.1)', borderRadius: 5, padding: '8px 9px', display: 'flex', flexDirection: 'column', gap: 5 }}>
              {node.inputs.map(p => (
                <div key={p.id} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <div style={{ width: 5, height: 5, borderRadius: '50%', border: '1.5px solid rgba(0,180,216,0.5)', flexShrink: 0 }} />
                  <span style={{ fontSize: 10, fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>← {p.label}</span>
                </div>
              ))}
              {node.outputs.map(p => (
                <div key={p.id} style={{ display: 'flex', alignItems: 'center', gap: 6, justifyContent: 'flex-end' }}>
                  <span style={{ fontSize: 10, fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>{p.label} →</span>
                  <div style={{ width: 5, height: 5, borderRadius: '50%', background: color, boxShadow: `0 0 5px ${color}80`, flexShrink: 0 }} />
                </div>
              ))}
            </div>
          </InspectorField>
        )}

        {node.configFields?.length > 0 && (
          <>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 2, textTransform: 'uppercase', margin: '14px 0 8px', paddingTop: 12, borderTop: '1px solid rgba(0,180,216,0.07)' }}>
              CONFIGURATION
            </div>
            {node.configFields.map(f => (
              <InspectorField key={f.key} label={f.label}>
                {f.type === 'textarea' ? (
                  <textarea
                    rows={5}
                    style={{
                      width: '100%', resize: 'vertical', background: '#0d1a28',
                      border: '1px solid rgba(0,180,216,0.18)', borderRadius: 5,
                      padding: '6px 9px', color: '#dde6f0',
                      fontFamily: 'var(--font-mono)', fontSize: 10, outline: 'none', lineHeight: 1.6,
                    }}
                    value={node.config?.[f.key] ?? ''}
                    onChange={e => onUpdateConfig(node.id, f.key, e.target.value)}
                    placeholder={`Enter ${f.label.toLowerCase()}...`}
                    onFocus={e => e.currentTarget.style.borderColor = `${color}50`}
                    onBlur={e => e.currentTarget.style.borderColor = 'rgba(0,180,216,0.18)'}
                  />
                ) : f.type === 'select' ? (
                  <select
                    style={{
                      width: '100%', background: '#0d1a28',
                      border: '1px solid rgba(0,180,216,0.18)', borderRadius: 5,
                      padding: '6px 9px', color: '#dde6f0',
                      fontFamily: 'var(--font-mono)', fontSize: 11, outline: 'none',
                    }}
                    value={node.config?.[f.key] ?? (f.options?.[0] ?? '')}
                    onChange={e => onUpdateConfig(node.id, f.key, e.target.value)}
                    onFocus={e => e.currentTarget.style.borderColor = `${color}50`}
                    onBlur={e => e.currentTarget.style.borderColor = 'rgba(0,180,216,0.18)'}
                  >
                    {(f.options || []).map(o => <option key={o} value={o}>{o}</option>)}
                  </select>
                ) : (
                  <input
                    type={f.type || 'text'}
                    style={{
                      width: '100%', background: '#0d1a28',
                      border: '1px solid rgba(0,180,216,0.18)', borderRadius: 5,
                      padding: '6px 9px', color: '#dde6f0',
                      fontFamily: 'var(--font-mono)', fontSize: 11, outline: 'none',
                    }}
                    value={node.config?.[f.key] ?? ''}
                    onChange={e => onUpdateConfig(node.id, f.key, e.target.value)}
                    placeholder={f.default ?? ''}
                    onFocus={e => e.currentTarget.style.borderColor = `${color}50`}
                    onBlur={e => e.currentTarget.style.borderColor = 'rgba(0,180,216,0.18)'}
                  />
                )}
              </InspectorField>
            ))}
          </>
        )}
      </div>

      <div style={{ padding: '10px 14px', borderTop: '1px solid rgba(0,180,216,0.07)', flexShrink: 0 }}>
        <button
          style={{
            width: '100%', background: 'rgba(239,68,68,0.08)',
            border: '1px solid rgba(239,68,68,0.2)', borderRadius: 6,
            padding: '7px 0', color: '#ef4444',
            fontFamily: 'var(--font-mono)', fontSize: 11, cursor: 'pointer',
            display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
            transition: 'background 120ms',
          }}
          onClick={onDelete}
          onMouseEnter={e => e.currentTarget.style.background = 'rgba(239,68,68,0.16)'}
          onMouseLeave={e => e.currentTarget.style.background = 'rgba(239,68,68,0.08)'}
        >
          <Trash2 size={12} /> Delete Node
        </button>
      </div>
    </div>
  )
}

function InspectorField({ label, children }) {
  return (
    <div style={{ marginBottom: 10 }}>
      <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 1.5, textTransform: 'uppercase', marginBottom: 5 }}>
        {label}
      </div>
      {children}
    </div>
  )
}

// ════════════════════════════════════════════════════════════════════
// ExecutionStatus banner
// ════════════════════════════════════════════════════════════════════
function ExecutionStatusBanner({ status, executionId, onDismiss }) {
  if (!status) return null

  const isRunning = status === 'running'
  const isSuccess = status === 'success' || status === 'completed'
  const isError   = status === 'error' || status === 'failed'

  const color = isRunning ? '#00b4d8' : isSuccess ? '#10b981' : '#ef4444'
  const bg    = isRunning ? 'rgba(0,180,216,0.08)' : isSuccess ? 'rgba(16,185,129,0.08)' : 'rgba(239,68,68,0.08)'
  const bdr   = isRunning ? 'rgba(0,180,216,0.25)' : isSuccess ? 'rgba(16,185,129,0.25)' : 'rgba(239,68,68,0.25)'
  const Icon  = isRunning ? Loader : isSuccess ? Check : AlertCircle
  const label = isRunning ? 'Workflow running...' : isSuccess ? 'Workflow completed successfully' : 'Workflow execution failed'

  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 8,
      padding: '7px 14px',
      background: bg, borderBottom: `1px solid ${bdr}`,
      color, fontFamily: 'var(--font-mono)', fontSize: 11,
      flexShrink: 0,
    }}>
      <Icon size={13} style={isRunning ? { animation: 'spin 1s linear infinite' } : {}} />
      <span style={{ flex: 1 }}>{label}{executionId ? ` (${executionId})` : ''}</span>
      {!isRunning && (
        <button
          style={{ background: 'none', border: 'none', cursor: 'pointer', color, padding: 2, display: 'flex' }}
          onClick={onDismiss}
        >
          <X size={12} />
        </button>
      )}
    </div>
  )
}

// ════════════════════════════════════════════════════════════════════
// WorkflowEditor
// ════════════════════════════════════════════════════════════════════
function WorkflowEditor({ workflowId, onBack }) {
  const [nodes, setNodes]           = useState([])
  const [edges, setEdges]           = useState([])
  const [workflowName, setWorkflowName] = useState('Untitled Workflow')
  const [workflowActive, setWorkflowActive] = useState(false)
  const [loadingWf, setLoadingWf]   = useState(true)
  const [selectedId, setSelectedId] = useState(null)
  const [camera, setCamera]         = useState({ x: 80, y: 80, zoom: 1 })
  const [paletteOpen, setPaletteOpen] = useState(true)
  const [pendingEdge, setPendingEdge] = useState(null)
  const [saved, setSaved]           = useState(false)
  const [saving, setSaving]         = useState(false)
  const [runStatus, setRunStatus]   = useState(null) // null | 'running' | 'success' | 'error'
  const [runExecId, setRunExecId]   = useState(null)
  const [editingName, setEditingName] = useState(false)
  const [nameInput, setNameInput]   = useState('')

  const wrapperRef = useRef(null)
  const dragRef    = useRef(null)
  const nodesRef   = useRef(nodes)
  const cameraRef  = useRef(camera)

  useEffect(() => { nodesRef.current = nodes }, [nodes])
  useEffect(() => { cameraRef.current = camera }, [camera])

  // Load workflow from backend
  useEffect(() => {
    if (!workflowId) { setLoadingWf(false); return }
    setLoadingWf(true)
    _GetWorkflow(workflowId).then(wf => {
      if (wf) {
        const { nodes: n, edges: e } = backendToCanvas(wf)
        setNodes(n)
        setEdges(e)
        setWorkflowName(wf.name || 'Untitled Workflow')
        setWorkflowActive(!!wf.active)
      }
    }).catch(() => {}).finally(() => setLoadingWf(false))
  }, [workflowId])

  const selectedNode = nodes.find(n => n.id === selectedId) || null

  const toWorld = (clientX, clientY) => {
    const rect = wrapperRef.current?.getBoundingClientRect() || { left: 0, top: 0 }
    const cam  = cameraRef.current
    return { x: (clientX - rect.left - cam.x) / cam.zoom, y: (clientY - rect.top - cam.y) / cam.zoom }
  }

  // Global mouse
  useEffect(() => {
    const onMove = (e) => {
      const d = dragRef.current
      if (!d) return
      if (d.type === 'canvas') {
        setCamera(c => ({ ...c, x: d.camX + (e.clientX - d.startX), y: d.camY + (e.clientY - d.startY) }))
      } else if (d.type === 'node') {
        const cam = cameraRef.current
        setNodes(prev => prev.map(n =>
          n.id === d.nodeId
            ? { ...n, x: d.nodeStartX + (e.clientX - d.startX) / cam.zoom, y: d.nodeStartY + (e.clientY - d.startY) / cam.zoom }
            : n
        ))
      } else if (d.type === 'edge') {
        const w = toWorld(e.clientX, e.clientY)
        setPendingEdge(pe => pe ? { ...pe, tx: w.x, ty: w.y } : null)
      }
    }
    const onUp = () => {
      if (dragRef.current?.type === 'edge') setPendingEdge(null)
      dragRef.current = null
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
    return () => { document.removeEventListener('mousemove', onMove); document.removeEventListener('mouseup', onUp) }
  }, []) // eslint-disable-line

  // Scroll zoom
  useEffect(() => {
    const el = wrapperRef.current
    if (!el) return
    const onWheel = (e) => {
      e.preventDefault()
      const factor = e.deltaY < 0 ? 1.1 : 0.9
      setCamera(c => {
        const newZoom = Math.max(0.25, Math.min(2.5, c.zoom * factor))
        const rect = el.getBoundingClientRect()
        const mx = e.clientX - rect.left, my = e.clientY - rect.top
        return { x: mx - (mx - c.x) * (newZoom / c.zoom), y: my - (my - c.y) * (newZoom / c.zoom), zoom: newZoom }
      })
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [])

  // Delete key
  useEffect(() => {
    const onKey = (e) => {
      if ((e.key === 'Delete' || e.key === 'Backspace') && !['INPUT', 'TEXTAREA', 'SELECT'].includes(e.target.tagName)) {
        if (selectedId) deleteNode(selectedId)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [selectedId]) // eslint-disable-line

  const handleCanvasMouseDown = (e) => {
    if (e.target !== wrapperRef.current && !e.target.dataset.canvasBg) return
    setSelectedId(null)
    dragRef.current = { type: 'canvas', startX: e.clientX, startY: e.clientY, camX: cameraRef.current.x, camY: cameraRef.current.y }
  }

  const startNodeDrag = (e, nodeId) => {
    const node = nodesRef.current.find(n => n.id === nodeId)
    if (!node) return
    dragRef.current = { type: 'node', nodeId, startX: e.clientX, startY: e.clientY, nodeStartX: node.x, nodeStartY: node.y }
  }

  const startEdge = (e, nodeId, portIdx) => {
    const node = nodesRef.current.find(n => n.id === nodeId)
    if (!node) return
    const pos = outPortPos(node, portIdx)
    dragRef.current = { type: 'edge', sourceNodeId: nodeId, sourcePortIdx: portIdx }
    setPendingEdge({ sx: pos.x, sy: pos.y, tx: pos.x, ty: pos.y })
  }

  const completeEdge = (e, targetNodeId, targetPortIdx) => {
    if (!dragRef.current || dragRef.current.type !== 'edge') return
    const { sourceNodeId, sourcePortIdx } = dragRef.current
    if (sourceNodeId === targetNodeId) { dragRef.current = null; setPendingEdge(null); return }
    const sNode = nodesRef.current.find(n => n.id === sourceNodeId)
    const tNode = nodesRef.current.find(n => n.id === targetNodeId)
    if (!sNode || !tNode) { dragRef.current = null; setPendingEdge(null); return }
    setEdges(prev => {
      const exists = prev.some(ed => ed.source === sourceNodeId && ed.sourcePortIdx === sourcePortIdx && ed.target === targetNodeId && ed.targetPortIdx === targetPortIdx)
      if (exists) return prev
      return [...prev, {
        id: uid(), source: sourceNodeId, sourcePortIdx,
        sourcePortId: sNode.outputs[sourcePortIdx]?.id,
        target: targetNodeId, targetPortIdx,
        targetPortId: tNode.inputs[targetPortIdx]?.id,
      }]
    })
    dragRef.current = null
    setPendingEdge(null)
  }

  const addNode = (type, template) => {
    const id   = uid()
    const rect = wrapperRef.current?.getBoundingClientRect() || { width: 800, height: 600 }
    const cam  = cameraRef.current
    const cx   = (rect.width / 2 - cam.x) / cam.zoom
    const cy   = (rect.height / 2 - cam.y) / cam.zoom
    const jitter = () => (Math.random() - 0.5) * 120
    const defaults = {}
    template.configFields?.forEach(f => { defaults[f.key] = f.default ?? '' })
    setNodes(prev => [...prev, {
      id, type,
      subtype: template.subtype,
      label: template.label,
      platform: template.platform || null,
      color: template.color || (template.platform ? PCOL[template.platform] : PCOL.default),
      inputs: template.inputs || [],
      outputs: template.outputs || [],
      configFields: template.configFields || [],
      config: defaults,
      x: cx - NODE_W / 2 + jitter(),
      y: cy - 60 + jitter(),
    }])
    setSelectedId(id)
  }

  const deleteNode = (id) => {
    setNodes(prev => prev.filter(n => n.id !== id))
    setEdges(prev => prev.filter(e => e.source !== id && e.target !== id))
    setSelectedId(s => s === id ? null : s)
  }

  const updateConfig = (nodeId, key, val) =>
    setNodes(prev => prev.map(n => n.id === nodeId ? { ...n, config: { ...n.config, [key]: val } } : n))

  const updateLabel = (nodeId, label) =>
    setNodes(prev => prev.map(n => n.id === nodeId ? { ...n, label } : n))

  const handleSave = async () => {
    setSaving(true)
    try {
      const req = canvasToBackend(workflowId, workflowName, nodes, edges)
      await _SaveWorkflow(req)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch (e) {
      alert('Save failed: ' + (e?.message || e))
    } finally {
      setSaving(false)
    }
  }

  const handleRun = async () => {
    if (!workflowId) { alert('Save the workflow first.'); return }
    setRunStatus('running')
    setRunExecId(null)
    try {
      const result = await _RunWorkflow(workflowId)
      setRunExecId(result?.execution_id || null)
      setRunStatus('success')
    } catch (e) {
      setRunStatus('error')
    }
  }

  const handleToggleActive = async () => {
    if (!workflowId) return
    try {
      await _SetWorkflowActive(workflowId, !workflowActive)
      setWorkflowActive(v => !v)
    } catch (e) {
      alert('Toggle failed: ' + (e?.message || e))
    }
  }

  const handleRenameCommit = () => {
    if (nameInput.trim()) setWorkflowName(nameInput.trim())
    setEditingName(false)
  }

  // Edge paths
  const edgePaths = edges.map(edge => {
    const sNode = nodes.find(n => n.id === edge.source)
    const tNode = nodes.find(n => n.id === edge.target)
    if (!sNode || !tNode) return null
    const sp = outPortPos(sNode, edge.sourcePortIdx)
    const tp = inPortPos(tNode, edge.targetPortIdx)
    return { ...edge, path: edgePath(sp.x, sp.y, tp.x, tp.y), color: sNode.color || PCOL.default }
  }).filter(Boolean)

  const pendingPath = pendingEdge ? edgePath(pendingEdge.sx, pendingEdge.sy, pendingEdge.tx, pendingEdge.ty) : null

  if (loadingWf) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', background: '#04060a', gap: 12, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
        <Loader size={16} style={{ animation: 'spin 1s linear infinite' }} /> Loading workflow...
        <style>{`@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }`}</style>
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', background: '#04060a', overflow: 'hidden' }}>

      {/* Toolbar */}
      <div style={{
        height: 46, flexShrink: 0, display: 'flex', alignItems: 'center',
        gap: 6, padding: '0 12px',
        background: '#080d16', borderBottom: '1px solid rgba(0,180,216,0.1)', zIndex: 10,
      }}>
        {/* Back */}
        <button
          style={{ ...toolbarBtnStyle, gap: 5 }}
          onClick={onBack}
          title="Back to workflows"
        >
          <ArrowLeft size={13} />
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10 }}>BACK</span>
        </button>

        <div style={{ width: 1, height: 20, background: 'rgba(0,180,216,0.1)', margin: '0 2px' }} />

        {/* Palette toggle */}
        <button
          style={{ ...toolbarBtnStyle, background: paletteOpen ? 'rgba(0,180,216,0.1)' : 'transparent', color: paletteOpen ? '#00b4d8' : 'var(--text-muted)', gap: 5 }}
          onClick={() => setPaletteOpen(p => !p)}
          title={paletteOpen ? 'Hide palette' : 'Show palette'}
        >
          {paletteOpen ? <ChevronLeft size={12} /> : <ChevronRight size={12} />}
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10 }}>PALETTE</span>
        </button>

        {/* Workflow name — editable */}
        <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          {editingName ? (
            <input
              autoFocus
              style={{
                background: 'rgba(0,180,216,0.08)', border: '1px solid rgba(0,180,216,0.3)',
                borderRadius: 5, padding: '3px 10px', color: '#dde6f0',
                fontFamily: 'var(--font-mono)', fontSize: 12, fontWeight: 700,
                letterSpacing: 1, outline: 'none', textAlign: 'center', minWidth: 180,
              }}
              value={nameInput}
              onChange={e => setNameInput(e.target.value)}
              onBlur={handleRenameCommit}
              onKeyDown={e => { if (e.key === 'Enter') handleRenameCommit(); if (e.key === 'Escape') setEditingName(false) }}
            />
          ) : (
            <button
              style={{
                background: 'none', border: '1px solid transparent', borderRadius: 5,
                padding: '3px 10px', color: 'var(--text-secondary)',
                fontFamily: 'var(--font-mono)', fontSize: 12, fontWeight: 700,
                letterSpacing: 1, cursor: 'text', textTransform: 'uppercase',
                transition: 'all 140ms',
              }}
              onMouseEnter={e => { e.currentTarget.style.borderColor = 'rgba(0,180,216,0.2)'; e.currentTarget.style.color = '#dde6f0' }}
              onMouseLeave={e => { e.currentTarget.style.borderColor = 'transparent'; e.currentTarget.style.color = 'var(--text-secondary)' }}
              onClick={() => { setNameInput(workflowName); setEditingName(true) }}
              title="Click to rename"
            >
              {workflowName}
            </button>
          )}
        </div>

        {/* Active toggle */}
        <button
          style={{
            display: 'flex', alignItems: 'center', gap: 5,
            padding: '4px 10px', borderRadius: 20,
            background: workflowActive ? 'rgba(16,185,129,0.1)' : 'rgba(100,120,140,0.08)',
            border: `1px solid ${workflowActive ? 'rgba(16,185,129,0.3)' : 'rgba(100,120,140,0.2)'}`,
            color: workflowActive ? '#10b981' : 'var(--text-muted)',
            fontFamily: 'var(--font-mono)', fontSize: 9, cursor: 'pointer',
            letterSpacing: 1, textTransform: 'uppercase', transition: 'all 140ms',
          }}
          onClick={handleToggleActive}
          title={workflowActive ? 'Click to deactivate' : 'Click to activate'}
        >
          {workflowActive ? <ToggleRight size={13} /> : <ToggleLeft size={13} />}
          {workflowActive ? 'Active' : 'Draft'}
        </button>

        {/* Stats */}
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)' }}>
          {nodes.length}n · {edges.length}e
        </span>

        {/* Zoom */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 3 }}>
          <button style={toolbarBtnStyle} onClick={() => setCamera(c => ({ ...c, zoom: Math.max(0.25, c.zoom / 1.2) }))} title="Zoom out"><ZoomOut size={13} /></button>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', minWidth: 34, textAlign: 'center' }}>{Math.round(camera.zoom * 100)}%</span>
          <button style={toolbarBtnStyle} onClick={() => setCamera(c => ({ ...c, zoom: Math.min(2.5, c.zoom * 1.2) }))} title="Zoom in"><ZoomIn size={13} /></button>
          <button style={toolbarBtnStyle} onClick={() => setCamera({ x: 80, y: 80, zoom: 1 })} title="Reset view"><RotateCcw size={13} /></button>
        </div>

        {/* Run */}
        <button
          style={{
            ...toolbarBtnStyle,
            background: runStatus === 'running' ? 'rgba(0,180,216,0.12)' : 'rgba(16,185,129,0.08)',
            border: `1px solid ${runStatus === 'running' ? 'rgba(0,180,216,0.3)' : 'rgba(16,185,129,0.25)'}`,
            color: runStatus === 'running' ? '#00b4d8' : '#10b981',
            padding: '4px 12px', gap: 5,
          }}
          onClick={handleRun}
          disabled={runStatus === 'running'}
          title="Run workflow"
        >
          {runStatus === 'running'
            ? <><Loader size={12} style={{ animation: 'spin 1s linear infinite' }} /><span style={{ fontFamily: 'var(--font-mono)', fontSize: 10 }}>RUNNING</span></>
            : <><Play size={12} /><span style={{ fontFamily: 'var(--font-mono)', fontSize: 10 }}>RUN</span></>}
        </button>

        {/* Save */}
        <button
          style={{
            ...toolbarBtnStyle,
            background: saved ? 'rgba(16,185,129,0.15)' : 'rgba(0,180,216,0.08)',
            border: `1px solid ${saved ? 'rgba(16,185,129,0.4)' : 'rgba(0,180,216,0.2)'}`,
            color: saved ? '#10b981' : '#00b4d8',
            padding: '4px 12px', gap: 5,
          }}
          onClick={handleSave}
          disabled={saving}
        >
          {saving
            ? <><Loader size={12} style={{ animation: 'spin 1s linear infinite' }} /><span style={{ fontFamily: 'var(--font-mono)', fontSize: 10 }}>SAVING</span></>
            : saved
              ? <><Check size={12} /><span style={{ fontFamily: 'var(--font-mono)', fontSize: 10 }}>SAVED</span></>
              : <><Save size={12} /><span style={{ fontFamily: 'var(--font-mono)', fontSize: 10 }}>SAVE</span></>}
        </button>
      </div>

      {/* Execution status banner */}
      <ExecutionStatusBanner
        status={runStatus}
        executionId={runExecId}
        onDismiss={() => { setRunStatus(null); setRunExecId(null) }}
      />

      {/* Main area */}
      <div style={{ display: 'flex', flex: 1, overflow: 'hidden', position: 'relative' }}>
        {paletteOpen && <NodePalette onAdd={addNode} />}

        {/* Canvas */}
        <div
          ref={wrapperRef}
          style={{ flex: 1, position: 'relative', overflow: 'hidden', cursor: dragRef.current?.type === 'canvas' ? 'grabbing' : 'default' }}
          onMouseDown={handleCanvasMouseDown}
        >
          {/* Dot grid */}
          <div
            data-canvas-bg="1"
            style={{
              position: 'absolute', inset: 0,
              backgroundImage: 'radial-gradient(circle, rgba(0,180,216,0.2) 1.2px, transparent 1.2px)',
              backgroundSize: '28px 28px',
              backgroundPosition: `${camera.x % 28}px ${camera.y % 28}px`,
              pointerEvents: 'none',
            }}
          />

          {/* SVG edges */}
          <svg style={{ position: 'absolute', inset: 0, width: '100%', height: '100%', overflow: 'visible', zIndex: 1, pointerEvents: 'none' }}>
            <defs>
              <filter id="wf-glow">
                <feGaussianBlur stdDeviation="3" result="blur" />
                <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
              </filter>
            </defs>
            <g transform={`translate(${camera.x} ${camera.y}) scale(${camera.zoom})`}>
              {edgePaths.map(ep => (
                <g key={ep.id}>
                  <path d={ep.path} fill="none" stroke={ep.color} strokeOpacity={0.12} strokeWidth={10} vectorEffect="non-scaling-stroke" />
                  <path d={ep.path} fill="none" stroke={ep.color} strokeOpacity={0.7} strokeWidth={1.8} vectorEffect="non-scaling-stroke" style={{ filter: 'url(#wf-glow)' }} />
                  <path d={ep.path} fill="none" stroke={ep.color} strokeOpacity={0.9} strokeWidth={1.2} strokeDasharray="7 12" vectorEffect="non-scaling-stroke" style={{ animation: 'wfFlow 1s linear infinite' }} />
                  <path
                    d={ep.path} fill="none" stroke="transparent" strokeWidth={14}
                    vectorEffect="non-scaling-stroke"
                    style={{ cursor: 'pointer', pointerEvents: 'stroke' }}
                    onClick={e => { e.stopPropagation(); setEdges(prev => prev.filter(e2 => e2.id !== ep.id)) }}
                    title="Click to delete connection"
                  />
                </g>
              ))}
              {pendingPath && (
                <path d={pendingPath} fill="none" stroke="rgba(0,180,216,0.45)" strokeWidth={2} strokeDasharray="5 8" vectorEffect="non-scaling-stroke" />
              )}
            </g>
          </svg>

          {/* Nodes */}
          <div style={{
            position: 'absolute', inset: 0, overflow: 'visible',
            transform: `translate(${camera.x}px, ${camera.y}px) scale(${camera.zoom})`,
            transformOrigin: '0 0', zIndex: 2,
          }}>
            {nodes.map(node => (
              <WorkflowNode
                key={node.id}
                node={node}
                selected={node.id === selectedId}
                zoom={camera.zoom}
                onClick={() => setSelectedId(node.id)}
                onHeaderMouseDown={e => { setSelectedId(node.id); startNodeDrag(e, node.id) }}
                onOutputPortMouseDown={(e, portIdx) => startEdge(e, node.id, portIdx)}
                onInputPortMouseUp={(e, portIdx) => completeEdge(e, node.id, portIdx)}
                onDelete={() => deleteNode(node.id)}
              />
            ))}
          </div>

          {/* Empty state */}
          {nodes.length === 0 && (
            <div style={{ position: 'absolute', inset: 0, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', pointerEvents: 'none', gap: 14 }}>
              <div style={{ fontSize: 56, opacity: 0.06, fontFamily: 'var(--font-mono)', color: '#00b4d8' }}>⬡⬡⬡</div>
              <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)', letterSpacing: 3, textTransform: 'uppercase' }}>
                Select nodes from the palette to build a workflow
              </div>
              <div style={{ fontSize: 10, color: 'var(--text-dim)', fontFamily: 'var(--font-mono)', letterSpacing: 1 }}>
                Drag · Connect · Configure
              </div>
            </div>
          )}
        </div>

        {/* Inspector */}
        {selectedNode && (
          <NodeInspector
            node={selectedNode}
            onUpdateLabel={updateLabel}
            onUpdateConfig={updateConfig}
            onDelete={() => deleteNode(selectedNode.id)}
            onClose={() => setSelectedId(null)}
          />
        )}
      </div>

      <style>{`
        @keyframes wfFlow { from { stroke-dashoffset: 0; } to { stroke-dashoffset: -38; } }
        @keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }
      `}</style>
    </div>
  )
}

const toolbarBtnStyle = {
  background: 'transparent',
  border: '1px solid rgba(0,180,216,0.15)',
  borderRadius: 5, padding: '4px 7px',
  color: 'var(--text-muted)', cursor: 'pointer',
  display: 'flex', alignItems: 'center', gap: 4,
  transition: 'all 120ms',
}

// ════════════════════════════════════════════════════════════════════
// Root: Workflow — manager or editor
// ════════════════════════════════════════════════════════════════════
export default function Workflow() {
  const [selectedWorkflowId, setSelectedWorkflowId] = useState(null)

  const handleNew = async () => {
    try {
      const wf = await _SaveWorkflow({
        id: '', name: 'Untitled Workflow', description: '',
        nodes: [], connections: [],
      })
      setSelectedWorkflowId(wf?.id || null)
    } catch (e) {
      alert('Failed to create workflow: ' + (e?.message || e))
    }
  }

  if (selectedWorkflowId !== null) {
    return (
      <WorkflowEditor
        workflowId={selectedWorkflowId}
        onBack={() => setSelectedWorkflowId(null)}
      />
    )
  }

  return (
    <WorkflowManager
      onSelect={id => setSelectedWorkflowId(id)}
      onNew={handleNew}
    />
  )
}
