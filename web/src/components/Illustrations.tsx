/** Векторные иллюстрации интерфейса (без внешних изображений: система работает в закрытой сети школы). */

export function BookMark(): React.JSX.Element {
  return (
    <svg viewBox="0 0 32 32" fill="none" aria-hidden="true">
      <path d="M4 7.5C4 6.7 4.7 6 5.5 6H12c1.7 0 3.1.8 4 2 .9-1.2 2.3-2 4-2h6.5c.8 0 1.5.7 1.5 1.5V24c0 .8-.7 1.5-1.5 1.5H20c-1.7 0-3.1.8-4 2-.9-1.2-2.3-2-4-2H5.5C4.7 25.5 4 24.8 4 24V7.5Z" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />
      <path d="M16 8v19.5M8 11h4.5M8 15h4.5M8 19h4.5M19.5 11H24M19.5 15H24M19.5 19H24" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  )
}

/** Стопка книг, стакан с карандашами и глобус — для приветственного баннера. */
export function DeskIllustration(): React.JSX.Element {
  return (
    <svg viewBox="0 0 260 190" role="img" aria-label="Книги, карандаши и глобус">
      <ellipse cx="135" cy="176" rx="118" ry="10" fill="#c9daf8" opacity=".6" />
      <rect x="22" y="140" width="112" height="22" rx="4" fill="#2463eb" />
      <rect x="22" y="140" width="112" height="6" rx="3" fill="#5b8def" />
      <rect x="30" y="118" width="98" height="22" rx="4" fill="#eb6834" />
      <rect x="30" y="118" width="98" height="6" rx="3" fill="#f59a72" />
      <rect x="18" y="96" width="104" height="22" rx="4" fill="#1baf7a" />
      <rect x="18" y="96" width="104" height="6" rx="3" fill="#5fd3a8" />
      <rect x="26" y="162" width="104" height="6" rx="2" fill="#f4f7fc" />
      <rect x="140" y="98" width="44" height="64" rx="8" fill="#10244b" />
      <rect x="148" y="40" width="7" height="62" rx="2" transform="rotate(-8 151 71)" fill="#eda100" />
      <rect x="160" y="34" width="7" height="68" rx="2" fill="#e34948" />
      <rect x="172" y="44" width="7" height="58" rx="2" transform="rotate(9 175 73)" fill="#2a78d6" />
      <path d="M150 40l3.5-9 3.5 9z" transform="rotate(-8 151 71)" fill="#f7d79a" />
      <path d="M160 34l3.5-9 3.5 9z" fill="#f4b3b3" />
      <path d="M172 44l3.5-9 3.5 9z" transform="rotate(9 175 73)" fill="#b8d3f6" />
      <circle cx="222" cy="92" r="34" fill="#5b8def" />
      <path d="M200 74c10 4 14 12 10 20s4 14 12 12 14 6 12 14M230 60c-4 8 2 12 8 12s8 8 2 14" stroke="#1baf7a" strokeWidth="9" strokeLinecap="round" fill="none" />
      <circle cx="222" cy="92" r="34" fill="none" stroke="#1a4fc4" strokeWidth="3" />
      <path d="M180 92a42 42 0 0 0 42 42" stroke="#10244b" strokeWidth="5" strokeLinecap="round" fill="none" />
      <rect x="216" y="134" width="12" height="20" rx="3" fill="#10244b" />
      <rect x="204" y="152" width="36" height="10" rx="5" fill="#10244b" />
    </svg>
  )
}

/** Здание школы — для страницы входа. */
export function SchoolIllustration(): React.JSX.Element {
  const windows = []
  for (let row = 0; row < 3; row += 1) {
    for (let col = 0; col < 8; col += 1) {
      if (col === 3 || col === 4) continue
      windows.push(<rect key={`${row}-${col}`} x={58 + col * 40} y={116 + row * 38} width="22" height="24" rx="3" fill="#cfe0ff" opacity={0.92} />)
    }
  }
  return (
    <svg viewBox="0 0 440 300" role="img" aria-label="Здание школы">
      <ellipse cx="220" cy="284" rx="210" ry="14" fill="#0b1a39" opacity=".35" />
      <rect x="40" y="96" width="360" height="184" rx="8" fill="#f4f7fc" />
      <rect x="40" y="96" width="360" height="14" fill="#dbe6ff" />
      <rect x="162" y="64" width="116" height="216" rx="8" fill="#ffffff" />
      <path d="M150 70 220 26l70 44z" fill="#2463eb" />
      <rect x="196" y="82" width="48" height="48" rx="24" fill="#2463eb" />
      <path d="M210 96v20M220 92v24M230 96v20" stroke="white" strokeWidth="3" strokeLinecap="round" />
      {windows}
      <rect x="196" y="216" width="48" height="64" rx="4" fill="#173264" />
      <rect x="182" y="150" width="76" height="40" rx="4" fill="#cfe0ff" />
      <path d="M220 150v40M182 170h76" stroke="#ffffff" strokeWidth="3" />
      <rect x="40" y="270" width="360" height="10" fill="#c3cfe2" />
      <circle cx="26" cy="232" r="26" fill="#1baf7a" />
      <circle cx="414" cy="226" r="30" fill="#1baf7a" />
      <circle cx="404" cy="200" r="20" fill="#3cc28f" />
      <rect x="22" y="240" width="8" height="40" fill="#0e7f5a" />
      <rect x="410" y="240" width="8" height="40" fill="#0e7f5a" />
      <path d="M220 10v18" stroke="#ffffff" strokeWidth="3" />
      <path d="M220 10h22l-6 6 6 6h-22" fill="#e34948" />
    </svg>
  )
}
