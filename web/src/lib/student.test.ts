import { clearPrivateDrafts, saveSelectedChild, scopedKey, selectedChild, studentScope } from './student'
import { userFixture } from '../test/fixtures'

describe('student query isolation', () => {
  it('includes selected child and all relevant context in the key', () => {
    expect(scopedKey('grades', 'child-2', 'period-1')).toEqual(['grades', 'child-2', 'period-1'])
    expect(scopedKey('grades', 'child-1', 'period-1')).not.toEqual(scopedKey('grades', 'child-2', 'period-1'))
  })

  it('uses child only for parent and own student id otherwise', () => {
    expect(studentScope({ ...userFixture, roles: ['parent'] }, 'child-1')).toBe('child-1')
    expect(studentScope(userFixture, 'other-child')).toBe(userFixture.student_id)
  })

  it('clears selected child and private drafts on logout', () => {
    saveSelectedChild('child-1')
    sessionStorage.setItem('diary_draft_homework', 'private')
    sessionStorage.setItem('unrelated', 'keep')
    expect(selectedChild()).toBe('child-1')
    clearPrivateDrafts()
    expect(selectedChild()).toBe('')
    expect(sessionStorage.getItem('diary_draft_homework')).toBeNull()
    expect(sessionStorage.getItem('unrelated')).toBe('keep')
  })
})
