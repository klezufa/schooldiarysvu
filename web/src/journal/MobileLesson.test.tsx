import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { MobileLesson } from './MobileLesson'
import { ToastProvider } from '../components/Toast'
import { entryFixture } from '../test/fixtures'
import type { Lesson } from '../api/types'

const lesson = { id: entryFixture.lesson_id, lesson_date: '2026-09-01', starts_at: '2026-09-01T09:00:00Z', subject_name: 'Математика', class_name: '5А', topic: 'Тема' } as Lesson

function renderLesson(conflict: React.ComponentProps<typeof MobileLesson>['conflict'], onSave: React.ComponentProps<typeof MobileLesson>['onSave'] = vi.fn()) {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('[]', { status: 200, headers: { 'Content-Type': 'application/json' } })))
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><ToastProvider><MemoryRouter>
    <MobileLesson lessons={[lesson]} students={[{ id: entryFixture.student_id, display_name: entryFixture.student_name }, { id: '20000000-0000-4000-8000-000000000002', display_name: 'Ученик Новый' }]} entries={[entryFixture]} selectedLessonId={lesson.id} onSelectLesson={vi.fn()} allowedValues={[2, 3, 4, 5]} onSave={onSave} isSaving={false} conflict={conflict} onCloseConflict={vi.fn()} />
  </MemoryRouter></ToastProvider></QueryClientProvider>)
}

describe('mobile lesson mode', () => {
  it('opens the editor with the unsaved value when a quick-button save hits a version conflict', async () => {
    renderLesson({ entry: entryFixture, value: 2, comment: '', message: 'Версия изменена' })
    expect(await screen.findByText('Серверная версия изменилась')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '2', pressed: true })).toBeInTheDocument()
  })

  it('does not inherit another student\'s grade for a student without a journal row', () => {
    renderLesson(null)
    const group = screen.getByLabelText('Оценка для Ученик Новый')
    expect(group.querySelectorAll('[aria-pressed="true"]')).toHaveLength(0)
  })

  it('saves a grade and moves the editor to the next student, without a next button for the last one', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn((_input: unknown, onSaved?: () => void) => onSaved?.())
    renderLesson(null, onSave)
    await user.click(screen.getByRole('button', { name: new RegExp(entryFixture.student_name) }))
    let dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: '4' }))
    await user.click(within(dialog).getByRole('button', { name: 'Сохранить и к следующему: Ученик Новый' }))
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ value: 4, entry: expect.objectContaining({ student_id: entryFixture.student_id }) }), expect.any(Function))
    dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/Ученик Новый/)).toBeInTheDocument()
    expect(within(dialog).queryByRole('button', { name: /Сохранить и к следующему/ })).not.toBeInTheDocument()
  })
})
