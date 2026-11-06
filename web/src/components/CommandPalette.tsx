import { useEffect, useId, useMemo, useRef, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { Bell, CornerDownLeft, LogOut, Monitor, Moon, Search, Sun } from 'lucide-react'
import type { NavigationItem } from '../lib/navigation'
import { useTheme } from '../themeContext'

type Command = { id: string; group: string; label: string; hint?: string; icon: typeof Search; keywords?: string; run: () => void }

/** «ё» и регистр не должны мешать поиску: «отчеты» находит «Отчёты». */
function normalize(text: string): string {
  return text.toLocaleLowerCase('ru').replaceAll('ё', 'е')
}

/**
 * Палитра команд (Ctrl+K): переход в любой доступный раздел и частые действия. Разделы берутся из того же
 * navigationFor, что и меню, — это удобство интерфейса, доступ по-прежнему проверяет сервер.
 */
export function CommandPalette({ open, onOpenChange, navigation, onNavigate, onNotifications, onSignOut }: {
  open: boolean
  onOpenChange: (open: boolean) => void
  navigation: NavigationItem[]
  onNavigate: (to: string) => void
  onNotifications: () => void
  onSignOut: () => void
}): React.JSX.Element {
  const id = useId()
  const { choice, setChoice } = useTheme()
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const listRef = useRef<HTMLUListElement>(null)

  const commands = useMemo<Command[]>(() => [
    ...navigation.map((item) => ({ id: `go:${item.to}`, group: 'Разделы', label: item.label, icon: item.icon, run: () => onNavigate(item.to) })),
    { id: 'notifications', group: 'Действия', label: 'Открыть уведомления', icon: Bell, run: onNotifications },
    { id: 'theme:light', group: 'Оформление', label: 'Светлая тема', hint: choice === 'light' ? 'выбрана' : undefined, icon: Sun, keywords: 'тема день', run: () => setChoice('light') },
    { id: 'theme:dark', group: 'Оформление', label: 'Тёмная тема', hint: choice === 'dark' ? 'выбрана' : undefined, icon: Moon, keywords: 'тема ночь вечер', run: () => setChoice('dark') },
    { id: 'theme:system', group: 'Оформление', label: 'Тема как в системе', hint: choice === 'system' ? 'выбрана' : undefined, icon: Monitor, keywords: 'тема авто', run: () => setChoice('system') },
    { id: 'logout', group: 'Действия', label: 'Выйти', icon: LogOut, keywords: 'выход', run: onSignOut },
  ], [navigation, onNavigate, onNotifications, onSignOut, choice, setChoice])

  const results = useMemo(() => {
    const words = normalize(query).split(/\s+/).filter(Boolean)
    if (!words.length) return commands
    return commands.filter((command) => {
      const text = normalize(`${command.label} ${command.group} ${command.keywords ?? ''}`)
      return words.every((word) => text.includes(word))
    })
  }, [commands, query])

  useEffect(() => { setActive(0) }, [query, open])
  useEffect(() => { if (!open) setQuery('') }, [open])
  useEffect(() => {
    listRef.current?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' })
  }, [active])

  const run = (command: Command | undefined) => {
    if (!command) return
    onOpenChange(false)
    command.run()
  }
  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'ArrowDown') { event.preventDefault(); setActive((value) => (results.length ? (value + 1) % results.length : 0)) }
    if (event.key === 'ArrowUp') { event.preventDefault(); setActive((value) => (results.length ? (value - 1 + results.length) % results.length : 0)) }
    if (event.key === 'Enter') { event.preventDefault(); run(results[active]) }
  }

  let lastGroup = ''
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content command-dialog" aria-describedby={undefined} onKeyDown={onKeyDown}>
          <Dialog.Title className="sr-only">Поиск разделов и действий</Dialog.Title>
          <div className="command-search">
            <Search aria-hidden="true" />
            <input
              autoFocus
              role="combobox"
              aria-label="Найти раздел или действие"
              aria-expanded="true"
              aria-controls={`${id}-list`}
              aria-activedescendant={results[active] ? `${id}-${results[active].id}` : undefined}
              aria-autocomplete="list"
              placeholder="Раздел или действие…"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              autoComplete="off"
              spellCheck={false}
            />
          </div>
          <ul className="command-list" role="listbox" id={`${id}-list`} aria-label="Разделы и действия" ref={listRef}>
            {results.map((command, index) => {
              const heading = command.group !== lastGroup ? command.group : null
              lastGroup = command.group
              const Icon = command.icon
              return (
                <li key={command.id} role="presentation">
                  {heading && <div className="command-group" aria-hidden="true">{heading}</div>}
                  <div
                    id={`${id}-${command.id}`}
                    role="option"
                    aria-selected={index === active}
                    className="command-item"
                    onMouseMove={() => setActive(index)}
                    onClick={() => run(command)}
                  >
                    <Icon aria-hidden="true" />
                    <span>{command.label}</span>
                    {command.hint && <small>{command.hint}</small>}
                  </div>
                </li>
              )
            })}
          </ul>
          {!results.length && <p className="command-empty">Ничего не нашлось. Попробуйте другое слово.</p>}
          <div className="command-foot" aria-hidden="true">
            <span><kbd>↑</kbd><kbd>↓</kbd> выбрать</span>
            <span><kbd><CornerDownLeft /></kbd> открыть</span>
            <span><kbd>Esc</kbd> закрыть</span>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
