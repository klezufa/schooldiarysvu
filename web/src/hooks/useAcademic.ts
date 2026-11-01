import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { AcademicPeriod, NamedEntity } from '../api/types'
import { classesOfYear, pickCurrentPeriod } from '../lib/academicYears'

export function useAcademic() {
  const years = useQuery({ queryKey: ['academic-years'], queryFn: () => api<import('../api/types').AcademicYear[]>('/academic-years') })
  const currentYear = useMemo(() => years.data?.find((year) => year.is_current) ?? years.data?.[0], [years.data])
  const [yearId, setYearId] = useState('')
  useEffect(() => { if (currentYear && !yearId) setYearId(currentYear.id) }, [currentYear, yearId])
  const periods = useQuery({ queryKey: ['academic-periods', yearId], queryFn: () => api<AcademicPeriod[]>(`/academic-periods?academic_year_id=${yearId}`), enabled: Boolean(yearId) })
  const currentPeriod = useMemo(() => pickCurrentPeriod(periods.data), [periods.data])
  const [periodId, setPeriodId] = useState('')
  useEffect(() => { if (currentPeriod && (!periodId || !periods.data?.some((period) => period.id === periodId))) setPeriodId(currentPeriod.id) }, [currentPeriod, periodId, periods.data])
  // Классы выбранного года: после перехода на новый год «5А» есть в двух годах, и в фильтрах журнала, отчётов,
  // объявлений и заданий нужны классы того года, к которому относится выбранный период.
  const classes = useQuery({
    queryKey: ['classes'],
    queryFn: () => api<NamedEntity[]>('/classes'),
    select: (data: NamedEntity[]) => (yearId && data.some((item) => item.academic_year_id === yearId) ? classesOfYear(data, yearId) : data),
  })
  const subjects = useQuery({ queryKey: ['subjects'], queryFn: () => api<import('../api/types').NamedEntity[]>('/subjects') })
  const scales = useQuery({ queryKey: ['grade-scales'], queryFn: () => api<import('../api/types').GradeScale[]>('/grade-scales') })
  return { years, classes, subjects, periods, scales, yearId, setYearId, periodId, setPeriodId, currentPeriod }
}
