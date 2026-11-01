import { useCallback, useState } from 'react'

export type AdminMode = 'simple' | 'advanced'

const storageKey = 'diary.admin-mode'

function readMode(): AdminMode {
  try {
    return window.localStorage.getItem(storageKey) === 'advanced' ? 'advanced' : 'simple'
  } catch {
    return 'simple'
  }
}

/**
 * Режим администрирования — удобство конкретного компьютера, а не настройка школы: по умолчанию упрощённый, выбор
 * запоминается в браузере. Права от режима не зависят — их проверяет сервер.
 */
export function useAdminMode(): [AdminMode, (mode: AdminMode) => void] {
  const [mode, setMode] = useState<AdminMode>(readMode)
  const choose = useCallback((next: AdminMode) => {
    setMode(next)
    try {
      window.localStorage.setItem(storageKey, next)
    } catch {
      // Хранилище недоступно (приватный режим): режим действует до перезагрузки страницы.
    }
  }, [])
  return [mode, choose]
}
