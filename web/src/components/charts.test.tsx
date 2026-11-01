import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ChartCard, ColumnChart, LineChart, Meter, SegmentedBar } from './charts'

describe('charts', () => {
  it('column chart exposes every value to assistive technology and shows a tooltip on focus', async () => {
    const user = userEvent.setup()
    render(<ColumnChart ariaLabel="Средний балл" max={5} data={[
      { key: 'm', label: 'Математика', value: 4.25, hint: '12 оценок' },
      { key: 'r', label: 'Русский язык', value: 3.5 },
    ]} />)
    expect(screen.getByRole('group', { name: 'Средний балл' })).toBeInTheDocument()
    await user.tab()
    expect(screen.getByRole('button', { name: 'Математика: 4,25, 12 оценок' })).toHaveFocus()
    expect(screen.getByRole('tooltip')).toHaveTextContent('4,25')
    await user.tab()
    expect(screen.getByRole('tooltip')).toHaveTextContent('Русский язык')
  })

  it('column chart shows an explicit empty state instead of a blank plot', () => {
    render(<ColumnChart ariaLabel="Пусто" data={[]} emptyText="Оценок пока нет" />)
    expect(screen.getByText('Оценок пока нет')).toBeInTheDocument()
  })

  it('segmented bar labels parts with value and share and lists all of them in the legend', () => {
    render(<SegmentedBar ariaLabel="Распределение" parts={[
      { key: '5', label: '«5»', value: 30, color: 'green' },
      { key: '4', label: '«4»', value: 10, color: 'blue' },
      { key: '2', label: '«2»', value: 0, color: 'red' },
    ]} />)
    expect(screen.getByRole('button', { name: /«5»: 30 \(75\s?%\)/ })).toBeInTheDocument()
    expect(screen.getAllByRole('button')).toHaveLength(2) // нулевая часть не рисуется
    expect(screen.getByText('«2»')).toBeInTheDocument() // но остаётся в легенде
  })

  it('meter reports its value as a percentage', () => {
    render(<Meter label="Посещаемость" value={0.92} />)
    const meter = screen.getByRole('meter', { name: 'Посещаемость' })
    expect(meter).toHaveAttribute('aria-valuenow', '92')
  })

  it('line chart moves its tooltip with the arrow keys', () => {
    render(<LineChart ariaLabel="Динамика" points={[
      { key: 'a', label: '1 сент.', value: 4 },
      { key: 'b', label: '8 сент.', value: 4.5 },
      { key: 'c', label: '15 сент.', value: 3.8 },
    ]} />)
    const plot = screen.getByRole('img', { name: /Динамика\. Последнее значение 3,8/ })
    fireEvent.focus(plot)
    expect(screen.getByRole('tooltip')).toHaveTextContent('15 сент.')
    fireEvent.keyDown(plot, { key: 'ArrowLeft' })
    expect(screen.getByRole('tooltip')).toHaveTextContent('8 сент.')
  })

  it('chart card switches to an accessible table view', async () => {
    const user = userEvent.setup()
    render(<ChartCard title="Классы" table={{ columns: ['Класс', 'Балл'], rows: [['5А', '4,2']] }}><p>график</p></ChartCard>)
    await user.click(screen.getByRole('button', { name: 'Показать таблицей' }))
    expect(screen.getByRole('table', { name: 'Классы' })).toHaveTextContent('5А')
    expect(screen.queryByText('график')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Показать графиком' })).toHaveAttribute('aria-pressed', 'true')
  })
})
