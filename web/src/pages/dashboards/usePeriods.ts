import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { AcademicPeriod, AcademicYear } from '../../api/types'
import { dateISO } from '../../lib/dates'
import { previousPeriod } from './helpers'

/** Периоды текущего учебного года и выбранный период дашборда ('' — текущий по мнению сервера). */
export function useDashboardPeriod() {
  const [periodId, setPeriodId] = useState('')
  const years = useQuery({ queryKey: ['academic-years'], queryFn: () => api<AcademicYear[]>('/academic-years') })
  const year = years.data?.find((item) => item.is_current) ?? years.data?.[0]
  const periods = useQuery({
    queryKey: ['academic-periods', year?.id],
    queryFn: () => api<AcademicPeriod[]>(`/academic-periods?academic_year_id=${year?.id}`),
    enabled: Boolean(year),
  })
  return { periodId, setPeriodId, periods: periods.data, previous: previousPeriod(periods.data, dateISO()) }
}
