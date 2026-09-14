import { useTheme, type Theme } from '../lib/theme';

const OPTIONS: { value: Theme; label: string }[] = [
  { value: 'system', label: 'System' },
  { value: 'light', label: 'Light' },
  { value: 'dark', label: 'Dark' },
];

/** ThemeToggle is a three-way radio group; arrow keys and Space work
 * because every option is a real focusable button with aria-checked. */
export function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  return (
    <div role="radiogroup" aria-label="Theme" className="flex rounded border border-line text-xs">
      {OPTIONS.map(({ value, label }) => (
        <button
          key={value}
          type="button"
          role="radio"
          aria-checked={theme === value}
          tabIndex={theme === value ? 0 : -1}
          onClick={() => setTheme(value)}
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
