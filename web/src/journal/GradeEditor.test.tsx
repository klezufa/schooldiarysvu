import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { GradeEditor } from './GradeEditor'
import { entryFixture } from '../test/fixtures'

describe('grade editor', () => {
  it('submits an allowed value and comment', async () => {
    const user = userEvent.setup()
    const save = vi.fn()
    render(<GradeEditor entry={entryFixture} open onOpenChange={vi.fn()} allowedValues={[2, 3, 4, 5]} onSave={save} isSaving={false} conflict={null} onCloseConflict={vi.fn()} />)
    await user.click(screen.getByRole('button', { name: '5' }))
    await user.type(screen.getByLabelText('Комментарий'), 'Отличная работа')
    await user.click(screen.getByRole('button', { name: 'Сохранить' }))
    expect(save).toHaveBeenCalledWith({ entry: entryFixture, value: 5, comment: 'Отличная работа' }, expect.any(Function))
  })

  it('keeps local and server values visible during a conflict', () => {
    render(<GradeEditor entry={entryFixture} open onOpenChange={vi.fn()} allowedValues={[2, 3, 4, 5]} onSave={vi.fn()} isSaving={false} conflict={{ entry: entryFixture, value: 5, comment: 'Локально', message: 'Версия изменена' }} onCloseConflict={vi.fn()} />)
    expect(screen.getByText('Серверная версия изменилась')).toBeInTheDocument()
    expect(screen.getByText(/5; Локально/)).toBeInTheDocument()
    expect(screen.getByText(/4; версия 1/)).toBeInTheDocument()
  })

  it('selects a grade with a digit and saves with Enter without touching comment input', async () => {
    const user = userEvent.setup()
    const save = vi.fn()
    render(<GradeEditor entry={entryFixture} open onOpenChange={vi.fn()} allowedValues={[2, 3, 4, 5]} onSave={save} isSaving={false} conflict={null} onCloseConflict={vi.fn()} />)
    await user.click(screen.getByRole('button', { name: '4' }))
    await user.keyboard('5{Enter}')
    expect(save).toHaveBeenCalledWith({ entry: entryFixture, value: 5, comment: '' }, expect.any(Function))
    save.mockClear()
    await user.type(screen.getByLabelText('Комментарий'), '3{Enter}')
    expect(save).not.toHaveBeenCalled()
    expect(screen.getByLabelText('Комментарий')).toHaveValue('3\n')
  })

  it('shows the server value, not a locally typed digit, as the current server value', () => {
    render(<GradeEditor entry={entryFixture} initialValue={2} open onOpenChange={vi.fn()} allowedValues={[2, 3, 4, 5]} onSave={vi.fn()} isSaving={false} conflict={{ entry: entryFixture, value: 2, comment: '', message: 'Версия изменена' }} onCloseConflict={vi.fn()} />)
    expect(screen.getByRole('button', { name: '2' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByText(/4; версия 1/)).toBeInTheDocument()
  })
})
