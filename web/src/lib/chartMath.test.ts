import { compact, formatBytes, formatPercent, gradeTone, niceTicks } from './chartMath'

describe('chart math', () => {
  it.each([
    [5, 5, [0, 1, 2, 3, 4, 5]],
    [1, 4, [0, 0.25, 0.5, 0.75, 1]],
    [172, 4, [0, 50, 100, 150, 200]],
    [0, 4, [0, 1]],
  ])('builds round ticks for max %s', (max, count, expected) => {
    expect(niceTicks(max, count)).toEqual(expected)
  })

  it('never ends the axis below the maximum', () => {
    for (const max of [3.7, 9.99, 13, 1234, 0.04]) {
      const ticks = niceTicks(max)
      expect(ticks[ticks.length - 1]).toBeGreaterThanOrEqual(max)
    }
  })

  it.each([
    [5, 'good'], [4.6, 'good'], [4, 'info'], [3.6, 'info'], [3, 'warning'], [2.4, 'critical'], [2, 'critical'],
  ] as const)('grade %s on a 5-point scale is %s', (value, tone) => {
    expect(gradeTone(value)).toBe(tone)
  })

  it('formats sizes, shares and compact numbers in Russian', () => {
    expect(formatBytes(0)).toBe('0 Б')
    expect(formatBytes(1536)).toBe('1,5 КБ')
    expect(formatBytes(5 * 1024 ** 3)).toBe('5 ГБ')
    expect(formatBytes(null)).toBe('—')
    expect(formatPercent(0.926)).toMatch(/^93\s?%$/)
    expect(formatPercent(undefined)).toBe('—')
    expect(compact(9_999)).toMatch(/^9\s?999$/)
    expect(compact(25_000)).toMatch(/25/)
  })
})
