// Thin wrapper around Wails Go bindings with error handling.
import * as GoApp from '../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'

export const api = {
  getDashboardStats:    () => GoApp.GetDashboardStats().catch(nullOnError),
  getActions:       (platform = '', state = '', limit = 0) => GoApp.GetActions(platform, state, limit).catch(nullOnError),
  getAction:        (id) => GoApp.GetAction(id).catch(nullOnError),
  createAction:     (req) => GoApp.CreateAction(req),
  updateActionState:(id, state) => GoApp.UpdateActionState(id, state),
  deleteAction:     (id) => GoApp.DeleteAction(id),
  updateActionParams:(id, params) => GoApp.UpdateActionParams(id, params),
  executeAction:    (id) => GoApp.ExecuteAction(id),
  getTargets:       (actionId) => GoApp.GetActionTargets(actionId).catch(nullOnError),
  addTarget:        (actionId, link, platform) => GoApp.AddActionTarget(actionId, link, platform),
  getPeople:        (platform = '', search = '', limit = 50, offset = 0) => GoApp.GetPeople(platform, search, limit, offset).catch(nullOnError),
  getPeopleCount:   (platform = '', search = '') => GoApp.GetPeopleCount(platform, search).catch(() => 0),
  getSessions:      () => GoApp.GetSessions().catch(nullOnError),
  deleteSession:    (id) => GoApp.DeleteSession(id),
  getSocialLists:   () => GoApp.GetSocialLists().catch(nullOnError),
  getTemplates:     () => GoApp.GetTemplates().catch(nullOnError),
  getLogs:          () => GoApp.GetLogs().catch(() => []),
  clearLogs:        () => GoApp.ClearLogs(),
  getAvailableActionTypes: () => GoApp.GetAvailableActionTypes().catch(() => ({})),
  getDBPath:        () => GoApp.GetDBPath().catch(() => ''),
  isDBConnected:    () => GoApp.IsDBConnected().catch(() => false),
  openURL:          (url) => GoApp.OpenURL(url).catch(console.warn),
  getPersonDetail:      (id) => GoApp.GetPersonDetail(id).catch(nullOnError),
  getPersonInteractions:(id) => GoApp.GetPersonInteractions(id).catch(() => []),
  getAllTags:            ()  => GoApp.GetAllTags().catch(() => []),
  getPersonTags:        (personId) => GoApp.GetPersonTags(personId).catch(() => []),
  addPersonTag:         (personId, name, color) => GoApp.AddPersonTag(personId, name, color).catch(nullOnError),
  removePersonTag:      (personId, tagId) => GoApp.RemovePersonTag(personId, tagId).catch(console.warn),
  getPeopleTagsMap:     (ids) => GoApp.GetPeopleTagsMap(ids).catch(() => ({})),
}

function nullOnError(e) {
  console.warn('API error:', e)
  return null
}

export function onLogEntry(callback) {
  EventsOn('log:entry', callback)
  return () => EventsOff('log:entry')
}

export function onActionComplete(callback) {
  EventsOn('action:complete', callback)
  return () => EventsOff('action:complete')
}

export const PLATFORMS = ['INSTAGRAM', 'LINKEDIN', 'X', 'TIKTOK']
export const STATES = ['PENDING', 'RUNNING', 'PAUSED', 'COMPLETED', 'FAILED', 'CANCELLED']

export const PLATFORM_COLORS = {
  INSTAGRAM: '#e1306c',
  LINKEDIN:  '#0077b5',
  X:         '#e7e9ea',
  TIKTOK:    '#ff0050',
  EMAIL:     '#6366f1',
  TELEGRAM:  '#26a5e4',
}

export const STATE_COLORS = {
  PENDING:   '#94a3b8',
  RUNNING:   '#00f5d4',
  PAUSED:    '#eab308',
  COMPLETED: '#10b981',
  FAILED:    '#ef4444',
  CANCELLED: '#6b7280',
}
