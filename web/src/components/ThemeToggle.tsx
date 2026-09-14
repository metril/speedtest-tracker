import { useRef } from 'react';
import { useTheme, type Theme } from '../lib/theme';

const OPTIONS: { value: Theme; label: string }[] = [
  { value: 'system', label: 'System' },
  { value: 'light', label: 'Light' },
  { value: 'dark', label: 'Dark' },
];

/** ThemeToggle is a three-way radio group using the standard roving-
 * tabindex pattern: only the checked option sits in the tab order, and
 * arrow/Home/End keys move both selection and focus across the other two,
 * per the ARIA authoring practices for a single-select radio group. */
export function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  const buttonRefs = useRef<Array<HTMLButtonElement | null>>([]);

  const selectIndex = (index: number) => {
    const wrapped = (index + OPTIONS.length) % OPTIONS.length;
    setTheme(OPTIONS[wrapped].value);
    buttonRefs.current[wrapped]?.focus();
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLButtonElement>, index: number) => {
    switch (e.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        e.preventDefault();
        selectIndex(index + 1);
        break;
      case 'ArrowLeft':
      case 'ArrowUp':
        e.preventDefault();
        selectIndex(index - 1);
        break;
      case 'Home':
        e.preventDefault();
        selectIndex(0);
        break;
      case 'End':
        e.preventDefault();
        selectIndex(OPTIONS.length - 1);
        break;
      default:
        break;
    }
  };

  return (
    <div role="radiogroup" aria-label="Theme" className="flex rounded border border-line text-xs">
      {OPTIONS.map(({ value, label }, index) => (
        <button
          key={value}
          ref={(el) => { buttonRefs.current[index] = el; }}
          type="button"
          role="radio"
          aria-checked={theme === value}
          tabIndex={theme === value ? 0 : -1}
          onClick={() => setTheme(value)}
          onKeyDown={(e) => onKeyDown(e, index)}
          className={`px-2 py-1 first:rounded-l last:rounded-r ${
            theme === value ? 'bg-accent text-accent-fg' : 'text-muted hover:text-fg'
          }`}
        >
          {label}
        </button>
      ))}
    </div>
  );
}
