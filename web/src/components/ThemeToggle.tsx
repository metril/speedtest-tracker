import { useRef, type ComponentType } from 'react';
import { Monitor, Moon, Sun } from 'lucide-react';
import { useTheme, type Theme } from '../lib/theme';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';

const OPTIONS: { value: Theme; label: string }[] = [
  { value: 'system', label: 'System' },
  { value: 'light', label: 'Light' },
  { value: 'dark', label: 'Dark' },
];

const ICONS: Record<Theme, ComponentType<{ className?: string }>> = {
  system: Monitor,
  light: Sun,
  dark: Moon,
};

/** CollapsedThemeToggle is a single icon button that cycles system → light
 * → dark on click, for the collapsed sidebar rail where the full
 * radiogroup label text doesn't fit. */
function CollapsedThemeToggle() {
  const { theme, setTheme } = useTheme();
  const index = OPTIONS.findIndex((o) => o.value === theme);
  const current = OPTIONS[index === -1 ? 0 : index];
  const Icon = ICONS[current.value];

  const cycle = () => {
    const next = OPTIONS[(index + 1) % OPTIONS.length];
    setTheme(next.value);
  };

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label={`Theme: ${current.label}`}
            onClick={cycle}
            className="flex h-8 w-8 items-center justify-center rounded text-muted hover:bg-raised hover:text-fg"
          >
            <Icon className="h-4 w-4" />
          </button>
        </TooltipTrigger>
        <TooltipContent>{`Theme: ${current.label}`}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

/** ThemeToggle is a three-way radio group using the standard roving-
 * tabindex pattern: only the checked option sits in the tab order, and
 * arrow/Home/End keys move both selection and focus across the other two,
 * per the ARIA authoring practices for a single-select radio group.
 * With `collapsed`, it renders as a single cycling icon button instead. */
export function ThemeToggle({ collapsed }: { collapsed?: boolean } = {}) {
  const { theme, setTheme } = useTheme();
  const buttonRefs = useRef<Array<HTMLButtonElement | null>>([]);

  if (collapsed) return <CollapsedThemeToggle />;

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
