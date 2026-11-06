import type { SubstitutionCandidate, SubstitutionLesson } from '../../api/types'

export const candidateReasonText: Record<SubstitutionCandidate['reasons'][number], string> = {
  teaches_subject: 'ведёт этот предмет',
  knows_class: 'работает с этим классом',
  at_school: 'в этот день в школе',
}

/** Первый и последний день месяца «YYYY-MM» для запроса учёта замен. */
export function monthRange(month: string): { from: string; to: string } {
  const [year, monthNumber] = month.split('-').map(Number)
  const last = new Date(Date.UTC(year!, monthNumber!, 0)).getUTCDate()
  return { from: `${month}-01`, to: `${month}-${String(last).padStart(2, '0')}` }
}

/** Сколько уроков провёл каждый заменяющий; больше — выше, при равенстве — по алфавиту. */
export function substituteTotals(lessons: SubstitutionLesson[]): { name: string; lessons: number }[] {
  const counts = new Map<string, number>()
  for (const lesson of lessons) counts.set(lesson.teacher_name, (counts.get(lesson.teacher_name) ?? 0) + 1)
  return [...counts].map(([name, count]) => ({ name, lessons: count }))
    .sort((a, b) => b.lessons - a.lessons || a.name.localeCompare(b.name, 'ru'))
}
