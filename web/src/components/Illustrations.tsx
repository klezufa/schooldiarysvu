/** Векторные иллюстрации интерфейса (без внешних изображений: система работает в закрытой сети школы).
 *  Цвета задают классы .ill-* в styles/pages.css через токены, поэтому рисунки следуют теме. */

export function BookMark(): React.JSX.Element {
  return (
    <svg viewBox="0 0 32 32" fill="none" aria-hidden="true">
      <path d="M4 7.5C4 6.7 4.7 6 5.5 6H12c1.7 0 3.1.8 4 2 .9-1.2 2.3-2 4-2h6.5c.8 0 1.5.7 1.5 1.5V24c0 .8-.7 1.5-1.5 1.5H20c-1.7 0-3.1.8-4 2-.9-1.2-2.3-2-4-2H5.5C4.7 25.5 4 24.8 4 24V7.5Z" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />
      <path d="M16 8v19.5M8 11h4.5M8 15h4.5M8 19h4.5M19.5 11H24M19.5 15H24M19.5 19H24" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  )
}

/** Линии клетки внутри прямоугольника: шаг step, без SVG-паттернов, чтобы не плодить id на странице. */
function GridLines({ x, y, width, height, step }: { x: number; y: number; width: number; height: number; step: number }): React.JSX.Element {
  const lines: string[] = []
  for (let lineX = x + step; lineX < x + width; lineX += step) lines.push(`M${lineX} ${y}V${y + height}`)
  for (let lineY = y + step; lineY < y + height; lineY += step) lines.push(`M${x} ${lineY}H${x + width}`)
  return <path className="ill-grid" d={lines.join('')} />
}

/** Тетрадь в клетку с записями чернилами, пятёркой на полях и карандашом — для приветствия на главной. */
export function DeskIllustration(): React.JSX.Element {
  return (
    <svg viewBox="0 0 240 180" role="img" aria-label="Тетрадь в клетку с оценкой и карандаш">
      <ellipse className="ill-shadow" cx="122" cy="166" rx="96" ry="8" />
      <g transform="rotate(-6 120 90)">
        <rect className="ill-paper-edge" x="40" y="22" width="150" height="138" rx="6" />
        <rect className="ill-paper" x="36" y="18" width="150" height="138" rx="6" />
        <GridLines x={36} y={18} width={150} height={138} step={10} />
        <path className="ill-margin" d="M160 18v138" />
        <path className="ill-ink" d="M50 40h62M50 60h84M50 80h52M50 100h74M50 120h40" />
        <path className="ill-red" d="M176 46h-11l-2 9c6-3 14 0 12 7-1.6 6-10.5 7-13.5 3" />
      </g>
      <g transform="rotate(32 196 108)">
        <rect className="ill-pencil" x="190" y="58" width="12" height="92" rx="2" />
        <rect className="ill-pencil-band" x="190" y="58" width="12" height="10" rx="2" />
        <path className="ill-pencil-wood" d="M190 150h12l-6 16z" />
        <path className="ill-pencil-lead" d="M194.2 161.5h3.6l-1.8 4.5z" />
      </g>
    </svg>
  )
}

/**
 * Раскрытый школьный дневник: слева уроки на день, справа задания и колонка оценок с полями. В клетке оценки
 * красная ручка один раз «пишет» пятёрку (stroke-dashoffset), при «уменьшить движение» пятёрка уже написана.
 */
export function SchoolIllustration(): React.JSX.Element {
  const rows = [0, 1, 2, 3, 4]
  return (
    <svg viewBox="0 0 440 300" role="img" aria-label="Раскрытый школьный дневник с пятёркой">
      <ellipse className="ill-shadow-dark" cx="220" cy="282" rx="196" ry="12" />
      <path className="ill-paper-edge" d="M28 52 216 44v232L28 272Z M224 44l188 8v220l-188 4Z" />
      <path className="ill-paper" d="M24 46 214 38v232L24 266Z" />
      <path className="ill-paper" d="M226 38l190 8v220l-190 4Z" />
      <path className="ill-spine" d="M214 38v232M226 38v232" />
      {/* левая страница: день недели и уроки */}
      <path className="ill-ink-strong" d="M44 66h64" />
      {rows.map((row) => (
        <g key={`left-${row}`}>
          <path className="ill-rule" d={`M40 ${96 + row * 34}H200`} />
          <path className="ill-ink" d={`M48 ${88 + row * 34}h${[58, 44, 66, 38, 52][row]}`} />
          <path className="ill-ink-soft" d={`M128 ${88 + row * 34}h${[50, 62, 34, 56, 44][row]}`} />
        </g>
      ))}
      {/* правая страница: задания, колонка оценок и поля */}
      <path className="ill-margin" d="M372 46v222" />
      <path className="ill-rule-strong" d="M330 46v222" />
      {rows.map((row) => (
        <g key={`right-${row}`}>
          <path className="ill-rule" d={`M240 ${96 + row * 34}H400`} />
          <path className="ill-ink-soft" d={`M250 ${88 + row * 34}h${[64, 48, 70, 40, 58][row]}`} />
        </g>
      ))}
      <path className="ill-red ill-write" d="M358 112h-12l-2.5 10.5c7-3.5 15.5 0 13.5 8-1.8 7-12 8-15.5 3.5" />
      <path className="ill-red ill-write ill-write-late" d="M380 128c4-6 9-6 10-1s-6 9 2 8" />
      {/* ручка с красной пастой */}
      <g className="ill-pen">
        <g transform="rotate(38 372 170)">
          <rect className="ill-pen-body" x="366" y="150" width="11" height="100" rx="5.5" />
          <rect className="ill-pen-cap" x="366" y="214" width="11" height="36" rx="5.5" />
          <path className="ill-pen-tip" d="M366.5 152 371.5 134l5 18z" />
        </g>
      </g>
    </svg>
  )
}
