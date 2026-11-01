import { DRAFT_TTL_MS, draftsEnabled, loadDraft, removeDraft, saveDraft, setSharedComputer } from './drafts'
import { clearPrivateDrafts } from './student'

describe('local drafts', () => {
  beforeEach(() => { sessionStorage.clear(); localStorage.clear() })

  it('restores a saved draft and drops it after removal', () => {
    saveDraft('homework_new', 'Решить задачи 1–3')
    expect(loadDraft('homework_new')).toBe('Решить задачи 1–3')
    removeDraft('homework_new')
    expect(loadDraft('homework_new')).toBe('')
  })

  it('expires drafts after the TTL and removes the stored value', () => {
    const now = 1_000_000
    saveDraft('homework_new', 'текст', now)
    expect(loadDraft('homework_new', now + DRAFT_TTL_MS - 1)).toBe('текст')
    expect(loadDraft('homework_new', now + DRAFT_TTL_MS)).toBe('')
    expect(sessionStorage.getItem('diary_draft_homework_new')).toBeNull()
  })

  it('does not store empty input and ignores corrupted entries', () => {
    saveDraft('homework_new', '   ')
    expect(sessionStorage.getItem('diary_draft_homework_new')).toBeNull()
    sessionStorage.setItem('diary_draft_broken', '{not json')
    expect(loadDraft('broken')).toBe('')
  })

  it('stores nothing in shared-computer mode and wipes existing drafts when it is enabled', () => {
    saveDraft('homework_new', 'текст')
    setSharedComputer(true)
    expect(draftsEnabled()).toBe(false)
    expect(loadDraft('homework_new')).toBe('')
    saveDraft('homework_new', 'новый')
    expect(sessionStorage.getItem('diary_draft_homework_new')).toBeNull()
    setSharedComputer(false)
    expect(draftsEnabled()).toBe(true)
  })

  it('is wiped together with other private data on logout', () => {
    saveDraft('homework_new', 'текст')
    clearPrivateDrafts()
    expect(loadDraft('homework_new')).toBe('')
  })
})
