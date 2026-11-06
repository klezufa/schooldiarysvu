import type { AcademicPeriod } from '../../../api/types'
import { addDays } from '../../../lib/dates'

/** Разделы упрощённого режима; пустая строка — главная со списком задач. */
export type SimpleView = '' | 'students' | 'parents' | 'teachers' | 'classes' | 'class' | 'schedule' | 'substitutions' | 'archive' | 'passwords' | 'subjects'

export type SetupInput = {
  hasCurrentPeriod: boolean
  classes: number
  subjects: number
  teachers: number
  students: number
  /** Классы текущего года без единого урока в шаблоне текущего периода. */
  classesWithoutSchedule: { id: string; name: string }[]
}

export type SetupStep = {
  id: 'period' | 'classes' | 'subjects' | 'teachers' | 'students' | 'schedule'
  title: string
  text: string
  done: boolean
  view: SimpleView
  /** Класс, который открыть сразу (для шага расписания). */
  classId?: string
  action: string
}

/**
 * Шаги подготовки школы в том порядке, в котором они зависят друг от друга: без четверти нет расписания, без классов
 * некуда зачислять, без предметов и учителей нечего ставить в сетку.
 */
export function setupSteps(input: SetupInput): SetupStep[] {
  const firstWithout = input.classesWithoutSchedule[0]
  const without = input.classesWithoutSchedule.length
  return [
    {
      id: 'period', title: 'Учебный год и четверти', done: input.hasCurrentPeriod, view: '', action: 'Открыть настройки',
      text: input.hasCurrentPeriod ? 'Текущая четверть настроена.' : 'Без учебного года и четвертей не работают журнал и расписание. Их создаёт системный администратор.',
    },
    {
      id: 'classes', title: 'Классы', done: input.classes > 0, view: 'classes', action: 'Создать класс',
      text: input.classes > 0 ? `Классов в этом учебном году: ${input.classes}.` : 'Создайте классы этого учебного года, например 5А и 5Б.',
    },
    {
      id: 'subjects', title: 'Предметы', done: input.subjects > 0, view: 'subjects', action: 'Добавить предметы',
      text: input.subjects > 0 ? `Предметов: ${input.subjects}.` : 'Добавьте предметы — можно выбрать из типового списка.',
    },
    {
      id: 'teachers', title: 'Учителя', done: input.teachers > 0, view: 'teachers', action: 'Добавить учителя',
      text: input.teachers > 0 ? `Учителей: ${input.teachers}.` : 'Добавьте учителей и укажите, что и в каких классах они ведут.',
    },
    {
      id: 'students', title: 'Ученики и родители', done: input.students > 0, view: 'students', action: 'Добавить ученика',
      text: input.students > 0 ? `Учеников: ${input.students}.` : 'Добавьте учеников в классы, а вместе с ними — родителей.',
    },
    {
      id: 'schedule', title: 'Расписание', done: input.classes > 0 && without === 0, view: 'schedule', classId: firstWithout?.id, action: 'Составить расписание',
      text: input.classes === 0 ? 'Появится после создания классов.'
        : without === 0 ? 'У всех классов есть расписание на текущую четверть.'
          : `Без расписания: ${input.classesWithoutSchedule.slice(0, 4).map((item) => item.name).join(', ')}${without > 4 ? ` и ещё ${without - 4}` : ''}.`,
    },
  ]
}

const maxGenerateDays = 119

/**
 * Даты, на которые создавать уроки после правки расписания: с сегодняшнего дня (прошедшие не трогаем) до конца
 * периода, но не больше, чем сервер создаёт за один раз (120 дней). null — период уже закончился.
 */
export function lessonRange(period: Pick<AcademicPeriod, 'starts_on' | 'ends_on'>, today: string): { from: string; to: string } | null {
  const from = today > period.starts_on ? today : period.starts_on
  if (from > period.ends_on) return null
  const limit = addDays(from, maxGenerateDays)
  return { from, to: period.ends_on < limit ? period.ends_on : limit }
}

/** Варианты логина: подсказка из ФИО, затем с номером — у однофамильцев с одинаковыми инициалами. */
export function loginCandidates(base: string): string[] {
  const clean = base.length >= 3 ? base : `${base || 'user'}.user`
  return [clean, ...Array.from({ length: 8 }, (_, index) => `${clean}${index + 2}`)]
}

/** Типовые предметы основной и средней школы: администратор отмечает нужные, а не набирает вручную. */
export const standardSubjects = [
  'Русский язык', 'Литература', 'Математика', 'Алгебра', 'Геометрия', 'Вероятность и статистика', 'Информатика',
  'Английский язык', 'История', 'Обществознание', 'География', 'Биология', 'Физика', 'Химия', 'Окружающий мир',
  'Музыка', 'Изобразительное искусство', 'Труд (технология)', 'Физическая культура', 'ОБЗР',
]
