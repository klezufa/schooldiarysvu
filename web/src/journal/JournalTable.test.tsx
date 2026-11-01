import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { JournalTable } from './JournalTable'
import { entryFixture } from '../test/fixtures'

vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count, estimateSize }: { count: number; estimateSize: () => number }) => ({
    getTotalSize: () => count * estimateSize(),
    scrollToIndex: vi.fn(),
    getVirtualItems: () => Array.from({ length: count }, (_, index) => ({ index, start: index * estimateSize(), size: estimateSize(), key: index })),
  }),
}))

describe('journal table', () => {
  it('renders long names and opens a grade cell with keyboard', async () => {
    const user = userEvent.setup()
    const entry = { ...entryFixture, student_name: 'Очень Длинное Имя Ученика Которое Не Должно Ломать Таблицу' }
    render(<JournalTable entries={[entry]} students={[{ id: entry.student_id, display_name: entry.student_name }]} allowedValues={[2, 3, 4, 5]} onSave={vi.fn()} isSaving={false} conflict={null} onCloseConflict={vi.fn()} month="2026-09" mode="all" selectedLessonId="" selectedStudentId="" compact={false} />)
    const cell = await screen.findByRole('button', { name: /Очень Длинное Имя/ })
    cell.focus()
    await user.keyboard('{Enter}')
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Оценка ученика')).toBeInTheDocument()
  })

  const second = { ...entryFixture, student_id: '20000000-0000-4000-8000-000000000002', student_name: 'Ученик Второй', grade_id: '50000000-0000-4000-8000-000000000002', grade_value: null, grade_version: null }
  const props = { students: [{ id: entryFixture.student_id, display_name: entryFixture.student_name }, { id: second.student_id, display_name: second.student_name }], allowedValues: [2, 3, 4, 5], isSaving: false, conflict: null, onCloseConflict: vi.fn(), month: '2026-09', mode: 'all' as const, selectedLessonId: '', selectedStudentId: '', compact: false }

  it('moves between cells by grid position with arrow keys', async () => {
    const user = userEvent.setup()
    render(<JournalTable {...props} entries={[entryFixture, second]} onSave={vi.fn()} />)
    const first = screen.getByRole('button', { name: /Ученик Тестовый/ })
    first.focus()
    await user.keyboard('{ArrowDown}')
    await waitFor(() => expect(screen.getByRole('button', { name: /Ученик Второй/ })).toHaveFocus())
    await user.keyboard('{ArrowUp}')
    await waitFor(() => expect(first).toHaveFocus())
  })

  it('opens the editor with a typed digit, keeps the server value, and saves with Enter then moves to the next student', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn((_input, done?: () => void) => done?.())
    render(<JournalTable {...props} entries={[entryFixture, second]} onSave={onSave} />)
    screen.getByRole('button', { name: /Ученик Тестовый/ }).focus()
    await user.keyboard('5')
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '5' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByText('1', { selector: 'dd' })).toBeInTheDocument()
    await user.keyboard('{Enter}')
    expect(onSave).toHaveBeenCalledWith({ entry: entryFixture, value: 5, comment: '' }, expect.any(Function))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    await waitFor(() => expect(screen.getByRole('button', { name: /Ученик Второй/ })).toHaveFocus())
  })
})
