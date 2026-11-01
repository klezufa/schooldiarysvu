import type { Homework, JournalEntry, User } from '../api/types'

export const userFixture: User = {
  id: '10000000-0000-4000-8000-000000000001', login: 'test.user', display_name: 'Тестовый Пользователь', is_active: true,
  must_change_password: false, roles: ['student'], student_id: '20000000-0000-4000-8000-000000000001',
}

export const entryFixture: JournalEntry = {
  student_id: '20000000-0000-4000-8000-000000000001', student_name: 'Ученик Тестовый',
  lesson_id: '30000000-0000-4000-8000-000000000001', lesson_date: '2026-09-01',
  lesson_work_id: '40000000-0000-4000-8000-000000000001', work_type: 'classwork', work_title: 'Проверка', weight: 1,
  grade_id: '50000000-0000-4000-8000-000000000001', grade_value: 4, grade_version: 1, grade_comment: '',
}

export const homeworkFixture: Homework = {
  id: '60000000-0000-4000-8000-000000000001', class_id: '70000000-0000-4000-8000-000000000001', class_name: '1 А',
  subject_id: '80000000-0000-4000-8000-000000000001', subject_name: 'Математика', text: 'Решить задачи 1–3',
  due_date: '2099-09-01', status: 'published', version: 1, completed: false, files: [],
}
